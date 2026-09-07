package auth

import (
	"testing"
	"time"
)

// Die Pruefvektoren aus RFC 6238, Anhang B, fuer HMAC-SHA1. Das Geheimnis dort
// ist die ASCII-Folge "12345678901234567890"; hier steht sie in Base32, so wie
// eine Authenticator-App sie bekaeme. Der RFC nennt acht Stellen, verglichen
// werden die unteren sechs -- das sind die, die Nexora ausgibt.
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

// Der laufende Code muss passen, ein anderer nicht, und Leerzeichen aus einer
// Zwischenablage duerfen nicht stoeren.
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
	// Eine Stunde daneben liegt weit ausserhalb der Toleranz von einem Fenster.
	alt, _ := TOTPCode(geheim, time.Now().Add(-time.Hour))
	if TOTPPruefen(geheim, alt) {
		t.Error("ein eine Stunde alter Code kam durch")
	}
}

// Das Geheimnis muss aus der Spalte unveraendert zurueckkommen, und mit einem
// anderen Signaturgeheimnis darf es sich nicht oeffnen lassen -- genau das ist
// der Zweck der Verschluesselung.
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

// Zwei Verschluesselungen desselben Geheimnisses duerfen nicht gleich aussehen,
// sonst verriete die Spalte, welche Konten dasselbe Geheimnis tragen.
func TestGeheimMitFrischemNonce(t *testing.T) {
	secret := []byte("Signaturgeheimnis")
	a, _ := GeheimVerschluesseln(secret, "GEZDGNBVGY3TQOJQ")
	b, _ := GeheimVerschluesseln(secret, "GEZDGNBVGY3TQOJQ")
	if a == b {
		t.Error("zweimal derselbe Chiffretext")
	}
}

// Das Ticket zwischen den Schritten darf nur mit seinem eigenen Schluessel
// aufgehen. Ginge es auch als Sitzungskeks durch, waere der zweite Faktor
// umgangen.
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

// Ein Ersatzcode wird abgeschrieben: er soll zwei Bloecke haben und keine
// Zeichen enthalten, die man mit anderen verwechselt.
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
