package handlers

import "testing"

func TestIstTextDatei(t *testing.T) {
	for _, f := range []struct {
		mime, name string
		text       bool
	}{
		{"application/x-sh", "start.sh", true},
		{"application/x-msdos-program", "Einrichten.bat", true},
		{"application/octet-stream", "notizen.md", true},
		{"application/octet-stream", "Dockerfile", true},
		{"", ".gitignore", true},
		{"text/plain", "irgendwas", true},
		{"application/octet-stream", "C:\\pfad\\skript.PS1", true},
		{"application/pdf", "vertrag.pdf", false},
		{"application/octet-stream", "bild.png", false},
		{"application/zip", "paket.zip", false},
	} {
		if got := istTextDatei(f.mime, f.name); got != f.text {
			t.Errorf("istTextDatei(%q, %q) = %v, erwartet %v", f.mime, f.name, got, f.text)
		}
	}
}
