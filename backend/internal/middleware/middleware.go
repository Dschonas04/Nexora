// Package middleware contains the request filters shared by the API routes.
package middleware

import (
	"context"
	"net/http"

	"nexora/internal/auth"
)

// ctxKey is a private type so no other package can collide with our context key
// by accident.
type ctxKey string

const userKey ctxKey = "userID"

// Auth validates the JWT cookie and injects the user id into the request
// context. It answers 401 for a missing as well as an invalid token, so an
// attacker learns nothing from the difference. Authorisation, meaning who may
// touch which page, is decided later in the handlers.
func Auth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// httpOnly cookie rather than an Authorization header: script on the
			// page cannot read it, which limits the damage of an XSS bug.
			c, err := r.Cookie("nexora_token")
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			uid, err := auth.ParseToken(secret, c.Value)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userKey, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserID returns the authenticated user id set by the Auth middleware.
func UserID(r *http.Request) string {
	if v, ok := r.Context().Value(userKey).(string); ok {
		return v
	}
	return ""
}
