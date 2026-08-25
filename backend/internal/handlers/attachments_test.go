package handlers

import "testing"

// Der Typ entscheidet über Vorschau und Volltextsuche. Kommt er nichtssagend
// an, und das tut er bei jeder Endung, die der Browser nicht kennt --, muss
// die Endung einspringen.
func TestTypAusAngabeUndName(t *testing.T) {
	faelle := []struct {
		angabe, name, will string
	}{
		{"application/pdf", "bericht.pdf", "application/pdf"},
		{"application/octet-stream", "bericht.pdf", "application/pdf"},
		{"", "bild.png", "image/png"},
		// Diese Endungen vergibt die eigene Liste, damit der Typ nicht
		// davon abhängt, welche Typtabelle auf dem Rechner liegt.
		{"binary/octet-stream", "notiz.md", "text/markdown"},
		{"", "protokoll.log", "text/plain"},
		{"", "nexora.conf", "text/plain"},
		{"", "werte.yaml", "text/plain"},
		{"text/plain; charset=utf-8", "a.txt", "text/plain"},
		// Sagt der Browser etwas Genaueres als die Endung, gilt seine Angabe.
		{"image/svg+xml", "zeichnung.svg", "image/svg+xml"},
		// Ohne Endung und ohne Angabe bleibt nur das Eingeständnis.
		{"", "readme", "application/octet-stream"},
	}
	for _, f := range faelle {
		if got := typAusAngabeUndName(f.angabe, f.name); got != f.will {
			t.Errorf("typAusAngabeUndName(%q, %q) = %q, erwartet %q", f.angabe, f.name, got, f.will)
		}
	}
}
