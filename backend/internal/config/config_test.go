package config

import (
	"os"
	"path/filepath"
	"testing"
)

// schreibe legt eine Testkonfiguration an und gibt ihren Pfad zurück.
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
	if k.SitzungTage != 7 || k.Port != "8080" {
		t.Fatalf("Vorgaben verloren: %d Tage, Port %q", k.SitzungTage, k.Port)
	}
}

func TestKaputteZeilenKippenNichts(t *testing.T) {
	// Eine unlesbare Zahl, eine Zeile ohne =, ein Abschnitt: nichts davon darf
	// die übrigen Werte verlieren. Eine Konfiguration mit einem Tippfehler soll
	// den Server nicht am Starten hindern.
	k := Laden(schreibe(t, `
[Abschnitt]
sitzung_tage = keine-zahl
kaputte zeile ohne gleichheitszeichen
; Strichpunkt-Kommentar
such_woerterbuch = simple
`))
	if k.SitzungTage != 7 {
		t.Fatalf("unlesbare Zahl hätte die Vorgabe behalten müssen, bekam %d", k.SitzungTage)
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
	// Der wichtigste Fall: ohne jede Konfiguration muss ein brauchbarer
	// Zustand herauskommen, sonst startet eine frische Installation nicht.
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
