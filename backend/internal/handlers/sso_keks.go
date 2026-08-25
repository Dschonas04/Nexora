package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// The half-finished state of an OIDC sign-in travels in a cookie of its own,
// signed with the same secret as the session.
//
// Why not in the service's memory: a sign-in already under way would then not
// survive a restart, and two instances behind a load balancer could not take
// turns. Why signed: the state parameter is precisely what stops a smuggled-in
// sign-in, and a value the browser could set freely would be worthless.

func (s *Server) oidcKeksSetzen(w http.ResponseWriter, r *http.Request, st oidcSitzung) {
	roh, err := json.Marshal(st)
	if err != nil {
		return
	}
	wert := base64.RawURLEncoding.EncodeToString(roh) + "." + s.unterschrift(roh)
	http.SetCookie(w, &http.Cookie{
		Name:     oidcKeksName,
		Value:    wert,
		Path:     "/",
		HttpOnly: true,
		Secure:   ueberTLS(r),
		// Lax is required here: the callback arrives as a redirect from another
		// site, and under Strict the browser would not send the cookie along.
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
}

func (s *Server) oidcKeksLesen(r *http.Request) (oidcSitzung, error) {
	var st oidcSitzung
	c, err := r.Cookie(oidcKeksName)
	if err != nil {
		return st, errors.New("kein Zwischenstand")
	}
	teile := splitZwei(c.Value)
	if len(teile) != 2 {
		return st, errors.New("Zwischenstand unlesbar")
	}
	roh, err := base64.RawURLEncoding.DecodeString(teile[0])
	if err != nil {
		return st, errors.New("Zwischenstand unlesbar")
	}
	if s.unterschrift(roh) != teile[1] {
		return st, errors.New("Zwischenstand verändert")
	}
	if err := json.Unmarshal(roh, &st); err != nil {
		return st, errors.New("Zwischenstand unlesbar")
	}
	if time.Now().After(st.Bis) {
		return st, errors.New("Zwischenstand abgelaufen")
	}
	return st, nil
}

func (s *Server) oidcKeksLoeschen(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcKeksName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   ueberTLS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func splitZwei(s string) []string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// unterschrift is an HMAC over the data. It uses the same secret as the
// session: introducing a second one would mean keeping a second value that has
// to stay just as secret.
func (s *Server) unterschrift(daten []byte) string {
	m := hmac.New(sha256.New, s.Secret)
	m.Write(daten)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
