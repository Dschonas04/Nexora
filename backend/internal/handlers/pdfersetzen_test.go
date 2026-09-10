package handlers

import "testing"

// The attachment has to be recognised as a PDF even when the browser sent no
// usable type along on upload -- out of an import some files carry no more than
// application/octet-stream.
func TestPDFWirdErkannt(t *testing.T) {
	faelle := []struct {
		mime, name string
		will       bool
	}{
		{"application/pdf", "vertrag.pdf", true},
		{"APPLICATION/PDF", "vertrag", true},
		{"application/octet-stream", "Vertrag.PDF", true},
		{"application/octet-stream", "vertrag.docx", false},
		{"image/png", "bild.png", false},
		{"", "", false},
		// No PDF, only a name pretending to be one: the extension sits in the
		// middle.
		{"text/plain", "vertrag.pdf.txt", false},
	}
	for _, f := range faelle {
		if got := istPDF(f.mime, f.name); got != f.will {
			t.Errorf("istPDF(%q, %q) = %v, erwartet %v", f.mime, f.name, got, f.will)
		}
	}
}
