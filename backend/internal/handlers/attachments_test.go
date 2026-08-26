package handlers

import "testing"

// The type decides about preview and full text search. If it arrives saying
// nothing, and it does for every extension the browser does not know, the
// extension has to step in.
func TestTypAusAngabeUndName(t *testing.T) {
	faelle := []struct {
		angabe, name, will string
	}{
		{"application/pdf", "bericht.pdf", "application/pdf"},
		{"application/octet-stream", "bericht.pdf", "application/pdf"},
		{"", "bild.png", "image/png"},
		// These extensions are assigned by our own list so that the type does not
		// depend on which type table lies on the machine.
		{"binary/octet-stream", "notiz.md", "text/markdown"},
		{"", "protokoll.log", "text/plain"},
		{"", "nexora.conf", "text/plain"},
		{"", "werte.yaml", "text/plain"},
		{"text/plain; charset=utf-8", "a.txt", "text/plain"},
		// When the browser knows better than the extension, its claim wins.
		{"image/svg+xml", "zeichnung.svg", "image/svg+xml"},
		// With neither extension nor claim, only the admission is left.
		{"", "readme", "application/octet-stream"},
	}
	for _, f := range faelle {
		if got := typAusAngabeUndName(f.angabe, f.name); got != f.will {
			t.Errorf("typAusAngabeUndName(%q, %q) = %q, erwartet %q", f.angabe, f.name, got, f.will)
		}
	}
}
