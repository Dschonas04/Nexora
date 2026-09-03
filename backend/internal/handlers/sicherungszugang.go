// Who is allowed to fetch backups.
//
// A logged-in administrator can fetch from the panel: the browser sends the
// session cookie. A script does not have a cookie, and automation is exactly
// the use case here. Therefore a second way in is required: a shared token.
//
// THE BACKUP TOKEN IS MORE SENSITIVE THAN THE METRICS TOKEN. The latter
// exposes a summary, this one exposes the entire dataset including password
// hashes, sessions and share tokens. It is therefore separate: configuring
// a metrics exporter should not also distribute a key to the entire
// database. Every retrieval is recorded in the audit trail with its source
// address.
package handlers

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"nexora/internal/middleware"
)

type zugangsArt string

const perTokenSchluessel zugangsArt = "sicherung.perToken"

// `SicherungToken` is the passphrase for retrieval without a session. Empty
// means this path is disabled and only the login-based access remains.
func SicherungToken() string { return wert("sicherung_token") }

// SicherungZugang allows either a valid token or a logged-in session.
//
// Composed rather than duplicated: exposing a second endpoint with its own
// authorization would create a second place to manage rights and would
// diverge over time.
func SicherungZugang(secret []byte, pruefe middleware.SitzungPruefer) func(http.Handler) http.Handler {
	ueberSitzung := middleware.Auth(secret, pruefe)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := SicherungToken(); token != "" {
				angeboten := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				// Constant-time comparison so the token cannot be guessed one
				// character at a time.
				if angeboten != "" &&
					subtle.ConstantTimeCompare([]byte(angeboten), []byte(token)) == 1 {
					r = r.WithContext(context.WithValue(r.Context(), perTokenSchluessel, true))
					next.ServeHTTP(w, r)
					return
				}
			}
			ueberSitzung(next).ServeHTTP(w, r)
		})
	}
}

// perToken reports whether this request arrived using the backup token.
func perToken(r *http.Request) bool {
	wert, _ := r.Context().Value(perTokenSchluessel).(bool)
	return wert
}
