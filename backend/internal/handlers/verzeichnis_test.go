package handlers

import "testing"

// Das Inhaltsverzeichnis der eigenen Ausfuhr muss beim Einlesen einer ganzen
// Ablage wegfallen -- ein selbst geschriebenes INHALT.md aber nicht.
func TestIstAusfuhrVerzeichnis(t *testing.T) {
	unser := "# Reise\n\n3 Seiten, ausgegeben am 24.08.2026 09:12.\n\n" +
		"- [Hinfahrt](<hinfahrt.md>)\n- [Unterwegs](<unterwegs.md>)\n"
	if !istAusfuhrVerzeichnis([]byte(unser)) {
		t.Error("eigene Ausfuhr nicht erkannt")
	}
	eigenes := []string{
		"# Inhalt\n\nHier steht, was wir vorhaben.\n\n- [Hinfahrt](<hinfahrt.md>)\n",
		"# Inhalt\n\n- Stichpunkt ohne Verweis\n",
		"Kein Titel, nur Text.\n",
		"# Leer\n",
	}
	for _, e := range eigenes {
		if istAusfuhrVerzeichnis([]byte(e)) {
			t.Errorf("fremdes Verzeichnis faelschlich erkannt: %q", e)
		}
	}
}
