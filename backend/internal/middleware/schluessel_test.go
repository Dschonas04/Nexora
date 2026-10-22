package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// An API key stands in for the cookie; a reading key is stopped at the first
// writing request, and NurSitzung turns every key away.
func TestSchluessel(t *testing.T) {
	alt := SchluesselPruefer
	defer func() { SchluesselPruefer = alt }()
	SchluesselPruefer = func(_ context.Context, k string) (string, bool, bool) {
		switch k {
		case "nxr_lesen":
			return "u1", false, true
		case "nxr_schreiben":
			return "u2", true, true
		}
		return "", false, false
	}
	var gesehen string
	ziel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gesehen = UserID(r)
		w.WriteHeader(http.StatusNoContent)
	})
	kette := Auth([]byte("geheim"), nil)(ziel)
	fall := func(methode, schluessel string) int {
		gesehen = ""
		r := httptest.NewRequest(methode, "/api/pages", nil)
		if schluessel != "" {
			r.Header.Set("Authorization", "Bearer "+schluessel)
		}
		w := httptest.NewRecorder()
		kette.ServeHTTP(w, r)
		return w.Code
	}
	if c := fall("GET", "nxr_lesen"); c != 204 || gesehen != "u1" {
		t.Errorf("lesender Schlüssel, GET: %d %q", c, gesehen)
	}
	if c := fall("PUT", "nxr_lesen"); c != 403 || gesehen != "" {
		t.Errorf("lesender Schlüssel, PUT: %d", c)
	}
	if c := fall("PUT", "nxr_schreiben"); c != 204 || gesehen != "u2" {
		t.Errorf("schreibender Schlüssel, PUT: %d %q", c, gesehen)
	}
	if c := fall("GET", "nxr_falsch"); c != 401 {
		t.Errorf("falscher Schlüssel: %d", c)
	}
	if c := fall("GET", ""); c != 401 {
		t.Errorf("ohne alles: %d", c)
	}

	gesperrt := Auth([]byte("geheim"), nil)(NurSitzung(ziel))
	r := httptest.NewRequest("GET", "/api/api-keys", nil)
	r.Header.Set("Authorization", "Bearer nxr_schreiben")
	w := httptest.NewRecorder()
	gesperrt.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Errorf("NurSitzung ließ einen Schlüssel durch: %d", w.Code)
	}
}
