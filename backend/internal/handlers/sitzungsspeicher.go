// A short-lived cache in front of the session check.
//
// Without it every single request would put a query on the database. That query
// is cheap, but it happens on every keystroke the editor saves. The cache
// remembers for a few seconds whether a session is still valid.
//
// The truth stays in the database. The cache may be empty, stale or missing
// altogether: then the database is asked. That is also why revoking takes
// effect at once, it rewrites the entry instead of waiting for it to expire.
package handlers

import (
	"sync"
	"time"
)

// speicherDauer is deliberately short. It is the window in which an entry
// revoked elsewhere could still count as valid here: across instances when
// Redis is in play, and only in theory within a single process.
const speicherDauer = 30 * time.Second

type speicherEintrag struct {
	gilt bool
	bis  time.Time
}

type sitzungsSpeicher struct {
	sync.Mutex
	eintraege map[string]speicherEintrag
}

// NeuerSitzungsSpeicher is called from main.
func NeuerSitzungsSpeicher() *sitzungsSpeicher {
	return &sitzungsSpeicher{eintraege: map[string]speicherEintrag{}}
}

// sitzungAusSpeicher returns (valid, found).
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

// sitzungMerken records the result for a short while.
func (s *Server) sitzungMerken(sid string, gilt bool) {
	if s.Sitzungen == nil {
		return
	}
	if s.Redis != nil {
		s.redisSitzungMerken(sid, gilt)
	}
	s.Sitzungen.Lock()
	defer s.Sitzungen.Unlock()
	// Sweep while growing rather than on a timer of its own: the map is small,
	// and one pass over a few thousand entries costs nothing.
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
