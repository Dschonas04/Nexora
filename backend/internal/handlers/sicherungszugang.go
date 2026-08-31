// Wer die Sicherung abholen darf.
//
// Aus dem Panel heraus reicht die Sitzung: ein angemeldeter Administrator
// klickt, der Browser schickt den Keks mit. Ein Skript hat keinen Keks, und
// genau das ist der Fall, um den es beim Automatisieren geht. Es braucht also
// einen zweiten Weg hinein, und der ist ein Losungswort.
//
// DAS LOSUNGSWORT WIEGT SCHWERER ALS DAS FÜR DIE KENNZAHLEN. Dort gibt es eine
// Zusammenfassung heraus, hier den gesamten Bestand samt Passwort-Hashes,
// Sitzungen und Freigabe-Tokens. Es ist deshalb ein eigenes: wer den Sammler
// für Kennzahlen einrichtet, soll dabei nicht nebenbei einen Schlüssel zur
// ganzen Datenbank verteilen. Und jeder Abruf landet in der Prüfspur, mit der
// Adresse, von der er kam.
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

// SicherungToken ist das Losungswort für den Abruf ohne Sitzung. Leer heißt:
// diesen Weg gibt es nicht, es bleibt bei der Anmeldung.
func SicherungToken() string { return wert("sicherung_token") }

// SicherungZugang lässt entweder ein gültiges Losungswort oder eine Sitzung
// durch.
//
// Zusammengesetzt statt nebeneinander: ein zweiter Weg mit eigener Adresse
// wäre eine zweite Stelle, an der die Rechte zu pflegen sind, und die beiden
// liefen früher oder später auseinander.
func SicherungZugang(secret []byte, pruefe middleware.SitzungPruefer) func(http.Handler) http.Handler {
	ueberSitzung := middleware.Auth(secret, pruefe)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := SicherungToken(); token != "" {
				angeboten := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				// Vergleich in fester Zeit, damit sich das Wort nicht Zeichen
				// für Zeichen erraten lässt.
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

// perToken sagt, ob dieser Aufruf über das Losungswort hereinkam.
func perToken(r *http.Request) bool {
	wert, _ := r.Context().Value(perTokenSchluessel).(bool)
	return wert
}
