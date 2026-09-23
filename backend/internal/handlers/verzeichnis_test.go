package handlers

import "testing"

// The table of contents of our own export has to drop out when a whole space
// is imported, but a hand-written INHALT.md must not.
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

// A hand-written index consists of a heading and links and nothing else.
// Without the line carrying the date it is no export, and it does not drop out
// on import.
func TestVerzeichnisOhneDatumBleibt(t *testing.T) {
	vonHand := "# Meine Sammlung\n\n- [Eins](<eins.md>)\n- [Zwei](<zwei.md>)\n"
	if istAusfuhrVerzeichnis([]byte(vonHand)) {
		t.Error("eine eigene Seite aus lauter Verweisen wurde als Ausfuhr genommen und faellt damit weg")
	}
}
