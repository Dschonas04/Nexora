package handlers

import (
	"strings"
	"testing"
)

// The lower bound counts characters and not bytes: a password of six umlauts
// is six characters long and must not fail because each of them needs two
// bytes.
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

// An account from SSO carries a marker instead of a hash. If that is not
// recognised, an administrator sets a password on it and thereby takes away its
// access, because sso.go no longer adopts it afterwards.
func TestSSOKontoWirdErkannt(t *testing.T) {
	if h, ja := ssoHerkunft("sso:keycloak"); !ja || h != "keycloak" {
		t.Fatalf("Herkunft nicht erkannt: %q %v", h, ja)
	}
	// A real bcrypt hash starts with $2 and must not count as SSO.
	if _, ja := ssoHerkunft("$2a$12$abcdefghijklmnopqrstuv"); ja {
		t.Fatal("bcrypt-Hash als SSO-Konto gelesen")
	}
	if _, ja := ssoHerkunft(""); ja {
		t.Fatal("leeres Feld als SSO-Konto gelesen")
	}
}
