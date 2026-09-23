package config

import (
	"os"
	"path/filepath"
	"testing"
)

// An installation from before the rename must keep booting: the old German key
// is read, and the old environment variable too.
func TestAlteSchreibweisenGeltenWeiter(t *testing.T) {
	verzeichnis := t.TempDir()
	pfad := filepath.Join(verzeichnis, "config.conf")
	inhalt := "such_woerterbuch = english\npapierkorb_tage = 7\nregistrierung_offen = nein\n"
	if err := os.WriteFile(pfad, []byte(inhalt), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEXORA_MAX_ANHANG_MB", "42")

	k := Laden(pfad)

	if k.SuchWoerterbuch != "english" {
		t.Errorf("such_woerterbuch nicht gelesen: %q", k.SuchWoerterbuch)
	}
	if k.PapierkorbTage != 7 {
		t.Errorf("papierkorb_tage nicht gelesen: %d", k.PapierkorbTage)
	}
	if k.RegistrierungOffen {
		t.Error("registrierung_offen = nein wurde nicht gelesen")
	}
	if k.MaxAnhangMB != 42 {
		t.Errorf("NEXORA_MAX_ANHANG_MB nicht gelesen: %d", k.MaxAnhangMB)
	}
}

// The English names are the canonical ones and win where both are present.
func TestEnglischGewinnt(t *testing.T) {
	verzeichnis := t.TempDir()
	pfad := filepath.Join(verzeichnis, "config.conf")
	inhalt := "such_woerterbuch = german\nsearch_dictionary = english\n"
	if err := os.WriteFile(pfad, []byte(inhalt), 0o600); err != nil {
		t.Fatal(err)
	}
	if k := Laden(pfad); k.SuchWoerterbuch != "english" {
		t.Errorf("erwartet english, bekommen %q", k.SuchWoerterbuch)
	}
}
