// Collaborative editing on the same page.
//
// The service does not produce its own text here: it only forwards packets
// between participants. What ends up in storage is computed by the browsers
// themselves; Yjs is built for that and does not need a central arbiter. This
// design keeps sessions intact even if the service restarts: browsers
// reconnect and reconcile, nothing is lost.
//
// Messages are forwarded to everyone in the room, including the sender. The
// audible echo may seem wasteful but is vital: the browser times out if it
// hears nothing for thirty seconds, and a single editor would otherwise
// never receive updates. Yjs tolerates receiving the same change twice; it
// becomes a no-op the second time.
//
// Permission to enter the room is the same check used for saving: only
// writers are allowed. A passive reader could otherwise inject text over the
// channel that another browser would unwittingly persist.
package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"golang.org/x/net/websocket"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

const (
	// Maximum number of simultaneous participants on a page. This is not a
	// technical limit but a safeguard against accidents: a page is edited by
	// a team, not a whole workforce.
	hoechstensImRaum = 25
	// How many packets a slow client may lag behind. After this it is kicked
	// out to avoid its backlog slowing everyone else; its browser reconnects
	// and resynchronizes.
	stauGrenze = 512
	// Maximum size of a single packet. In practice Yjs packages are a few
	// hundred bytes; one megabyte also covers the initial full synchronization
	// of a long page.
	groesstesPaket = 1 << 20
)

// `sitzer` represents an open browser in a room. Messages are written to the
// `post` channel rather than directly to the connection: otherwise multiple
// senders could write concurrently to the same stream.
type sitzer struct {
	post chan []byte
	uid  string
	// `once` protects against closing the channel twice: the dispatcher may
	// close it when backpressure becomes too high and the departure routine
	// closes it when the browser leaves. Closing a channel twice panics.
	einmal sync.Once
}

func (s *sitzer) schliessen() {
	s.einmal.Do(func() { close(s.post) })
}

type raum struct {
	sync.Mutex
	leute map[*sitzer]bool
}

// `raeume` holds the active sessions. Kept in-memory, not in the database:
// the content of a room is only useful while someone is present.
var raeume = struct {
	sync.Mutex
	nach map[string]*raum
}{nach: map[string]*raum{}}

func betreten(seite string, s *sitzer) bool {
	raeume.Lock()
	r := raeume.nach[seite]
	if r == nil {
		r = &raum{leute: map[*sitzer]bool{}}
		raeume.nach[seite] = r
	}
	raeume.Unlock()

	r.Lock()
	defer r.Unlock()
	if len(r.leute) >= hoechstensImRaum {
		return false
	}
	r.leute[s] = true
	return true
}

func verlassen(seite string, s *sitzer) {
	raeume.Lock()
	r := raeume.nach[seite]
	raeume.Unlock()
	if r == nil {
		return
	}
	r.Lock()
	delete(r.leute, s)
	leer := len(r.leute) == 0
	r.Unlock()

	// The empty room is removed to avoid the map growing with every page
	// ever opened and never shrinking again.
	if leer {
		raeume.Lock()
		if r2 := raeume.nach[seite]; r2 == r {
			r2.Lock()
			if len(r2.leute) == 0 {
				delete(raeume.nach, seite)
			}
			r2.Unlock()
		}
		raeume.Unlock()
	}
}

func verteilen(seite string, paket []byte) {
	raeume.Lock()
	r := raeume.nach[seite]
	raeume.Unlock()
	if r == nil {
		return
	}
	r.Lock()
	defer r.Unlock()
	for s := range r.leute {
		select {
		case s.post <- paket:
		default:
				// Full: the channel is closed, the writer ends and closes the
				// connection.
			s.schliessen()
			delete(r.leute, s)
		}
	}
}

// ImRaum returns how many participants are currently on a page. Used for the
// sharing UI.
func ImRaum(seite string) int {
	raeume.Lock()
	r := raeume.nach[seite]
	raeume.Unlock()
	if r == nil {
		return 0
	}
	r.Lock()
	defer r.Unlock()
	return len(r.leute)
}

var errFremdeHerkunft = errors.New("fremde Herkunft")

// `Mitschreibende` reports how many participants are currently on a page.
// The share dialog displays this so a user can see if someone is present
// before changing permissions.
func (s *Server) Mitschreibende(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"anzahl":   ImRaum(id),
		"moeglich": s.echtzeitAn(),
	})
}

// offeneRaeume returns, for every page with active participants, the IDs of
// those participants. A copy is returned rather than a view of the original
// to avoid exposing internal state and to keep locks brief.
func offeneRaeume() map[string][]string {
	raeume.Lock()
	kopie := make(map[string]*raum, len(raeume.nach))
	for seite, r := range raeume.nach {
		kopie[seite] = r
	}
	raeume.Unlock()

	raus := map[string][]string{}
	for seite, r := range kopie {
		r.Lock()
		for s := range r.leute {
			raus[seite] = append(raus[seite], s.uid)
		}
		r.Unlock()
		if len(raus[seite]) == 0 {
			delete(raus, seite)
		}
	}
	return raus
}

// MitschriftZustand reports current collaborative activity to administrators:
// which pages are open and who is editing them.
//
// Only live sessions are reported; historic edits and timestamps are stored
// in the audit trail and in versions. This endpoint concerns the present
// moment, which ends as soon as the last participant closes the tab.
func (s *Server) MitschriftZustand(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	offen := offeneRaeume()
	// Fetch names and titles in a single batch rather than in a loop: the
	// number of open rooms is small and we want to keep the number of
	// queries small as well.
	namen := map[string]string{}
	titel := map[string]string{}
	alleKonten := map[string]bool{}
	seiten := make([]string, 0, len(offen))
	for seite, konten := range offen {
		seiten = append(seiten, seite)
		for _, k := range konten {
			alleKonten[k] = true
		}
	}
	if len(seiten) > 0 {
		if rows, err := s.Pool.Query(r.Context(),
			`SELECT id::text, title FROM pages WHERE id = ANY($1)`, seiten); err == nil {
			for rows.Next() {
				var id, t string
				if err := rows.Scan(&id, &t); err == nil {
					titel[id] = t
				}
			}
			rows.Close()
		}
		liste := make([]string, 0, len(alleKonten))
		for k := range alleKonten {
			liste = append(liste, k)
		}
		if rows, err := s.Pool.Query(r.Context(),
			`SELECT id::text, name FROM users WHERE id = ANY($1)`, liste); err == nil {
			for rows.Next() {
				var id, n string
				if err := rows.Scan(&id, &n); err == nil {
					namen[id] = n
				}
			}
			rows.Close()
		}
	}

	art := make([]map[string]interface{}, 0, len(offen))
	for seite, konten := range offen {
		wer := make([]string, 0, len(konten))
		for _, k := range konten {
			if n := namen[k]; n != "" {
				wer = append(wer, n)
			}
		}
		sort.Strings(wer)
		t := titel[seite]
		if t == "" {
			t = "Ohne Titel"
		}
		art = append(art, map[string]interface{}{
			"seite":  seite,
			"titel":  t,
			"anzahl": len(konten),
			"wer":    wer,
		})
	}
	sort.Slice(art, func(i, j int) bool {
		return art[i]["titel"].(string) < art[j]["titel"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"an":         s.echtzeitAn(),
		"lizenziert": lizenz.Frei(lizenz.Echtzeit),
		"hoechstens": hoechstensImRaum,
		"raeume":     art,
	})
}

// `gleicheHerkunft` allows only connections from the same origin as the
// web UI.
//
// The session cookie is SameSite=Lax and is not sent from a foreign page;
// this is an additional safeguard. The check provided by x/net only requires
// a readable Origin header and performs a host comparison.
func gleicheHerkunft(_ *websocket.Config, req *http.Request) error {
	herkunft := req.Header.Get("Origin")
	if herkunft == "" {
		return errFremdeHerkunft
	}
	u, err := url.Parse(herkunft)
	if err != nil {
		return err
	}
	if !strings.EqualFold(u.Host, req.Host) {
		return errFremdeHerkunft
	}
	return nil
}

// `Mitschrift` is the channel for collaborative editing of a page.
func (s *Server) Mitschrift(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "read-only access")
		return
	}
	if !s.echtzeitAn() {
		writeErr(w, http.StatusForbidden, "gemeinsames Bearbeiten ist abgeschaltet")
		return
	}

	mich := &sitzer{post: make(chan []byte, stauGrenze), uid: uid}
	if !betreten(id, mich) {
		writeErr(w, http.StatusConflict, "an dieser Seite sitzen bereits genug Leute")
		return
	}

	leitung(id, mich).ServeHTTP(w, r)

	verlassen(id, mich)
	// Unregister before closing: the distributor no longer holds the channel
	// and a send cannot race with the closing.
	mich.schliessen()
}

// `leitung` is the open connection itself, without permission checks or
// database access. Extracted for testing: distributing over a real
// connection is the part most likely to fail silently.
func leitung(id string, mich *sitzer) http.Handler {
	return &websocket.Server{
		Handshake: gleicheHerkunft,
		Handler: func(c *websocket.Conn) {
			defer c.Close()
			c.MaxPayloadBytes = groesstesPaket

			// The writer runs concurrently so that reading does not block.
			fertig := make(chan struct{})
			go func() {
				defer close(fertig)
				for paket := range mich.post {
					if err := websocket.Message.Send(c, paket); err != nil {
						return
					}
				}
			}()

			for {
				var paket []byte
				// Do not use r.Context() here: the router sets a 30 second
				// timeout on it, while a session can last hours. The session
				// ends when the connection is closed.
				if err := websocket.Message.Receive(c, &paket); err != nil {
					return
				}
				if len(paket) == 0 {
					continue
				}
				verteilen(id, paket)
			}
		},
	}
}
