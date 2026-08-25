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

// paar erzeugt ein Schlüsselpaar für den Test. Der eingebaute öffentliche
// Schlüssel taugt hier nicht: zu ihm gehört ein privater, der nicht im
// Verzeichnis liegt und auch nie dort liegen darf.
func paar(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	oeff, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Schlüsselpaar: %v", err)
	}
	return oeff, priv
}

// Der ganze Weg, für jede Stufe: ausstellen, prüfen, und was dabei
// freigeschaltet wird, muss genau die Stufe sein.
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

// Ohne Angabe gilt ein Jahr, und länger wird nicht ausgestellt. Der Grund steht
// bei HoechsteLaufzeit: geprüft wird offline, ein Schlüssel lässt sich nicht
// zurückrufen, das Datum ist der einzige Hebel.
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

// Ein abgelaufener Schlüssel schaltet nichts frei, sonst wäre die Frist eine
// Behauptung ohne Folgen.
func TestAbgelaufenerSchluesselGiltNicht(t *testing.T) {
	oeff, priv := paar(t)
	// Am Aussteller vorbei, weil der ein Datum in der Vergangenheit zu Recht
	// verweigert. Geprüft wird hier die andere Seite.
	alt := Nutzlast{Inhaber: "Kundin", Stufe: string(kern.StufePro),
		Ablauf: time.Now().Add(-72 * time.Hour).Format("2006-01-02")}
	schluessel := selbstGebaut(t, priv, alt)
	if _, err := NeuerPruefer(oeff).Pruefe(schluessel); err == nil {
		t.Error("ein abgelaufener Schlüssel wurde angenommen")
	} else if !strings.Contains(err.Error(), "abgelaufen") {
		t.Errorf("unerwarteter Grund: %v", err)
	}
}

// Ein veränderter Schlüssel muss auffallen, sonst könnte sich jeder die
// Stufe umschreiben.
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

// selbstGebaut unterschreibt eine Nutzlast unmittelbar, für die Fälle, die
// der Aussteller zu Recht verweigert und die der Prüfer trotzdem abfangen muss.
func selbstGebaut(t *testing.T, priv ed25519.PrivateKey, n Nutzlast) string {
	t.Helper()
	daten, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("verpacken: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(daten) + "." +
		base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, daten))
}
