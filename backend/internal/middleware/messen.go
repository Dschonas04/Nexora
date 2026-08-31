// Das Messen der Anfragen, als Filter vor allen anderen.
//
// Es steht ganz vorn in der Kette und nicht bei den fachlichen Filtern, weil
// eine Anfrage auch dann gezählt gehört, wenn sie an der Anmeldung scheitert
// oder wenn eine Lizenz fehlt. Wer sucht, warum es hängt, will gerade die
// sehen, die nicht durchkommen.
package middleware

import (
	"net/http"

	"nexora/internal/puls"
)

// schreiberMitStatus merkt sich, was geantwortet wurde. net/http gibt den
// Status nicht her, und ohne ihn liesse sich eine überlastete Instanz nicht von
// einer unterscheiden, die fleissig 401 verteilt.
type schreiberMitStatus struct {
	http.ResponseWriter
	status int
}

func (s *schreiberMitStatus) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *schreiberMitStatus) Write(b []byte) (int, error) {
	// Wer schreibt, ohne den Kopf zu setzen, hat 200 gemeint.
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Messen zählt jede Anfrage und ihre Dauer.
func Messen(m *puls.Messer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Der eigene Abfrageweg wird nicht mitgezählt. Die Oberfläche ruft
			// ihn im Sekundentakt, und er stünde sonst als Grundrauschen in
			// jeder Messung, die er anzeigen soll.
			if r.URL.Path == "/api/system/puls" {
				next.ServeHTTP(w, r)
				return
			}
			ende := m.Beginn()
			sw := &schreiberMitStatus{ResponseWriter: w}
			defer func() {
				if sw.status == 0 {
					sw.status = http.StatusOK
				}
				ende(sw.status)
			}()
			next.ServeHTTP(sw, r)
		})
	}
}
