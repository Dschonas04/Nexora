// The pulse: what is happening right now, not what happened yesterday.
//
// Until now the system view answered questions about state: which service
// responds, how large the database is, what stood in the configuration at
// start. All of that changes rarely. What was missing is the other kind of
// question, the one asked while somebody says "it is stalling right now": how
// many requests are running this second, how long do they take, and is
// anything waiting.
//
// Two decisions shape this package.
//
// Counting happens in per-second buckets, not as a running average. An average
// over the whole uptime dilutes every spike into invisibility: one minute in
// which nothing works vanishes inside eight hours of normal operation. Sixty
// buckets show the last minute the way it was.
//
// Measuring happens without a lock on the hot path. Every request touches this,
// and a lock shared by all requests would itself be the slowdown it is meant to
// help find. The buckets are therefore atomic counters, and the switch to the
// next bucket happens off the clock rather than off a timer.
package puls

import (
	"sync/atomic"
	"time"
)

// Faecher is the length of the memory. One minute, because that is the span
// somebody takes in while looking at the page.
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

	// How many requests the service has answered since it started. The buckets
	// forget after a minute -- right for showing the recent shape, but the
	// question "how much has this service done in total" is only answered by a
	// counter that never goes back.
	gesamt atomic.Int64
}

func Neu() *Messer {
	return &Messer{seit: time.Now()}
}

// Beginn reports a request that has started and returns what to call at the
// end. A return value instead of two methods, so that a forgotten end becomes
// impossible: whoever begins holds the end in their hand.
func (m *Messer) Beginn() func(status int) {
	m.laufend.Add(1)
	start := time.Now()
	return func(status int) {
		m.laufend.Add(-1)
		m.gesamt.Add(1)

		jetzt := time.Now()
		sek := jetzt.Unix()
		f := &m.faecher[sek%Faecher]
		// If the bucket still belongs to an earlier minute it is cleared rather
		// than added to. Without that a number a minute old would stand next to
		// a fresh one, and nothing would show which was which.
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

// Sekunde is one bucket as it goes out.
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

// Lies returns the last minute, oldest first.
//
// The current second stays out: only part of it has passed, and half a second
// would look like a collapse.
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
		// Only buckets that really belong to this second. Anything else comes
		// from an earlier minute and is not merely old but wrong.
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
