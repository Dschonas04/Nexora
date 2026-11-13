package auth

import (
	"testing"
	"time"
)

func TestTOTPPruefenEinmal(t *testing.T) {
	geheim := "JBSWY3DPEHPK3PXP"
	code, err := TOTPCode(geheim, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !TOTPPruefenEinmal("konto-a", geheim, code) {
		t.Fatal("erster Einsatz abgelehnt")
	}
	if TOTPPruefenEinmal("konto-a", geheim, code) {
		t.Fatal("derselbe Code ein zweites Mal angenommen")
	}
	alt, _ := TOTPCode(geheim, time.Now().Add(-TOTPSchritt))
	if alt != code && TOTPPruefenEinmal("konto-a", geheim, alt) {
		t.Fatal("älterer Code nach einem neueren angenommen")
	}
	if !TOTPPruefenEinmal("konto-b", geheim, code) {
		t.Fatal("anderes Konto durch fremden Verbrauch gesperrt")
	}
}

func TestCheckPasswordOhneBcrypt(t *testing.T) {
	start := time.Now()
	if CheckPassword("sso:keycloak", "egal") {
		t.Fatal("SSO-Marker als Passwort angenommen")
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("Antwort ohne bcrypt-Lauf – verrät die Kontoart")
	}
}
