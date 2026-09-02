package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Der Keks traegt Secure genau dann, wenn der BROWSER verschluesselt spricht --
// nicht, wenn diese eine Verbindung es tut.
//
// Seit der Innenverkehr des Verbunds verschluesselt ist, kommt jede Anfrage
// ueber TLS beim Dienst an. Entschiede r.TLS, bekaeme jemand, der ueber
// gewoehnliches HTTP auf die Oberflaeche zugreift, einen Secure-Keks, sein
// Browser schickte ihn nicht zurueck, und er waere nach der Anmeldung sofort
// wieder abgemeldet. Genau das ist am 02.09.2026 passiert.
func TestUeberTLSFolgtDemBrowser(t *testing.T) {
	faelle := []struct {
		name       string
		eigenesTLS bool
		weiterKopf string
		erwartet   bool
	}{
		{"offen, ohne Gegenstueck", false, "", false},
		{"verschluesselt, ohne Gegenstueck", true, "", true},
		// Der Fall, der den Ausfall gemacht hat: Browser offen, Innenverkehr
		// verschluesselt.
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
