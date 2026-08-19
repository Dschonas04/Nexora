package lizenz_test

import (
	"testing"

	kern "nexora/internal/lizenz"
	_ "nexora/premium/lizenz"
)

const gueltig = "TESTSCHLUESSEL_AUS_DER_HISTORIE_ENTFERNT"

func TestOhneSchluesselAllesGesperrt(t *testing.T) {
	kern.Laden("")
	for _, f := range kern.Alle {
		if kern.Frei(f) {
			t.Fatalf("%s ist ohne Schlüssel frei", f)
		}
	}
	if kern.Aktuell().Gueltig {
		t.Fatal("Zustand meldet gültig ohne Schlüssel")
	}
}

func TestGueltigerSchluesselSchaltetFrei(t *testing.T) {
	kern.Laden(gueltig)
	z := kern.Aktuell()
	if !z.Gueltig {
		t.Fatalf("gültiger Schlüssel abgelehnt: %s", z.Grund)
	}
	for _, f := range kern.Alle {
		if !kern.Frei(f) {
			t.Fatalf("%s wurde nicht freigeschaltet", f)
		}
	}
	if z.Inhaber != "Jonas Groll" {
		t.Fatalf("falscher Inhaber: %q", z.Inhaber)
	}
}

func TestVeraenderterSchluesselWirdAbgelehnt(t *testing.T) {
	// Ein einzelnes Zeichen im Datenteil kippen -- die Signatur darf nicht
	// mehr passen. Genau das ist der Kern des Verfahrens.
	b := []byte(gueltig)
	for i := range b {
		if b[i] == 'o' {
			b[i] = '0'
			break
		}
	}
	kern.Laden(string(b))
	if kern.Aktuell().Gueltig {
		t.Fatal("veränderter Schlüssel wurde angenommen")
	}
	for _, f := range kern.Alle {
		if kern.Frei(f) {
			t.Fatalf("%s trotz verändertem Schlüssel frei", f)
		}
	}
}

func TestUnsinnWirdAbgelehnt(t *testing.T) {
	for _, s := range []string{"quatsch", "a.b", "....", "eyJ9.eyJ9"} {
		kern.Laden(s)
		if kern.Aktuell().Gueltig {
			t.Fatalf("Unsinn %q wurde angenommen", s)
		}
	}
}
