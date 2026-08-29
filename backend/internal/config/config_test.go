package config

import (
	"os"
	"path/filepath"
	"strings"
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

// Without a setting of its own the attachments stay where they have always
// been. An upgrade must not look for them in a different directory, otherwise
// every file uploaded so far would be gone at once.
func TestAnhangOrtFaelltAufDatenVerzeichnisZurueck(t *testing.T) {
	k := Laden(schreibe(t, "daten_verzeichnis = /var/nexora\n"))
	if k.AnhangOrt() != "/var/nexora" {
		t.Fatalf("ohne anhang_verzeichnis gilt daten_verzeichnis, bekam %q", k.AnhangOrt())
	}
}

// Set, it wins, and only for the attachments: the working directory stays
// where it is, so the configuration backups do not wander along.
func TestAnhangVerzeichnisSchlaegtDatenVerzeichnis(t *testing.T) {
	k := Laden(schreibe(t, `
daten_verzeichnis = /var/nexora
anhang_verzeichnis = /mnt/ablage
`))
	if k.AnhangOrt() != "/mnt/ablage" {
		t.Fatalf("anhang_verzeichnis erwartet, bekam %q", k.AnhangOrt())
	}
	if k.DatenVerzeich != "/var/nexora" {
		t.Fatalf("daten_verzeichnis darf unberührt bleiben, bekam %q", k.DatenVerzeich)
	}
}

// The disk as a stopgap for the object store is off unless it is asked for, and
// asking for it is worth a warning: the attachments then lie in two places.
func TestRueckfallAufDiePlatteIstAus(t *testing.T) {
	k := Laden(schreibe(t, "s3_aktiv = ja\ns3_endpunkt = 10.0.0.9:9000\n"))
	if k.S3Rueckfall {
		t.Fatal("s3_rueckfall muss ohne Angabe aus sein")
	}
	mit := Laden(schreibe(t, "s3_aktiv = ja\ns3_endpunkt = 10.0.0.9:9000\ns3_rueckfall = ja\n"))
	gewarnt := false
	for _, w := range mit.Warnungen() {
		if strings.Contains(w, "s3_rueckfall") {
			gewarnt = true
		}
	}
	if !gewarnt {
		t.Fatalf("erwartete Warnung zum Rückfall, bekam %v", mit.Warnungen())
	}
}
