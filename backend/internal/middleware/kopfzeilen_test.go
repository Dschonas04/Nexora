package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// leer is a handler that writes nothing. It stands for a route that answers
// before it has decided anything about its own headers.
func leer(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func TestKopfzeilenStehenAnJederAntwort(t *testing.T) {
	w := httptest.NewRecorder()
	Sicherheitskopfzeilen(http.HandlerFunc(leer)).
		ServeHTTP(w, httptest.NewRequest("GET", "/api/pages", nil))

	will := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Referrer-Policy":              "no-referrer",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'; base-uri 'none'",
	}
	for k, v := range will {
		if got := w.Header().Get(k); got != v {
			t.Errorf("%s = %q, erwartet %q", k, got, v)
		}
	}
}

// Without TLS the promise must not be made: a browser would refuse the plain
// connection on the next visit and lock out an instance that runs open.
func TestHstsNurVerschluesselt(t *testing.T) {
	w := httptest.NewRecorder()
	Sicherheitskopfzeilen(http.HandlerFunc(leer)).
		ServeHTTP(w, httptest.NewRequest("GET", "/api/pages", nil))
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("offen ausgeliefert, trotzdem HSTS: %q", got)
	}
}

func TestHstsHinterDemGegenstueck(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/pages", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	Sicherheitskopfzeilen(http.HandlerFunc(leer)).ServeHTTP(w, r)
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("hinter TLS fehlt HSTS")
	}
}

// A handler that hands out an attachment needs a stricter policy than the API
// does. The filter must not stand in its way.
func TestHandlerDarfUeberschreiben(t *testing.T) {
	streng := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.WriteHeader(http.StatusOK)
	}
	w := httptest.NewRecorder()
	Sicherheitskopfzeilen(http.HandlerFunc(streng)).
		ServeHTTP(w, httptest.NewRequest("GET", "/api/attachments/x/datei", nil))
	if got := w.Header().Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
		t.Errorf("der Handler kam nicht durch: %q", got)
	}
}
