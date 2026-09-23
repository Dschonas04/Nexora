// Measuring requests, as the middleware in front of all the others.
//
// It sits right at the front of the chain and not among the domain middleware,
// because a request deserves to be counted even when it fails at sign-in or
// when a licence is missing. Whoever is looking for why it stalls wants to see
// exactly those that do not get through.
package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strings"

	"nexora/internal/puls"
)

// schreiberMitStatus remembers what was answered. net/http does not hand the
// status back, and without it an overloaded instance could not be told apart
// from one busily handing out 401s.
type schreiberMitStatus struct {
	http.ResponseWriter
	status int
}

func (s *schreiberMitStatus) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *schreiberMitStatus) Write(b []byte) (int, error) {
	// Whoever writes without setting a header meant 200.
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Hijack passes the connection through.
//
// Without it there would be no writing together: a WebSocket takes over the
// bare connection, and passing on only the http.ResponseWriter here hides the
// place where it can be had. The call would then get as far as the upgrade and
// fail there.
func (s *schreiberMitStatus) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("die Verbindung lässt sich nicht übernehmen")
	}
	// From here on somebody else writes, and the status is whatever the upgrade
	// left behind: 101, or it would not have got this far.
	if s.status == 0 {
		s.status = http.StatusSwitchingProtocols
	}
	return h.Hijack()
}

// Messen counts every request and how long it took.
func Messen(m *puls.Messer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The route the measurements are read through does not count
			// itself: the interface polls it once a second, and it would
			// otherwise sit as background noise inside every measurement it is
			// meant to show -- the request rate would never be zero, even with
			// nobody working.
			// Writing together does not count either. It is not a request that
			// gets answered but a connection that stays open for hours; counted
			// as one request of two hours it would spoil every average shown
			// beside it.
			if r.URL.Path == "/api/system/puls" ||
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
