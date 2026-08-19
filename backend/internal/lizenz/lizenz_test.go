package lizenz_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	kern "nexora/internal/lizenz"
	plizenz "nexora/premium/lizenz"
)

// Der Test erzeugt sein eigenes Schlüsselpaar und stellt sich seine Lizenzen
// selbst aus.
//
// Ein fertiger Schlüssel im Repository wäre ein Schlüssel für jeden, der es
// klont -- und weil offline geprüft wird, ließe er sich nie zurückziehen.
// Nebenbei prüft der Test so den ganzen Weg vom Signieren bis zum
// Freischalten, statt nur die halbe Strecke.
func einrichten(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	oeff, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kern.Registriere(plizenz.NeuerPruefer(oeff))
	return priv
}

// schluessel signiert eine Nutzlast und baut daraus den fertigen Schlüssel.
func schluessel(t *testing.T, priv ed25519.PrivateKey, n plizenz.Nutzlast) string {
	t.Helper()
	daten, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, daten)
	return base64.RawURLEncoding.EncodeToString(daten) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

func alleNamen() []string {
	var s []string
	for _, f := range kern.Alle {
		s = append(s, string(f))
	}
	return s
}

func TestOhneSchluesselAllesGesperrt(t *testing.T) {
	einrichten(t)
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
	priv := einrichten(t)
	kern.Laden(schluessel(t, priv, plizenz.Nutzlast{
		Inhaber:     "Testfirma",
		Funktionen:  alleNamen(),
		Ausgestellt: time.Now().Format("2006-01-02"),
	}))

	z := kern.Aktuell()
	if !z.Gueltig {
		t.Fatalf("gültiger Schlüssel abgelehnt: %s", z.Grund)
	}
	if z.Inhaber != "Testfirma" {
		t.Fatalf("falscher Inhaber: %q", z.Inhaber)
	}
	for _, f := range kern.Alle {
		if !kern.Frei(f) {
			t.Fatalf("%s wurde nicht freigeschaltet", f)
		}
	}
}

func TestNurBezahlteFunktionenSindFrei(t *testing.T) {
	priv := einrichten(t)
	kern.Laden(schluessel(t, priv, plizenz.Nutzlast{
		Inhaber:    "Sparsam",
		Funktionen: []string{string(kern.Versionen)},
	}))
	if !kern.Frei(kern.Versionen) {
		t.Fatal("die bezahlte Funktion fehlt")
	}
	if kern.Frei(kern.Anhaenge) || kern.Frei(kern.Pruefspur) {
		t.Fatal("nicht bezahlte Funktionen wurden freigeschaltet")
	}
}

func TestVeraenderterSchluesselWirdAbgelehnt(t *testing.T) {
	priv := einrichten(t)
	s := schluessel(t, priv, plizenz.Nutzlast{Inhaber: "Echt", Funktionen: alleNamen()})

	// Ein einzelnes Zeichen im Datenteil kippen. Die Signatur darf dann nicht
	// mehr passen -- das ist der Kern des ganzen Verfahrens.
	b := []byte(s)
	for i := range b {
		if b[i] == 'E' {
			b[i] = 'F'
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

func TestFremdeSignaturWirdAbgelehnt(t *testing.T) {
	einrichten(t)
	// Mit einem anderen Paar unterschrieben: inhaltlich einwandfrei, nur nicht
	// von uns. Genau das muss die Prüfung erkennen.
	_, fremd, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kern.Laden(schluessel(t, fremd, plizenz.Nutzlast{Inhaber: "Fälscher", Funktionen: alleNamen()}))
	if kern.Aktuell().Gueltig {
		t.Fatal("fremd signierter Schlüssel wurde angenommen")
	}
}

func TestAbgelaufenerSchluessel(t *testing.T) {
	priv := einrichten(t)
	kern.Laden(schluessel(t, priv, plizenz.Nutzlast{
		Inhaber:    "Gestern",
		Funktionen: alleNamen(),
		Ablauf:     time.Now().AddDate(0, 0, -3).Format("2006-01-02"),
	}))
	if kern.Aktuell().Gueltig {
		t.Fatal("abgelaufener Schlüssel wurde angenommen")
	}
}

func TestHeuteAblaufendeLizenzGiltNoch(t *testing.T) {
	priv := einrichten(t)
	kern.Laden(schluessel(t, priv, plizenz.Nutzlast{
		Inhaber:    "Letzter Tag",
		Funktionen: alleNamen(),
		Ablauf:     time.Now().Format("2006-01-02"),
	}))
	if !kern.Aktuell().Gueltig {
		t.Fatalf("eine heute ablaufende Lizenz muss heute noch gelten: %s", kern.Aktuell().Grund)
	}
}

func TestUnbekannteFunktionWirdVerworfen(t *testing.T) {
	priv := einrichten(t)
	// Ein Schlüssel aus einem neueren Generator darf nichts freischalten, was
	// dieser Stand gar nicht kann.
	kern.Laden(schluessel(t, priv, plizenz.Nutzlast{
		Inhaber:    "Zukunft",
		Funktionen: []string{"gibt-es-nicht", string(kern.Anhaenge)},
	}))
	if !kern.Frei(kern.Anhaenge) {
		t.Fatal("die bekannte Funktion hätte greifen müssen")
	}
	for _, f := range kern.Aktuell().Funktionen {
		if f == "gibt-es-nicht" {
			t.Fatal("unbekannter Name wurde durchgereicht")
		}
	}
}

func TestUnsinnWirdAbgelehnt(t *testing.T) {
	einrichten(t)
	for _, s := range []string{"quatsch", "a.b", "....", "eyJ9.eyJ9", ".", ""} {
		kern.Laden(s)
		if kern.Aktuell().Gueltig {
			t.Fatalf("Unsinn %q wurde angenommen", s)
		}
	}
}
