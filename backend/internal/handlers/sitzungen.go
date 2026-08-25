// Stored sessions.
//
// A session used to be nothing but a signed token: valid until its time ran
// out, and stoppable by nothing. Signing out only dropped the cookie in the
// browser, so whoever had copied the token beforehand stayed signed in. A lost
// laptop could not be locked out without taking everybody else's access away.
//
// Now every session is a row in the database and the token merely points at it.
// Three things follow: a single session can be ended, the list answers who is
// currently signed in, and a session that is in use renews itself.
//
// The price is one query per request. It is cheap, a primary key lookup, and it
// is cached on top of that, see sitzungsspeicher.go.
package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/auth"
	"nexora/internal/middleware"
)

// sitzungsTakt is the interval between two sweeps.
const sitzungsTakt = 6 * time.Hour

// aufgefrischtAb: a session is only extended once it is this far along.
// Extending on every request would mean writing on every request, a lot of load
// for nothing.
const aufgefrischtAb = 0.5

// benutztSpanne limits how often "last used" is written back. Without it every
// single request would write to the row.
const benutztSpanne = 5 * time.Minute

// Sitzung is one row as the interface shows it.
type Sitzung struct {
	ID         string    `json:"id"`
	AngelegtAm time.Time `json:"angelegtAm"`
	ZuletztAm  time.Time `json:"zuletztAm"`
	LaeuftAb   time.Time `json:"laeuftAb"`
	IP         string    `json:"ip"`
	Browser    string    `json:"browser"`
	// True for the session the request came in on, so the list can say "this
	// device" and nobody ends their own by accident.
	Diese bool `json:"diese"`
}

// sitzungAnlegen writes the row and returns its id.
func (s *Server) sitzungAnlegen(ctx context.Context, r *http.Request, userID string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO sitzungen (user_id, laeuft_ab, ip, browser)
		 VALUES ($1, now() + make_interval(hours => $2), $3, $4)
		 RETURNING id`,
		userID, SitzungStunden(), absenderIP(r), kurzerBrowser(r.UserAgent())).Scan(&id)
	return id, err
}

// SitzungGilt checks a session and renews it when due. This function is what
// the middleware calls.
func (s *Server) SitzungGilt(r *http.Request, w http.ResponseWriter, uid, sid string) bool {
	// A token without a session id predates this change. It stays valid until it
	// expires: signing everyone out at once would be needless harshness for a
	// change nobody is meant to notice.
	if sid == "" {
		return true
	}

	if gilt, ok := s.sitzungAusSpeicher(sid); ok {
		if !gilt {
			return false
		}
		// No renewal from the cache: that happens on the next pass through the
		// database.
		return true
	}

	var besitzer string
	var angelegt, laeuftAb, zuletzt time.Time
	var widerrufen *time.Time
	err := s.Pool.QueryRow(r.Context(),
		`SELECT user_id, angelegt_am, zuletzt_am, laeuft_ab, widerrufen_am
		 FROM sitzungen WHERE id=$1`, sid).
		Scan(&besitzer, &angelegt, &zuletzt, &laeuftAb, &widerrufen)
	if err != nil || besitzer != uid || widerrufen != nil || time.Now().After(laeuftAb) {
		s.sitzungMerken(sid, false)
		return false
	}

	jetzt := time.Now()

	// Renewal: once a session has more than half of its time behind it, it gets
	// a fresh span and a fresh cookie. Someone working daily is never signed
	// out; someone away for weeks is.
	gesamt := laeuftAb.Sub(angelegt)
	if gesamt > 0 && jetzt.Sub(angelegt) > time.Duration(float64(gesamt)*aufgefrischtAb) {
		neuAb := jetzt.Add(SitzungDauer())
		if _, err := s.Pool.Exec(r.Context(),
			`UPDATE sitzungen SET laeuft_ab=$2, angelegt_am=now(), zuletzt_am=now() WHERE id=$1`,
			sid, neuAb); err == nil {
			if token, err := auth.GenerateToken(s.Secret, uid, sid, SitzungDauer()); err == nil {
				s.setAuthCookieFuer(w, r, token)
			}
		}
	} else if jetzt.Sub(zuletzt) > benutztSpanne {
		s.Pool.Exec(r.Context(), `UPDATE sitzungen SET zuletzt_am=now() WHERE id=$1`, sid)
	}

	s.sitzungMerken(sid, true)
	return true
}

// ListSitzungen shows the account's own sessions.
func (s *Server) ListSitzungen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	diese := middleware.SitzungID(r)

	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, angelegt_am, zuletzt_am, laeuft_ab, ip, browser
		 FROM sitzungen
		 WHERE user_id=$1 AND widerrufen_am IS NULL AND laeuft_ab > now()
		 ORDER BY zuletzt_am DESC`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	liste := []Sitzung{}
	for rows.Next() {
		var si Sitzung
		if err := rows.Scan(&si.ID, &si.AngelegtAm, &si.ZuletztAm, &si.LaeuftAb,
			&si.IP, &si.Browser); err == nil {
			si.Diese = si.ID == diese
			liste = append(liste, si)
		}
	}
	writeJSON(w, http.StatusOK, liste)
}

// SitzungBeenden revokes a single session.
func (s *Server) SitzungBeenden(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	// user_id is part of the condition: otherwise a stranger's id would sign
	// somebody else out.
	tag, err := s.Pool.Exec(r.Context(),
		`UPDATE sitzungen SET widerrufen_am=now()
		 WHERE id=$1 AND user_id=$2 AND widerrufen_am IS NULL`, id, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht beendet werden")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "Sitzung nicht gefunden")
		return
	}
	s.sitzungMerken(id, false)
	s.spurAusRequest(r, AktSitzungBeendet, "sitzung", id, "", nil)

	// Ending your own session is allowed, and then the cookie has to go too,
	// or the browser keeps sending a token that is valid nowhere.
	if id == middleware.SitzungID(r) {
		s.clearAuthCookie(w)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// SitzungenBeenden revokes every session except the one in use.
func (s *Server) SitzungenBeenden(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	diese := middleware.SitzungID(r)

	rows, err := s.Pool.Query(r.Context(),
		`UPDATE sitzungen SET widerrufen_am=now()
		 WHERE user_id=$1 AND widerrufen_am IS NULL AND id <> $2
		 RETURNING id`, uid, diese)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht beendet werden")
		return
	}
	defer rows.Close()
	anzahl := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			s.sitzungMerken(id, false)
			anzahl++
		}
	}
	s.spurAusRequest(r, AktSitzungBeendet, "konto", uid, "",
		map[string]any{"beendet": anzahl, "umfang": "alle anderen"})
	writeJSON(w, http.StatusOK, map[string]int{"beendet": anzahl})
}

// SitzungenUhr sweeps away expired and revoked sessions.
//
// Not right after they expire: a row that stays for a few more days answers the
// question "when was I last signed in, and from where", and that is exactly
// what gets asked after an incident.
func (s *Server) SitzungenUhr(ctx context.Context) {
	uhr := time.NewTicker(sitzungsTakt)
	defer uhr.Stop()
	for {
		tag, err := s.Pool.Exec(ctx,
			`DELETE FROM sitzungen
			 WHERE laeuft_ab < now() - interval '30 days'
			    OR (widerrufen_am IS NOT NULL AND widerrufen_am < now() - interval '30 days')`)
		if err != nil {
			log.Printf("Sitzungen aufräumen: %v", err)
		} else if n := tag.RowsAffected(); n > 0 {
			log.Printf("Sitzungen: %d alte Zeilen entfernt", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-uhr.C:
		}
	}
}

// SitzungenEinesKontos revokes everything belonging to one account. Needed when
// a password changes and when an administrator locks somebody out.
func (s *Server) SitzungenEinesKontos(ctx context.Context, uid string) {
	rows, err := s.Pool.Query(ctx,
		`UPDATE sitzungen SET widerrufen_am=now()
		 WHERE user_id=$1 AND widerrufen_am IS NULL RETURNING id`, uid)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			s.sitzungMerken(id, false)
		}
	}
}

// kurzerBrowser turns a user agent string into something readable.
//
// The full string is a paragraph of historical baggage ("Mozilla/5.0" appears in
// every browser, including those that never were Mozilla). For the question "was
// that me?" the program and the operating system are enough.
func kurzerBrowser(ua string) string {
	if ua == "" {
		return "unbekannt"
	}
	var programm, system string
	switch {
	case strings.Contains(ua, "Firefox/"):
		programm = "Firefox"
	case strings.Contains(ua, "Edg/"):
		programm = "Edge"
	case strings.Contains(ua, "Chrome/"):
		programm = "Chrome"
	case strings.Contains(ua, "Safari/"):
		programm = "Safari"
	default:
		programm = "anderer Browser"
	}
	switch {
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		system = "iOS"
	case strings.Contains(ua, "Mac OS"):
		system = "macOS"
	case strings.Contains(ua, "Linux"):
		system = "Linux"
	}
	if system == "" {
		return programm
	}
	return programm + " auf " + system
}
