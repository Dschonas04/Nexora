package config

import (
	"os"
	"path/filepath"
	"testing"
)

// schreibe writes a test configuration and returns its path.
func schreibe(t *testing.T, inhalt string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.conf")
	if err := os.WriteFile(p, []byte(inhalt), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUmgebungSchlaegtDatei(t *testing.T) {
	p := schreibe(t, "port = 9999\nsuch_woerterbuch = english\n")
	t.Setenv("PORT", "7777")

	k := Laden(p)
	if k.Port != "7777" {
		t.Fatalf("Umgebung sollte gewinnen, bekam %q", k.Port)
	}
	if k.SuchWoerterbuch != "english" {
		t.Fatalf("Dateiwert fehlt, bekam %q", k.SuchWoerterbuch)
	}
}

func TestVorgabeBleibtOhneEintrag(t *testing.T) {
	k := Laden(schreibe(t, "# nur ein Kommentar\n"))
	if k.SitzungStunden != 12 || k.Port != "8080" {
		t.Fatalf("Vorgaben verloren: %d Stunden, Port %q", k.SitzungStunden, k.Port)
	}
}

func TestKaputteZeilenKippenNichts(t *testing.T) {
	// An unreadable number, a line without =, a section: none of it may lose the
	// remaining values. A configuration with a typo shall not keep the server from
	// starting.
	k := Laden(schreibe(t, `
[Abschnitt]
sitzung_stunden = keine-zahl
kaputte zeile ohne gleichheitszeichen
; Strichpunkt-Kommentar
such_woerterbuch = simple
`))
	if k.SitzungStunden != 12 {
		t.Fatalf("unlesbare Zahl hätte die Vorgabe behalten müssen, bekam %d", k.SitzungStunden)
	}
	if k.SuchWoerterbuch != "simple" {
		t.Fatalf("Wert nach der kaputten Zeile verloren: %q", k.SuchWoerterbuch)
	}
}

func TestAnfuehrungszeichenErhaltenRand(t *testing.T) {
	k := Laden(schreibe(t, "jwt_geheimnis = \"  mit Rand  \"\n"))
	if k.JWTGeheimnis != "  mit Rand  " {
		t.Fatalf("Rand-Leerzeichen verloren: %q", k.JWTGeheimnis)
	}
}

func TestJaNeinSchreibweisen(t *testing.T) {
	for _, wert := range []string{"ja", "true", "an", "1", "yes", "on"} {
		if k := Laden(schreibe(t, "ldap_aktiv = "+wert+"\n")); !k.LDAPAktiv {
			t.Fatalf("%q hätte wahr sein müssen", wert)
		}
	}
	for _, wert := range []string{"nein", "false", "aus", "0", "no", "off"} {
		if k := Laden(schreibe(t, "registrierung_offen = "+wert+"\n")); k.RegistrierungOffen {
			t.Fatalf("%q hätte falsch sein müssen", wert)
		}
	}
}

func TestListeWirdGetrennt(t *testing.T) {
	k := Laden(schreibe(t, "erlaubte_domaenen = example.de , tochter.example.de,\n"))
	if len(k.ErlaubteDomaenen) != 2 {
		t.Fatalf("erwartete 2 Domänen, bekam %v", k.ErlaubteDomaenen)
	}
}

func TestFehlendeDateiIstKeinFehler(t *testing.T) {
	// The most important case: without any configuration at all a usable state
	// has to come out, otherwise a fresh installation does not start.
	k := Laden(filepath.Join(t.TempDir(), "gibt-es-nicht.conf"))
	if k.Port == "" || k.DatenbankURL == "" {
		t.Fatal("Vorgaben fehlen, wenn keine Datei da ist")
	}
}

func TestWarnungenGreifen(t *testing.T) {
	k := Laden(schreibe(t, `
ldap_aktiv = ja
ldap_server = ldap://ohne-tls.example.local
ldap_starttls = nein
ldap_tls_pruefen = nein
`))
	w := k.Warnungen()
	if len(w) < 3 {
		t.Fatalf("erwartete Warnungen zu Vorgabegeheimnis, TLS und Zertifikat, bekam %v", w)
	}
}

// An existing file still names the duration in days. It has to keep applying,
// otherwise a week would silently drop to twelve hours as soon as somebody
// installs the new version.
func TestAlteAngabeInTagenGiltWeiter(t *testing.T) {
	k := Laden(schreibe(t, "sitzung_tage = 7\n"))
	if k.SitzungStunden != 168 {
		t.Fatalf("sieben Tage sind 168 Stunden, bekam %d", k.SitzungStunden)
	}
}

// If both stand there, the value in days wins: it is the one that was set
// deliberately, while the new one may merely be the default in a fresh file.
func TestNeueAngabeAlleinReicht(t *testing.T) {
	k := Laden(schreibe(t, "sitzung_stunden = 4\n"))
	if k.SitzungStunden != 4 {
		t.Fatalf("vier Stunden erwartet, bekam %d", k.SitzungStunden)
	}
}
