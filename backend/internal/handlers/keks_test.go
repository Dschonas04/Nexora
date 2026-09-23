package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The cookie carries Secure exactly when the BROWSER speaks encrypted -- not
// when this one connection does.
//
// Since traffic inside the compound is encrypted, every request reaches the
// service over TLS. If r.TLS decided, somebody reaching the interface over
// plain HTTP would get a Secure cookie, their browser would not send it back,
// and they would be signed out again immediately after signing in. That is
// exactly what happened on 02.09.2026.
func TestUeberTLSFolgtDemBrowser(t *testing.T) {
	faelle := []struct {
		name       string
		eigenesTLS bool
		weiterKopf string
		erwartet   bool
	}{
		{"offen, ohne Gegenstueck", false, "", false},
		{"verschluesselt, ohne Gegenstueck", true, "", true},
		// The case that caused the outage: browser in the clear, internal
		// traffic encrypted.
		{"Gegenstueck sagt http, innen TLS", true, "http", false},
		{"Gegenstueck sagt https, innen TLS", true, "https", true},
		{"Gegenstueck sagt https, innen offen", false, "https", true},
		{"Gegenstueck sagt http, innen offen", false, "http", false},
		{"Grossschreibung stoert nicht", false, "HTTPS", true},
	}
	for _, f := range faelle {
		r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		if f.eigenesTLS {
			r.TLS = &tls.ConnectionState{}
		}
		if f.weiterKopf != "" {
			r.Header.Set("X-Forwarded-Proto", f.weiterKopf)
		}
		if raus := ueberTLS(r); raus != f.erwartet {
			t.Errorf("%s: erwartet %v, bekam %v", f.name, f.erwartet, raus)
		}
	}
	if ueberTLS(nil) {
		t.Error("ohne Anfrage ist nichts verschluesselt")
	}
}
