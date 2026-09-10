package auth

import (
	"testing"
	"time"
)

// The test vectors from RFC 6238, appendix B, for HMAC-SHA1. The secret there
// is the ASCII sequence "12345678901234567890"; here it stands in Base32, the
// way an authenticator app would receive it. The RFC names eight digits, the
// lower six are compared -- those are the ones Nexora issues.
func TestTOTPCodeGegenRFC6238(t *testing.T) {
	const geheim = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	faelle := []struct {
		sekunden int64
		will     string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, f := range faelle {
		got, err := TOTPCode(geheim, time.Unix(f.sekunden, 0))
		if err != nil {
			t.Fatalf("t=%d: %v", f.sekunden, err)
		}
		if got != f.will {
			t.Errorf("t=%d: %s, erwartet %s", f.sekunden, got, f.will)
		}
	}
}

// The current code must match, another one must not, and spaces out of a
// clipboard must not get in the way.
func TestTOTPPruefen(t *testing.T) {
	geheim, err := NeuesGeheimnis()
	if err != nil {
		t.Fatal(err)
	}
	jetzt, err := TOTPCode(geheim, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !TOTPPruefen(geheim, jetzt) {
		t.Error("der laufende Code wurde abgelehnt")
	}
	if !TOTPPruefen(geheim, " "+jetzt+" ") {
		t.Error("Leerzeichen um den Code herum duerfen nicht stoeren")
	}
	if TOTPPruefen(geheim, "000000") && jetzt != "000000" {
		t.Error("ein falscher Code kam durch")
	}
	if TOTPPruefen(geheim, "12345") {
		t.Error("ein zu kurzer Code kam durch")
	}
	// An hour off lies far outside the tolerance of one window.
	alt, _ := TOTPCode(geheim, time.Now().Add(-time.Hour))
	if TOTPPruefen(geheim, alt) {
		t.Error("ein eine Stunde alter Code kam durch")
	}
}

// The secret must come back out of the column unchanged, and must not open
// with a different signing secret -- that is precisely the point of the
// encryption.
func TestGeheimHinUndZurueck(t *testing.T) {
	secret := []byte("Signaturgeheimnis der Instanz")
	klar, err := NeuesGeheimnis()
	if err != nil {
		t.Fatal(err)
	}
	abgelegt, err := GeheimVerschluesseln(secret, klar)
	if err != nil {
		t.Fatal(err)
	}
	if abgelegt == klar {
		t.Fatal("das Geheimnis steht im Klartext in der Spalte")
	}
	zurueck, err := GeheimEntschluesseln(secret, abgelegt)
	if err != nil || zurueck != klar {
		t.Fatalf("zurueck: %q, %v", zurueck, err)
	}
	if _, err := GeheimEntschluesseln([]byte("ein anderes Geheimnis"), abgelegt); err == nil {
		t.Error("mit falschem Schluessel liess es sich oeffnen")
	}
}

// Two encryptions of the same secret must not look alike, or the column would
// reveal which accounts carry the same secret.
func TestGeheimMitFrischemNonce(t *testing.T) {
	secret := []byte("Signaturgeheimnis")
	a, _ := GeheimVerschluesseln(secret, "GEZDGNBVGY3TQOJQ")
	b, _ := GeheimVerschluesseln(secret, "GEZDGNBVGY3TQOJQ")
	if a == b {
		t.Error("zweimal derselbe Chiffretext")
	}
}

// The ticket between the two steps must only open with its own key. If it
// passed as a session cookie as well, the second factor would be bypassed.
func TestZweitTicket(t *testing.T) {
	secret := []byte("Signaturgeheimnis")
	ticket, err := ZweitTicket(secret, "konto-1")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := ZweitTicketLesen(secret, ticket)
	if err != nil || uid != "konto-1" {
		t.Fatalf("gelesen: %q, %v", uid, err)
	}
	if _, _, err := ParseToken(secret, ticket); err == nil {
		t.Error("das Ticket ging als Sitzungstoken durch")
	}
	sitzung, _ := GenerateToken(secret, "konto-1", "sitzung-1", time.Hour)
	if _, err := ZweitTicketLesen(secret, sitzung); err == nil {
		t.Error("ein Sitzungstoken ging als Ticket durch")
	}
}

// A recovery code gets copied out by hand: it should have two blocks and
// contain no characters that are mistaken for others.
func TestErsatzcodeForm(t *testing.T) {
	gesehen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := NeuerErsatzcode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 11 || c[5] != '-' {
			t.Fatalf("unerwartete Form: %q", c)
		}
		for _, z := range c {
			if z == '-' {
				continue
			}
			if z == 'l' || z == 'o' || z == '0' || z == '1' {
				t.Fatalf("verwechselbares Zeichen in %q", c)
			}
		}
		if gesehen[c] {
			t.Fatalf("Code kam zweimal: %q", c)
		}
		gesehen[c] = true
	}
}
