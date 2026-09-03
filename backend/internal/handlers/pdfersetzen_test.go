package handlers

import "testing"

// Der Anhang muss als PDF erkannt werden, auch wenn der Browser beim Hochladen
// keinen brauchbaren Typ mitgeschickt hat -- aus einer Einfuhr traegt manche
// Datei nur application/octet-stream.
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
		// Kein PDF, nur ein Name, der so tut: die Endung steht mitten drin.
		{"text/plain", "vertrag.pdf.txt", false},
	}
	for _, f := range faelle {
		if got := istPDF(f.mime, f.name); got != f.will {
			t.Errorf("istPDF(%q, %q) = %v, erwartet %v", f.mime, f.name, got, f.will)
		}
	}
}
