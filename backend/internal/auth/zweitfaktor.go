// Der zweite Faktor: zeitbasierte Einmalkennwörter nach RFC 6238.
//
// Absichtlich ohne fremde Bibliothek. Das Verfahren ist ein HMAC über einen
// Zähler und eine Stelle daraus; das steht hier vollständig und ist damit
// nachlesbar, statt in einer Abhängigkeit zu liegen, die für dreißig Zeilen
// gepflegt werden müsste.
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

// TOTPSchritt ist die Länge eines Zeitfensters. Dreißig Sekunden sind das, was
// jede Authenticator-App ohne Rückfrage annimmt; ein anderer Wert müsste in der
// App von Hand eingetragen werden und wäre damit keine Erleichterung.
const TOTPSchritt = 30 * time.Second

// TOTPToleranz ist die Zahl der Fenster, die nach vorn und nach hinten gelten.
// Eins bedeutet: die Uhr des Telefons darf eine halbe Minute falsch gehen. Mehr
// vergrößert nur das Fenster, in dem ein abgelesener Code noch passt.
const TOTPToleranz = 1

var base32Roh = base32.StdEncoding.WithPadding(base32.NoPadding)

// NeuesGeheimnis liefert ein frisches Geheimnis in Base32, so wie eine
// Authenticator-App es erwartet. Zwanzig Byte sind die Länge, auf die HMAC-SHA1
// seinen Schlüssel ohnehin zusammenfaltet -- mehr brächte keine Sicherheit.
func NeuesGeheimnis() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32Roh.EncodeToString(b), nil
}

// TOTPCode rechnet den Code für einen Zeitpunkt aus.
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

	// Dynamic truncation nach RFC 4226: die letzten vier Bit zeigen auf die
	// Stelle, an der die vier Bytes stehen, aus denen der Code entsteht.
	pos := summe[len(summe)-1] & 0x0f
	wert := binary.BigEndian.Uint32(summe[pos:pos+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", wert%1000000), nil
}

// TOTPPruefen sagt, ob der eingegebene Code zum Geheimnis passt. Verglichen wird
// in gleichbleibender Zeit, und es werden die Nachbarfenster mitgeprüft, damit
// eine leicht falsch gehende Uhr niemanden aussperrt.
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
		// Nicht abbrechen, wenn es passt: die Schleife soll für jede Eingabe
		// gleich lange laufen.
		if subtle.ConstantTimeCompare([]byte(soll), []byte(code)) == 1 {
			passt = true
		}
	}
	return passt
}

// OtpauthURI ist das, was im QR-Code steht. Der Aussteller taucht zweimal auf,
// als Pfad und als Parameter: ältere Apps lesen das eine, neuere das andere.
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

// NeuerErsatzcode liefert einen Code für den Fall, dass das Telefon weg ist.
// Zehn Zeichen aus Base32 ohne die Ziffern, die für Buchstaben gehalten werden,
// in zwei Blöcken -- er wird abgeschrieben und nicht kopiert.
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
// Das Geheimnis in der Datenbank
//
// Ein TOTP-Geheimnis lässt sich nicht wie ein Passwort hashen: der Server muss
// es zurücklesen können, um zu rechnen. Es liegt deshalb verschlüsselt in der
// Spalte, mit einem Schlüssel, der aus dem Signaturgeheimnis der Instanz kommt.
// Ein Abzug der Datenbank allein -- eine Sicherung, ein verlorenes Backup --
// gibt damit keine Codes her.

func totpSchluessel(secret []byte) []byte {
	h := sha256.Sum256(append([]byte("nexora-zweitfaktor:"), secret...))
	return h[:]
}

// GeheimVerschluesseln legt das Geheimnis für die Spalte ab: AES-GCM, der Nonce
// steht vorn, alles zusammen in Base64.
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

// GeheimEntschluesseln liest zurück, was GeheimVerschluesseln abgelegt hat.
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
// Das Ticket zwischen den beiden Schritten
//
// Zwischen Passwort und Code ist niemand angemeldet, und trotzdem muss der
// zweite Aufruf wissen, um wen es geht. Er bekommt dafür ein kurzlebiges,
// signiertes Ticket statt einer Sitzung.
//
// Signiert wird mit einem eigenen, aus dem Signaturgeheimnis abgeleiteten
// Schlüssel. Damit ist ausgeschlossen, dass ein Ticket je als Sitzungskeks
// durchgeht: die Prüfung der Sitzung rechnet mit dem anderen Schlüssel und
// verwirft es.

// ZweitTicketDauer ist die Frist für den zweiten Schritt. Fünf Minuten reichen,
// um ein Telefon aus der Tasche zu holen, und sind kurz genug, dass ein
// abgefangenes Ticket wertlos altert.
const ZweitTicketDauer = 5 * time.Minute

func ticketSchluessel(secret []byte) []byte {
	h := sha256.Sum256(append([]byte("nexora-zweitfaktor-ticket:"), secret...))
	return h[:]
}

// ZweitTicket signiert das Ticket für ein Konto.
func ZweitTicket(secret []byte, userID string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ZweitTicketDauer)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString(ticketSchluessel(secret))
}

// ZweitTicketLesen gibt das Konto zurück, für das das Ticket ausgestellt wurde.
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
