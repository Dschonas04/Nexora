// Der Puls: was gerade passiert, nicht was gestern war.
//
// Die Systemansicht beantwortete bisher Fragen über den Zustand: welcher Dienst
// antwortet, wie groß ist die Datenbank, was stand beim Start in der
// Konfiguration. Alles davon ändert sich selten. Was fehlt, ist die andere
// Sorte Frage, die man stellt, während jemand sagt "es hängt gerade": wie viele
// Anfragen laufen in dieser Sekunde, wie lange dauern sie, und wartet etwas.
//
// Zwei Entscheidungen prägen dieses Paket.
//
// Gezählt wird in Sekunden-Fächern, nicht als laufender Durchschnitt. Ein
// Durchschnitt über die ganze Laufzeit verdünnt jede Spitze bis zur
// Unsichtbarkeit: eine Minute, in der nichts geht, verschwindet in acht Stunden
// Normalbetrieb. Sechzig Fächer zeigen die letzte Minute so, wie sie war.
//
// Gemessen wird ohne Sperre auf dem heißen Weg. Jede Anfrage fasst das hier an,
// und eine Sperre, die alle Anfragen teilen, wäre selbst die Verlangsamung, die
// zu finden sie helfen soll. Die Fächer sind darum atomare Zähler, und der
// Wechsel des Fachs geschieht über die Uhr und nicht über einen Zeitgeber.
package puls

import (
	"sync/atomic"
	"time"
)

// Faecher ist die Länge des Gedächtnisses. Eine Minute, weil das die Spanne
// ist, über die jemand hinsieht, während er auf die Seite schaut.
const Faecher = 60

type fach struct {
	sekunde   atomic.Int64 // Unix-Sekunde, für die dieses Fach gilt
	anfragen  atomic.Int64
	fehler    atomic.Int64 // Antwortstatus ab 500
	abgelehnt atomic.Int64 // 4xx, also abgewiesen und nicht kaputt
	dauerNS   atomic.Int64
	maxNS     atomic.Int64
}

// Messer sammelt. Eines je Dienst, angelegt beim Start.
type Messer struct {
	faecher [Faecher]fach
	laufend atomic.Int64 // gerade in Bearbeitung
	seit    time.Time

	// Zähler über die ganze Laufzeit, neben den Fächern.
	//
	// Die Fächer vergessen nach einer Minute, und das ist für die Anzeige
	// richtig. Prometheus braucht das Gegenteil: Zähler, die nur steigen. Es
	// bildet die Rate selbst aus zwei Abfragen, und ein Wert, der zwischendurch
	// zurückspringt, wird dort als Neustart des Dienstes gelesen.
	gesamt    atomic.Int64
	abgelehnt atomic.Int64
	fehler    atomic.Int64
	dauerNS   atomic.Int64
}

func Neu() *Messer {
	return &Messer{seit: time.Now()}
}

// Beginn meldet eine angefangene Anfrage und gibt zurück, was am Ende zu rufen
// ist. Ein Rückgabewert statt zweier Methoden, damit ein vergessenes Ende
// unmöglich wird: wer beginnt, hält das Ende in der Hand.
func (m *Messer) Beginn() func(status int) {
	m.laufend.Add(1)
	start := time.Now()
	return func(status int) {
		m.laufend.Add(-1)
		m.gesamt.Add(1)

		jetzt := time.Now()
		m.dauerNS.Add(jetzt.Sub(start).Nanoseconds())
		switch {
		case status >= 500:
			m.fehler.Add(1)
		case status >= 400:
			m.abgelehnt.Add(1)
		}

		sek := jetzt.Unix()
		f := &m.faecher[sek%Faecher]
		// Gehört das Fach noch zu einer früheren Minute, wird es geleert statt
		// weitergezählt. Ohne das stünde eine Minute alte Zahl neben einer
		// frischen, und niemand sähe den Unterschied an.
		if alt := f.sekunde.Load(); alt != sek {
			if f.sekunde.CompareAndSwap(alt, sek) {
				f.anfragen.Store(0)
				f.fehler.Store(0)
				f.abgelehnt.Store(0)
				f.dauerNS.Store(0)
				f.maxNS.Store(0)
			}
		}

		d := jetzt.Sub(start).Nanoseconds()
		f.anfragen.Add(1)
		f.dauerNS.Add(d)
		switch {
		case status >= 500:
			f.fehler.Add(1)
		case status >= 400:
			f.abgelehnt.Add(1)
		}
		for {
			alt := f.maxNS.Load()
			if d <= alt || f.maxNS.CompareAndSwap(alt, d) {
				break
			}
		}
	}
}

// Sekunde ist ein Fach, wie es nach außen geht.
type Sekunde struct {
	VorSekunden int     `json:"vorSekunden"`
	Anfragen    int64   `json:"anfragen"`
	Fehler      int64   `json:"fehler"`
	Abgelehnt   int64   `json:"abgelehnt"`
	MittelMS    float64 `json:"mittelMs"`
	MaxMS       float64 `json:"maxMs"`
}

// Stand ist die Momentaufnahme.
type Stand struct {
	Laufend     int64     `json:"laufend"`
	Gesamt      int64     `json:"gesamt"`
	LaufzeitSek int64     `json:"laufzeitSek"`
	Minute      []Sekunde `json:"minute"`
	ProSekunde  float64   `json:"proSekunde"`
	MittelMS    float64   `json:"mittelMs"`
	SpitzeMS    float64   `json:"spitzeMs"`
	Fehler      int64     `json:"fehler"`
	Abgelehnt   int64     `json:"abgelehnt"`
}

// Lies gibt die letzte Minute zurück, älteste zuerst.
//
// Die laufende Sekunde bleibt draußen: sie ist erst zum Teil vergangen, und
// eine halbe Sekunde sähe wie ein Einbruch aus.
func (m *Messer) Lies() Stand {
	jetzt := time.Now().Unix()
	s := Stand{
		Laufend:     m.laufend.Load(),
		Gesamt:      m.gesamt.Load(),
		LaufzeitSek: int64(time.Since(m.seit).Seconds()),
		Minute:      make([]Sekunde, 0, Faecher),
	}

	var summeAnfragen, summeDauer int64
	for zurueck := Faecher - 1; zurueck >= 1; zurueck-- {
		sek := jetzt - int64(zurueck)
		f := &m.faecher[sek%Faecher]
		// Nur Fächer, die wirklich zu dieser Sekunde gehören. Alles andere
		// stammt aus einer früheren Minute und ist nicht bloß alt, sondern
		// falsch.
		if f.sekunde.Load() != sek {
			s.Minute = append(s.Minute, Sekunde{VorSekunden: zurueck})
			continue
		}
		anfragen := f.anfragen.Load()
		dauer := f.dauerNS.Load()
		eintrag := Sekunde{
			VorSekunden: zurueck,
			Anfragen:    anfragen,
			Fehler:      f.fehler.Load(),
			Abgelehnt:   f.abgelehnt.Load(),
			MaxMS:       float64(f.maxNS.Load()) / 1e6,
		}
		if anfragen > 0 {
			eintrag.MittelMS = float64(dauer) / float64(anfragen) / 1e6
		}
		s.Minute = append(s.Minute, eintrag)
		summeAnfragen += anfragen
		summeDauer += dauer
		s.Fehler += eintrag.Fehler
		s.Abgelehnt += eintrag.Abgelehnt
		if eintrag.MaxMS > s.SpitzeMS {
			s.SpitzeMS = eintrag.MaxMS
		}
	}

	s.ProSekunde = float64(summeAnfragen) / float64(Faecher-1)
	if summeAnfragen > 0 {
		s.MittelMS = float64(summeDauer) / float64(summeAnfragen) / 1e6
	}
	return s
}

// SeitDemStart gibt die Zähler über die ganze Laufzeit zurück: beantwortet,
// davon abgewiesen und gescheitert, dazu die aufsummierte Dauer und wie viele
// gerade laufen. Für Prometheus, das die Rate selbst bildet.
func (m *Messer) SeitDemStart() (gesamt, abgelehnt, fehler int64, dauer time.Duration, laufend int64) {
	return m.gesamt.Load(), m.abgelehnt.Load(), m.fehler.Load(),
		time.Duration(m.dauerNS.Load()), m.laufend.Load()
}
