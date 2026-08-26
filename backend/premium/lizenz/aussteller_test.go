package lizenz

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	kern "nexora/internal/lizenz"
)

// paar generates a key pair for the test. The built in public key is of no use
// here: a private one belongs to it that does not lie in the repository and
// never may.
func paar(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	oeff, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Schlüsselpaar: %v", err)
	}
	return oeff, priv
}

// The whole way, for every tier: issue, check, and what gets unlocked has to be
// exactly that tier.
func TestJedeStufeLaesstSichAktivieren(t *testing.T) {
	oeff, priv := paar(t)
	pruefer := NeuerPruefer(oeff)

	for _, stufe := range kern.StufenReihe {
		schluessel, err := Ausstellen(priv, "Kundin "+string(stufe), stufe, nil, time.Time{})
		if err != nil {
			t.Fatalf("%s ausstellen: %v", stufe, err)
		}
		z, err := pruefer.Pruefe(schluessel)
		if err != nil {
			t.Fatalf("%s prüfen: %v", stufe, err)
		}
		if !z.Gueltig {
			t.Errorf("%s: nicht gültig", stufe)
		}
		if z.Stufe != stufe {
			t.Errorf("%s: Stufe kommt als %q zurück", stufe, z.Stufe)
		}
		soll := kern.FunktionenDerStufe(stufe)
		if len(z.Funktionen) != len(soll) {
			t.Errorf("%s: %d Funktionen statt %d", stufe, len(z.Funktionen), len(soll))
		}
		haben := map[kern.Funktion]bool{}
		for _, f := range z.Funktionen {
			haben[f] = true
		}
		for _, f := range soll {
			if !haben[f] {
				t.Errorf("%s: %q fehlt", stufe, f)
			}
		}
	}
}

// A tier plus extras bought on top of it.
func TestStufePlusZusatz(t *testing.T) {
	oeff, priv := paar(t)
	schluessel, err := Ausstellen(priv, "Kundin", kern.StufeAdvanced,
		[]kern.Funktion{kern.Gruppen}, time.Time{})
	if err != nil {
		t.Fatalf("ausstellen: %v", err)
	}
	z, err := NeuerPruefer(oeff).Pruefe(schluessel)
	if err != nil {
		t.Fatalf("prüfen: %v", err)
	}
	hat := func(f kern.Funktion) bool {
		for _, x := range z.Funktionen {
			if x == f {
				return true
			}
		}
		return false
	}
	if !hat(kern.Versionen) {
		t.Error("Versionen aus der Stufe fehlt")
	}
	if !hat(kern.Gruppen) {
		t.Error("der einzeln gekaufte Zusatz fehlt")
	}
	if hat(kern.Pruefspur) {
		t.Error("Pruefspur wurde freigeschaltet, obwohl niemand sie gekauft hat")
	}
}

// Without a given duration one year applies, and nothing longer is issued. The
// reason stands at HoechsteLaufzeit: checking happens offline, a key cannot be
// recalled, the date is the only lever.
func TestLaufzeit(t *testing.T) {
	oeff, priv := paar(t)

	schluessel, err := Ausstellen(priv, "Kundin", kern.StufePro, nil, time.Time{})
	if err != nil {
		t.Fatalf("ausstellen: %v", err)
	}
	z, _ := NeuerPruefer(oeff).Pruefe(schluessel)
	bis, err := time.Parse("2006-01-02", z.LaeuftAb)
	if err != nil {
		t.Fatalf("Ablauf %q unlesbar", z.LaeuftAb)
	}
	tage := int(time.Until(bis).Hours() / 24)
	if tage < 360 || tage > 366 {
		t.Errorf("Vorgabe sind %d Tage, erwartet rund ein Jahr", tage)
	}

	if _, err := Ausstellen(priv, "Kundin", kern.StufePro, nil,
		time.Now().Add(3*365*24*time.Hour)); err == nil {
		t.Error("drei Jahre wurden ausgestellt")
	}
	if _, err := Ausstellen(priv, "Kundin", kern.StufePro, nil,
		time.Now().Add(-24*time.Hour)); err == nil {
		t.Error("ein Datum in der Vergangenheit wurde angenommen")
	}
	if _, err := Ausstellen(priv, "", kern.StufePro, nil, time.Time{}); err == nil {
		t.Error("ohne Inhaber wurde ausgestellt")
	}
	if _, err := Ausstellen(priv, "Kundin", "gold", nil, time.Time{}); err == nil {
		t.Error("eine erfundene Stufe wurde ausgestellt")
	}
}

// An expired key unlocks nothing, otherwise the expiry would be a claim without
// consequences.
func TestAbgelaufenerSchluesselGiltNicht(t *testing.T) {
	oeff, priv := paar(t)
	// Past the issuer, because it rightly refuses a date in the past. What is
	// checked here is the other side.
	alt := Nutzlast{Inhaber: "Kundin", Stufe: string(kern.StufePro),
		Ablauf: time.Now().Add(-72 * time.Hour).Format("2006-01-02")}
	schluessel := selbstGebaut(t, priv, alt)
	if _, err := NeuerPruefer(oeff).Pruefe(schluessel); err == nil {
		t.Error("ein abgelaufener Schlüssel wurde angenommen")
	} else if !strings.Contains(err.Error(), "abgelaufen") {
		t.Errorf("unerwarteter Grund: %v", err)
	}
}

// A tampered key has to be noticed, otherwise anybody could rewrite their own
// tier.
func TestVeraenderterSchluesselFaelltAuf(t *testing.T) {
	oeff, priv := paar(t)
	schluessel, err := Ausstellen(priv, "Kundin", kern.StufeAdvanced, nil, time.Time{})
	if err != nil {
		t.Fatalf("ausstellen: %v", err)
	}
	teile := strings.SplitN(schluessel, ".", 2)
	// Flip one character in the data half.
	daten := []byte(teile[0])
	if daten[3] == 'A' {
		daten[3] = 'B'
	} else {
		daten[3] = 'A'
	}
	if _, err := NeuerPruefer(oeff).Pruefe(string(daten) + "." + teile[1]); err == nil {
		t.Error("ein verändertes Datenpaket wurde angenommen")
	}

	// And a key from a foreign pair.
	_, fremdPriv := paar(t)
	fremd, _ := Ausstellen(fremdPriv, "Fremde", kern.StufeBusiness, nil, time.Time{})
	if _, err := NeuerPruefer(oeff).Pruefe(fremd); err == nil {
		t.Error("ein fremd unterschriebener Schlüssel wurde angenommen")
	}
}

// selbstGebaut signs a payload directly, for the cases the issuer rightly
// refuses and the verifier still has to catch.
func selbstGebaut(t *testing.T, priv ed25519.PrivateKey, n Nutzlast) string {
	t.Helper()
	daten, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("verpacken: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(daten) + "." +
		base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, daten))
}
