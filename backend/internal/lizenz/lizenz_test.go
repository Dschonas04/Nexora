package lizenz_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	kern "nexora/internal/lizenz"
	plizenz "nexora/premium/lizenz"
)

// The test generates its own key pair and issues its licences itself.
//
// A ready made key in the repository would be a key for everybody who clones
// it, and because checking happens offline it could never be withdrawn. As a
// side effect the test this way covers the whole way from signing to unlocking
// instead of only half the distance.
func einrichten(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	oeff, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kern.Registriere(plizenz.NeuerPruefer(oeff))
	return priv
}

// schluessel signs a payload and assembles the finished key from it.
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

	// Flip a single character in the data part. The signature must then no longer
	// match, that is the core of the whole procedure.
	//
	// Flipped at a fixed position rather than by searching for a particular
	// letter: which characters occur in the Base64 depends on the content, and a
	// letter that is not found would have left the key unchanged, so the test
	// would have been green without ever checking anything.
	punkt := strings.Index(s, ".")
	if punkt < 2 {
		t.Fatal("Schlüssel hat nicht die erwartete Form")
	}
	b := []byte(s)
	if b[1] == 'A' {
		b[1] = 'B'
	} else {
		b[1] = 'A'
	}
	if string(b) == s {
		t.Fatal("die Manipulation hat nichts verändert")
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
	// Signed with a different pair: sound in content, only not by us. That is
	// exactly what the check has to notice.
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
	// A key from a newer generator must unlock nothing this version cannot do at
	// all.
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
