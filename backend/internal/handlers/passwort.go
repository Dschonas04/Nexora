// Das Passwort wechseln: selbst, mit dem bisherigen als Nachweis, und das
// Zurücksetzen durch eine Verwaltung.
//
// Beide Wege schreiben in dieselbe Spalte und unterscheiden sich in dem, was
// sie verlangen und was sie hinterher mit den Sitzungen tun. Wer sein eigenes
// Passwort wechselt, bleibt an diesem Gerät angemeldet und wird überall sonst
// abgemeldet: das ist der Sinn des Wechsels, wenn ein fremdes Gerät im Spiel
// war. Setzt eine Verwaltung ein Passwort, fliegt das Konto überall heraus,
// auch dort, wo gerade jemand daran sitzt.
package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/auth"
	"nexora/internal/middleware"
)

const (
	// mindestPasswort ist dieselbe Untergrenze wie bei der Anmeldung. Zwei
	// verschiedene Grenzen wären schlimmer als eine niedrige: dann ginge ein
	// Passwort durch die Registrierung und beim nächsten Wechsel nicht mehr.
	mindestPasswort = 6
	// hoechstensPasswort, weil bcrypt nur die ersten 72 Bytes liest. Ein
	// längeres Passwort wird stillschweigend abgeschnitten, und dann stimmt der
	// Satz nicht mehr, den jemand sich gemerkt hat. Lieber hier ablehnen als
	// unbemerkt kürzen.
	hoechstensPasswort = 72
)

// passwortPruefen sagt, ob ein neues Passwort taugt.
func passwortPruefen(neu string) string {
	if len([]rune(neu)) < mindestPasswort {
		return "das Passwort braucht mindestens 6 Zeichen"
	}
	if len(neu) > hoechstensPasswort {
		return "das Passwort ist zu lang, mehr als 72 Zeichen liest die Prüfung nicht"
	}
	if strings.TrimSpace(neu) == "" {
		return "das Passwort besteht nur aus Leerzeichen"
	}
	return ""
}

// ssoHerkunft erkennt ein Konto, das sich über einen fremden Dienst anmeldet.
// Statt eines Hashs steht dort "sso:<herkunft>", siehe sso.go.
func ssoHerkunft(hash string) (string, bool) {
	if !strings.HasPrefix(hash, "sso:") {
		return "", false
	}
	return strings.TrimPrefix(hash, "sso:"), true
}

type passwortWechselReq struct {
	Alt string `json:"alt"`
	Neu string `json:"neu"`
}

// PasswortWechseln ändert das Passwort des angemeldeten Kontos.
//
// Das bisherige Passwort wird verlangt, obwohl die Sitzung bereits bewiesen
// hat, wer da sitzt. Der Nachweis gilt einem anderen Fall: ein Gerät, das
// jemand kurz unbeaufsichtigt lässt. Ohne die Abfrage wäre ein offener Browser
// genug, um das Konto zu übernehmen.
func (s *Server) PasswortWechseln(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	var req passwortWechselReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	var hash string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, uid).Scan(&hash); err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if herkunft, ja := ssoHerkunft(hash); ja {
		writeErr(w, http.StatusConflict,
			"dieses Konto meldet sich über "+herkunft+" an und hat hier kein Passwort")
		return
	}
	if !auth.CheckPassword(hash, req.Alt) {
		// Der Fehlversuch gehört ins Protokoll: wer fremd an einem offenen
		// Browser sitzt und das Passwort raten will, fängt genau hier an.
		s.spurAusRequest(r, AktPasswortFehl, "konto", uid, "", nil)
		writeErr(w, http.StatusForbidden, "das bisherige Passwort stimmt nicht")
		return
	}
	if meldung := passwortPruefen(req.Neu); meldung != "" {
		writeErr(w, http.StatusBadRequest, meldung)
		return
	}
	if req.Neu == req.Alt {
		writeErr(w, http.StatusBadRequest, "das neue Passwort ist das bisherige")
		return
	}

	neu, err := auth.HashPassword(req.Neu)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $2 WHERE id = $1`, uid, neu); err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Alle anderen Sitzungen fallen. Ein gewechseltes Passwort, nach dem ein
	// fremdes Gerät weiter angemeldet bleibt, wechselt nichts.
	beendet := s.sitzungenWiderrufen(r.Context(), uid, middleware.SitzungID(r))
	s.spurAusRequest(r, AktPasswortGeaendert, "konto", uid, "",
		map[string]any{"sitzungen_beendet": beendet})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "beendet": beendet})
}

type passwortSetzenReq struct {
	Neu string `json:"neu"`
}

// PasswortSetzen ist der Weg der Verwaltung: ein neues Passwort für ein fremdes
// Konto, ohne das bisherige zu kennen.
//
// Das eigene Konto ist ausgenommen und wird auf den anderen Weg verwiesen. Nicht
// aus Vorsicht, sondern weil dieser hier alle Sitzungen beendet: eine Verwaltung,
// die sich damit selbst aussperrt, während sie noch am Bildschirm sitzt, hat
// nichts gewonnen.
func (s *Server) PasswortSetzen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	ziel := chi.URLParam(r, "id")
	if ziel == uid {
		writeErr(w, http.StatusBadRequest,
			"das eigene Passwort wechselt man mit dem bisherigen, nicht hierüber")
		return
	}

	var req passwortSetzenReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if meldung := passwortPruefen(req.Neu); meldung != "" {
		writeErr(w, http.StatusBadRequest, meldung)
		return
	}

	var hash, mail string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT password_hash, email FROM users WHERE id = $1`, ziel).Scan(&hash, &mail); err != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	// Ein Passwort auf ein SSO-Konto zu setzen, nähme ihm die Anmeldung: die
	// Übernahme in sso.go erkennt es danach nicht mehr wieder und verweigert
	// den Zugang. Das Konto wäre danach nur noch über dieses eine Passwort
	// erreichbar, und niemand hätte es so gemeint.
	if herkunft, ja := ssoHerkunft(hash); ja {
		writeErr(w, http.StatusConflict,
			"dieses Konto meldet sich über "+herkunft+" an, ein Passwort würde ihm den Zugang nehmen")
		return
	}

	neu, err := auth.HashPassword(req.Neu)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $2 WHERE id = $1`, ziel, neu); err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Hier fällt alles, auch die Sitzung, an der das Konto gerade sitzt. Ein
	// zurückgesetztes Passwort heißt in aller Regel: der Zugang ist unsicher.
	beendet := s.sitzungenWiderrufen(r.Context(), ziel, "")
	s.spurAusRequest(r, AktPasswortGesetzt, "konto", ziel, mail,
		map[string]any{"sitzungen_beendet": beendet})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "beendet": beendet})
}
