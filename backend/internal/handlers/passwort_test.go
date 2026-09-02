package handlers

import (
	"strings"
	"testing"
)

// Die Untergrenze zaehlt Zeichen und nicht Bytes: ein Passwort aus sechs
// Umlauten ist sechs Zeichen lang und darf nicht daran scheitern, dass jedes
// davon zwei Bytes braucht.
func TestPasswortGrenzen(t *testing.T) {
	faelle := []struct {
		name    string
		wert    string
		abweist bool
	}{
		{"zu kurz", "12345", true},
		{"gerade lang genug", "123456", false},
		{"sechs Umlaute", "äöüäöü", false},
		{"nur Leerzeichen", "        ", true},
		{"zu lang fuer bcrypt", strings.Repeat("a", 73), true},
		{"genau 72", strings.Repeat("a", 72), false},
	}
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			meldung := passwortPruefen(f.wert)
			if f.abweist && meldung == "" {
				t.Fatalf("%q sollte abgewiesen werden", f.wert)
			}
			if !f.abweist && meldung != "" {
				t.Fatalf("%q wurde abgewiesen: %s", f.wert, meldung)
			}
		})
	}
}

// Ein Konto aus dem SSO traegt statt eines Hashs eine Marke. Wird die nicht
// erkannt, setzt eine Verwaltung ihm ein Passwort und nimmt ihm damit den
// Zugang, weil sso.go es danach nicht mehr uebernimmt.
func TestSSOKontoWirdErkannt(t *testing.T) {
	if h, ja := ssoHerkunft("sso:keycloak"); !ja || h != "keycloak" {
		t.Fatalf("Herkunft nicht erkannt: %q %v", h, ja)
	}
	// Ein echter bcrypt-Hash faengt mit $2 an und darf nicht als SSO gelten.
	if _, ja := ssoHerkunft("$2a$12$abcdefghijklmnopqrstuv"); ja {
		t.Fatal("bcrypt-Hash als SSO-Konto gelesen")
	}
	if _, ja := ssoHerkunft(""); ja {
		t.Fatal("leeres Feld als SSO-Konto gelesen")
	}
}
