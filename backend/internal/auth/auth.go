// Package auth holds the two credential primitives the API needs: bcrypt
// password hashing and signed JWTs for the session cookie.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plaintext password with bcrypt. Cost 12 is a deliberate
// step above the library default: it costs a few hundred milliseconds per login,
// which is unnoticeable for a person and expensive for an attacker.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	return string(b), err
}

// CheckPassword reports whether pw matches the stored hash. The comparison is
// constant time, so it leaks nothing about how far it got before failing.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Claims is the JWT payload. It carries the user id only, so any change to a
// user's role or password takes effect on the next login rather than instantly
// invalidating tokens already handed out.
type Claims struct {
	UserID string `json:"uid"`
	// SitzungID verweist auf die Zeile in der Tabelle sitzungen. Ohne sie wäre
	// ein Token bis zum Ablauf gültig, egal was danach passiert, Abmelden,
	// Passwortwechsel, ein verlorenes Gerät. Mit ihr entscheidet die Datenbank
	// bei jeder Anfrage mit.
	SitzungID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// GenerateToken signs a session token for userID that expires after ttl.
//
// sitzungID darf leer sein, dann ist das Token wie früher rein rechnerisch
// gültig. Genutzt wird das nirgends mehr; die Möglichkeit bleibt, damit ein
// altes Token aus einer Sitzung vor dieser Änderung nicht schlagartig ungültig
// wird.
func GenerateToken(secret []byte, userID, sitzungID string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:    userID,
		SitzungID: sitzungID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ParseToken verifies a token and returns the user id and session it was issued
// for.
// Every failure returns the same opaque error so a caller cannot tell an expired
// token from a forged one.
func ParseToken(secret []byte, tokenStr string) (string, string, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		// Pin the algorithm. Without this check a token could claim "none" or
		// swap in RS256 and have the public key treated as the HMAC secret.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return "", "", errors.New("invalid token")
	}
	return claims.UserID, claims.SitzungID, nil
}
