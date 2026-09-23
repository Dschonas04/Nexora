// The second factor: time-based one-time passwords after RFC 6238.
//
// Deliberately without a third-party library. The scheme is an HMAC over a
// counter plus one offset taken from it; that stands here in full and can
// therefore be read, instead of sitting in a dependency that would have to be
// maintained for thirty lines of code.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TOTPSchritt is the length of one time window. Thirty seconds is what every
// authenticator app assumes without asking; a different value would have to be
// entered in the app by hand and would therefore be no help at all.
const TOTPSchritt = 30 * time.Second

// TOTPToleranz is the number of windows accepted forwards and backwards. One
// means: the phone's clock may be off by half a minute. More only widens the
// window in which a code read over somebody's shoulder still fits.
const TOTPToleranz = 1

var base32Roh = base32.StdEncoding.WithPadding(base32.NoPadding)

// NeuesGeheimnis returns a fresh secret in Base32, the way an authenticator
// app expects it. Twenty bytes is the length HMAC-SHA1 folds its key down to
// anyway -- more would buy no security.
func NeuesGeheimnis() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32Roh.EncodeToString(b), nil
}

// TOTPCode computes the code for a point in time.
func TOTPCode(geheim string, wann time.Time) (string, error) {
	schluessel, err := base32Roh.DecodeString(strings.ToUpper(strings.TrimSpace(geheim)))
	if err != nil {
		return "", err
	}
	zaehler := uint64(wann.Unix()) / uint64(TOTPSchritt/time.Second)
	var block [8]byte
	binary.BigEndian.PutUint64(block[:], zaehler)

	m := hmac.New(sha1.New, schluessel)
	m.Write(block[:])
	summe := m.Sum(nil)

	// Dynamic truncation after RFC 4226: the last four bits point at the offset
	// where the four bytes sit that the code is made from.
	pos := summe[len(summe)-1] & 0x0f
	wert := binary.BigEndian.Uint32(summe[pos:pos+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", wert%1000000), nil
}

// TOTPPruefen says whether the entered code matches the secret. The comparison
// runs in constant time, and the neighbouring windows are checked as well so
// that a slightly wrong clock locks nobody out.
func TOTPPruefen(geheim, code string) bool {
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(code) != 6 {
		return false
	}
	jetzt := time.Now()
	passt := false
	for i := -TOTPToleranz; i <= TOTPToleranz; i++ {
		soll, err := TOTPCode(geheim, jetzt.Add(time.Duration(i)*TOTPSchritt))
		if err != nil {
			return false
		}
		// Do not break out on a match: the loop should take the same time for
		// every input.
		if subtle.ConstantTimeCompare([]byte(soll), []byte(code)) == 1 {
			passt = true
		}
	}
	return passt
}

// OtpauthURI is what the QR code contains. The issuer appears twice, as part
// of the path and as a parameter: older apps read the one, newer ones the
// other.
func OtpauthURI(aussteller, konto, geheim string) string {
	pfad := url.PathEscape(aussteller) + ":" + url.PathEscape(konto)
	q := url.Values{}
	q.Set("secret", geheim)
	q.Set("issuer", aussteller)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + pfad + "?" + q.Encode()
}

// NeuerErsatzcode returns a code for the case where the phone is gone. Ten
// characters from Base32 without the digits mistaken for letters, in two
// blocks -- it gets copied out by hand, not with a mouse.
func NeuerErsatzcode() (string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	aus := make([]byte, 0, 11)
	for i, x := range b {
		if i == 5 {
			aus = append(aus, '-')
		}
		aus = append(aus, alphabet[int(x)%len(alphabet)])
	}
	return string(aus), nil
}

// ---------------------------------------------------------------------------
// The secret in the database
//
// A TOTP secret cannot be hashed like a password: the server has to read it
// back in order to compute. It therefore sits in the column encrypted, with a
// key derived from the instance's signing secret. A dump of the database alone
// -- a backup, a lost archive -- thus yields no codes.

func totpSchluessel(secret []byte) []byte {
	h := sha256.Sum256(append([]byte("nexora-zweitfaktor:"), secret...))
	return h[:]
}

// GeheimVerschluesseln prepares the secret for the column: AES-GCM, the nonce
// in front, all of it in Base64.
func GeheimVerschluesseln(secret []byte, klar string) (string, error) {
	gcm, err := totpGCM(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(klar), nil)), nil
}

// GeheimEntschluesseln reads back what GeheimVerschluesseln stored.
func GeheimEntschluesseln(secret []byte, abgelegt string) (string, error) {
	roh, err := base64.StdEncoding.DecodeString(abgelegt)
	if err != nil {
		return "", err
	}
	gcm, err := totpGCM(secret)
	if err != nil {
		return "", err
	}
	if len(roh) < gcm.NonceSize() {
		return "", errors.New("Geheimnis unbrauchbar")
	}
	klar, err := gcm.Open(nil, roh[:gcm.NonceSize()], roh[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("Geheimnis unbrauchbar")
	}
	return string(klar), nil
}

func totpGCM(secret []byte) (cipher.AEAD, error) {
	c, err := aes.NewCipher(totpSchluessel(secret))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(c)
}

// ---------------------------------------------------------------------------
// The ticket between the two steps
//
// Between password and code nobody is signed in, and yet the second call has
// to know who this is about. It gets a short-lived, signed ticket for that
// instead of a session.
//
// It is signed with a separate key derived from the signing secret. That rules
// out a ticket ever passing as a session cookie: the session check computes
// with the other key and throws it away.

// ZweitTicketDauer is the deadline for the second step. Five minutes are
// enough to get a phone out of a pocket, and short enough that an intercepted
// ticket ages into worthlessness.
const ZweitTicketDauer = 5 * time.Minute

func ticketSchluessel(secret []byte) []byte {
	h := sha256.Sum256(append([]byte("nexora-zweitfaktor-ticket:"), secret...))
	return h[:]
}

// ZweitTicket signs the ticket for an account.
func ZweitTicket(secret []byte, userID string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ZweitTicketDauer)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString(ticketSchluessel(secret))
}

// ZweitTicketLesen returns the account the ticket was issued for.
func ZweitTicketLesen(secret []byte, ticket string) (string, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(ticket, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return ticketSchluessel(secret), nil
	})
	if err != nil || !tok.Valid || claims.UserID == "" {
		return "", errors.New("Der zweite Schritt ist abgelaufen. Bitte melde dich neu an.")
	}
	return claims.UserID, nil
}
