// Password management: changing one's own password (with the old password)
// and resetting it as an administrator.
//
// Both routes write to the same column but differ in what they require and
// how they treat sessions afterwards. Changing your own password keeps you
// logged in on this device and logs you out everywhere else — the purpose of
// a change when an untrusted device was involved. When an administrator sets
// a password, the account is logged out everywhere, including any session
// currently active on it.
package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/auth"
	"nexora/internal/middleware"
)

const (
	// mindestPasswort matches the same lower bound as registration. Two
	// different minima would be worse than a low one: a password might pass
	// registration but be rejected on the next change.
	mindestPasswort = 6
	// hoechstensPasswort because bcrypt only reads the first 72 bytes. A
	// longer password would be silently truncated and then the phrase someone
	// remembered would no longer match. Better to reject it here than to
	// truncate unnoticed.
	hoechstensPasswort = 72
)

// passwortPruefen reports whether a new password is acceptable.
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

// ssoHerkunft detects an account that authenticates via an external SSO
// provider. Instead of a hash the column contains "sso:<provider>", see
// sso.go.
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

// PasswortWechseln changes the password of the currently authenticated
// account.
//
// The old password is required even though the session already proved who
// is present. The check protects against the case of an unattended device:
// without it an open browser would be enough to take over the account.
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
		// The failed attempt is recorded in the audit trail: someone sitting at
		// an open browser trying to guess the password would start here.
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

	// All other sessions are terminated. A changed password that leaves a
	// foreign device still logged in defeats the purpose of the change.
	beendet := s.sitzungenWiderrufen(r.Context(), uid, middleware.SitzungID(r))
	s.spurAusRequest(r, AktPasswortGeaendert, "konto", uid, "",
		map[string]any{"sitzungen_beendet": beendet})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "beendet": beendet})
}

type passwortSetzenReq struct {
	Neu string `json:"neu"`
}

// PasswortSetzen is the administrator route: set a new password for another
// account without knowing the previous one.
//
// The administrator may not use this on their own account; the other route is
// used instead. Not out of caution but because this route terminates all
// sessions: an administrator who locks themselves out while still at the
// screen gains nothing.
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
	// Setting a password on an SSO account would remove its SSO login: the
	// logic in sso.go would no longer recognize it and would deny access.
	// The account would then only be reachable via that one password, which
	// is unlikely to be the intended effect.
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

	// This revokes everything, including any session currently active on the
	// account. A reset password usually means the access was compromised.
	beendet := s.sitzungenWiderrufen(r.Context(), ziel, "")
	s.spurAusRequest(r, AktPasswortGesetzt, "konto", ziel, mail,
		map[string]any{"sitzungen_beendet": beendet})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "beendet": beendet})
}
