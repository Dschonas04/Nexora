// Package middleware contains the request filters shared by the API routes.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"nexora/internal/auth"
)

// ctxKey is a private type so no other package can collide with our context key
// by accident.
type ctxKey string

const userKey ctxKey = "userID"
const sitzungKey ctxKey = "sitzungID"
const schluesselKey ctxKey = "apiSchluessel"

// SchluesselPruefer resolves an API key to its account and whether it may
// write. Set once by main; nil means API keys are not accepted at all.
var SchluesselPruefer func(ctx context.Context, schluessel string) (uid string, schreiben bool, ok bool)

// bearer returns the key from "Authorization: Bearer <key>", or "".
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// UeberSchluessel reports whether the request came in with an API key rather
// than a browser session.
func UeberSchluessel(r *http.Request) bool {
	v, _ := r.Context().Value(schluesselKey).(bool)
	return v
}

// NurSitzung refuses API keys. For what only the person at the browser should
// do: change the password or the second factor, end sessions, make or revoke
// keys -- a leaked key must not be able to mint the next one.
func NurSitzung(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UeberSchluessel(r) {
			http.Error(w, `{"error":"not available with an API key"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Auth validates the JWT cookie and injects the user id into the request
// context. It answers 401 for a missing as well as an invalid token, so an
// attacker learns nothing from the difference. Authorisation, meaning who may
// touch which page, is decided later in the handlers.
// SitzungPruefer says whether a stored session is still valid. The middleware
// is handed one instead of knowing a database itself, so it stays testable, and
// where the answer comes from (database, cache) is the caller's decision.
type SitzungPruefer func(r *http.Request, w http.ResponseWriter, uid, sid string) bool

// Auth validates the JWT cookie and injects the user id into the request
// context. It answers 401 for a missing as well as an invalid token, so an
// attacker learns nothing from the difference. Authorisation, meaning who may
// touch which page, is decided later in the handlers.
//
// A second stage since sessions are stored: the token says who it was, the
// session says whether it should still count. Without that separation a token
// would stay usable after logging out, signed as it is.
func Auth(secret []byte, pruefe SitzungPruefer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// An API key from a script comes in the Authorization header; the
			// browser never sends one, it has the cookie.
			if k := bearer(r); k != "" && SchluesselPruefer != nil {
				uid, schreiben, ok := SchluesselPruefer(r.Context(), k)
				if !ok {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				if !schreiben && r.Method != http.MethodGet && r.Method != http.MethodHead {
					http.Error(w, `{"error":"this API key may only read"}`, http.StatusForbidden)
					return
				}
				ctx := context.WithValue(r.Context(), userKey, uid)
				ctx = context.WithValue(ctx, schluesselKey, true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// httpOnly cookie rather than an Authorization header: script on the
			// page cannot read it, which limits the damage of an XSS bug.
			c, err := r.Cookie("nexora_token")
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			uid, sid, err := auth.ParseToken(secret, c.Value)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if pruefe != nil && !pruefe(r, w, uid, sid) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userKey, uid)
			if sid != "" {
				ctx = context.WithValue(ctx, sitzungKey, sid)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SitzungID returns the session the request was authenticated with. Empty for
// a token from before sessions were stored.
func SitzungID(r *http.Request) string {
	if v, ok := r.Context().Value(sitzungKey).(string); ok {
		return v
	}
	return ""
}

// UserID returns the authenticated user id set by the Auth middleware.
func UserID(r *http.Request) string {
	if v, ok := r.Context().Value(userKey).(string); ok {
		return v
	}
	return ""
}
