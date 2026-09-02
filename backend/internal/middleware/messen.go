// Das Messen der Anfragen, als Filter vor allen anderen.
//
// Es steht ganz vorn in der Kette und nicht bei den fachlichen Filtern, weil
// eine Anfrage auch dann gezählt gehört, wenn sie an der Anmeldung scheitert
// oder wenn eine Lizenz fehlt. Wer sucht, warum es hängt, will gerade die
// sehen, die nicht durchkommen.
package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strings"

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

// Hijack reicht die Leitung durch.
//
// Ohne das gäbe es kein gemeinsames Schreiben: eine WebSocket-Verbindung
// übernimmt die nackte Verbindung, und wer hier nur den http.ResponseWriter
// weitergibt, verdeckt die Stelle, an der sie zu holen ist. Der Aufruf käme
// dann bis zum Aufschalten und schlüge dort fehl.
func (s *schreiberMitStatus) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("die Verbindung lässt sich nicht übernehmen")
	}
	// Von hier an schreibt jemand anderes, und der Status ist das, was der
	// Aufschlag hinterlassen hat: 101, sonst wäre es nicht so weit gekommen.
	if s.status == 0 {
		s.status = http.StatusSwitchingProtocols
	}
	return h.Hijack()
}

// Messen zählt jede Anfrage und ihre Dauer.
func Messen(m *puls.Messer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Die beiden Wege, über die gemessen wird, zählen sich nicht selbst
			// mit. Die Oberfläche ruft den einen im Sekundentakt ab und
			// Prometheus den anderen alle fünfzehn; sie stünden sonst als
			// Grundrauschen in jeder Messung, die sie anzeigen sollen, und die
			// Rate der Anfragen wäre nie null, auch wenn niemand arbeitet.
			// Und das gemeinsame Schreiben zählt auch nicht mit. Es ist keine
			// Anfrage, die beantwortet wird, sondern eine Leitung, die
			// stundenlang offen steht; als eine Anfrage von zwei Stunden
			// gezählt verdürbe sie jeden Mittelwert, den die Anzeige daneben
			// zeigt.
			if r.URL.Path == "/api/system/puls" || r.URL.Path == "/metrics" ||
				strings.HasPrefix(r.URL.Path, "/api/echtzeit/") {
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
