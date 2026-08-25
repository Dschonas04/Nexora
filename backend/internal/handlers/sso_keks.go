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

// Der Zwischenstand einer begonnenen OIDC-Anmeldung reist in einem eigenen
// Plätzchen mit, unterschrieben mit demselben Geheimnis wie die Sitzung.
//
// Warum nicht im Speicher des Dienstes: eine begonnene Anmeldung überlebte dann
// keinen Neustart, und zwei Instanzen hinter einem Verteiler könnten sich nicht
// abwechseln. Warum unterschrieben: der Zustand ist genau das, was eine
// untergeschobene Anmeldung verhindern soll, ein Wert, den der Browser frei
// setzen könnte, wäre wertlos.

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
		// Lax reicht nicht: der Rücksprung kommt als Weiterleitung von einer
		// fremden Seite, und bei Strict schickte der Browser das Plätzchen
		// dann nicht mit.
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

// unterschrift ist ein HMAC über die Daten, gekürzt auf die üblichen 32 Zeichen.
// Dasselbe Geheimnis wie bei der Sitzung: ein zweites einzuführen hieße, einen
// zweiten Wert zu verwalten, der genauso geheim bleiben muss.
func (s *Server) unterschrift(daten []byte) string {
	m := hmac.New(sha256.New, s.Secret)
	m.Write(daten)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
