// Zwischenspeicher für die Sitzungsprüfung.
//
// Ohne ihn läge bei jeder einzelnen Anfrage eine Abfrage auf der Datenbank --
// billig, aber eben bei jedem Tastendruck im Editor, der speichert. Der
// Speicher hält für kurze Zeit fest, ob eine Sitzung gilt.
//
// Die Wahrheit steht weiterhin in der Datenbank. Der Speicher darf leer sein,
// veraltet sein oder ganz fehlen: dann wird eben gefragt. Deshalb ist auch das
// Widerrufen sofort wirksam -- es schreibt den Eintrag um, statt auf sein
// Ablaufen zu warten.
package handlers

import (
	"sync"
	"time"
)

// speicherDauer ist kurz gewählt. Sie ist die Zeitspanne, in der ein anderswo
// widerrufener Eintrag hier noch als gültig gelten könnte -- bei Redis über
// mehrere Instanzen hinweg, im eigenen Speicher nur theoretisch.
const speicherDauer = 30 * time.Second

type speicherEintrag struct {
	gilt bool
	bis  time.Time
}

type sitzungsSpeicher struct {
	sync.Mutex
	eintraege map[string]speicherEintrag
}

// NeuerSitzungsSpeicher wird von main aufgerufen.
func NeuerSitzungsSpeicher() *sitzungsSpeicher {
	return &sitzungsSpeicher{eintraege: map[string]speicherEintrag{}}
}

// sitzungAusSpeicher liefert (gilt, gefunden).
func (s *Server) sitzungAusSpeicher(sid string) (bool, bool) {
	if s.Sitzungen == nil {
		return false, false
	}
	if s.Redis != nil {
		if gilt, ok := s.redisSitzung(sid); ok {
			return gilt, true
		}
	}
	s.Sitzungen.Lock()
	defer s.Sitzungen.Unlock()
	e, ok := s.Sitzungen.eintraege[sid]
	if !ok || time.Now().After(e.bis) {
		return false, false
	}
	return e.gilt, true
}

// sitzungMerken hält das Ergebnis kurz fest.
func (s *Server) sitzungMerken(sid string, gilt bool) {
	if s.Sitzungen == nil {
		return
	}
	if s.Redis != nil {
		s.redisSitzungMerken(sid, gilt)
	}
	s.Sitzungen.Lock()
	defer s.Sitzungen.Unlock()
	// Beim Wachsen aufräumen statt mit einer eigenen Uhr: der Speicher ist
	// klein, und ein Durchgang über ein paar tausend Einträge kostet nichts.
	if len(s.Sitzungen.eintraege) > 4096 {
		jetzt := time.Now()
		for k, v := range s.Sitzungen.eintraege {
			if jetzt.After(v.bis) {
				delete(s.Sitzungen.eintraege, k)
			}
		}
	}
	s.Sitzungen.eintraege[sid] = speicherEintrag{gilt: gilt, bis: time.Now().Add(speicherDauer)}
}
