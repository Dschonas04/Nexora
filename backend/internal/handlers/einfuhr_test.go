package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"nexora/internal/einlesen"
)

// hinUndZurueck ist die Probe, die zählt: was der Export schreibt, muss der
// Import wieder als dasselbe lesen. Wer nur den Leser für sich prüft, merkt
// nicht, dass die beiden Seiten auseinanderlaufen.
func hinUndZurueck(t *testing.T, md string) string {
	t.Helper()
	_, _, bloecke := einlesen.Lies(md)
	roh, err := json.Marshal(bloecke)
	if err != nil {
		t.Fatalf("Blöcke nicht serialisierbar: %v", err)
	}
	return MarkdownAusInhalt(roh)
}

func TestRundlaufAbsaetzeUndListen(t *testing.T) {
	md := "Ein Absatz mit **fett** und *schräg*.\n\n- Eins\n  - Unter\n- Zwei\n\n1. Erstens\n2. Zweitens\n"
	zurueck := hinUndZurueck(t, md)
	for _, erwartet := range []string{"**fett**", "*schräg*", "- Eins", "  - Unter", "1. Erstens", "2. Zweitens"} {
		if !strings.Contains(zurueck, erwartet) {
			t.Fatalf("%q fehlt nach dem Rundlauf:\n%s", erwartet, zurueck)
		}
	}
}

func TestRundlaufTabelle(t *testing.T) {
	md := "| Name | Wert |\n| --- | --- |\n| a | 1 |\n"
	zurueck := hinUndZurueck(t, md)
	if !strings.Contains(zurueck, "| Name | Wert |") || !strings.Contains(zurueck, "| a | 1 |") {
		t.Fatalf("Tabelle überlebt den Rundlauf nicht:\n%s", zurueck)
	}
}

func TestRundlaufKaestchen(t *testing.T) {
	zurueck := hinUndZurueck(t, "- [x] fertig\n- [ ] offen\n")
	if !strings.Contains(zurueck, "- [x] fertig") || !strings.Contains(zurueck, "- [ ] offen") {
		t.Fatalf("Kästchen verloren:\n%s", zurueck)
	}
}

func TestRundlaufWikiVerweis(t *testing.T) {
	zurueck := hinUndZurueck(t, "Siehe [[Andere Seite]].\n")
	if !strings.Contains(zurueck, "[[Andere Seite]]") {
		t.Fatalf("Wiki-Verweis verloren:\n%s", zurueck)
	}
}

func TestRundlaufMaskierung(t *testing.T) {
	// Der Export maskiert das Sternchen, der Import nimmt die Maskierung
	// zurück, der Export maskiert wieder -- am Text darf sich nichts ändern.
	zurueck := hinUndZurueck(t, `2 \* 3 ist 6`+"\n")
	if !strings.Contains(zurueck, `2 \* 3 ist 6`) {
		t.Fatalf("Maskierung nicht stabil:\n%s", zurueck)
	}
}

func TestZielPfadRelativ(t *testing.T) {
	faelle := []struct{ adresse, verz, will string }{
		{"andere.md", "", "andere.md"},
		{"andere.md", "notizen", "notizen/andere.md"},
		{"bilder/baum.png", "notizen", "notizen/bilder/baum.png"},
		{"mein%20bild.png", "", "mein bild.png"},
		{"/oben.md", "notizen", "oben.md"},
		{"andere.md#kapitel", "", "andere.md"},
		{"https://example.org/a.md", "", ""},
		{"#nur-anker", "", ""},
		{"mailto:jemand@example.org", "", ""},
	}
	for _, f := range faelle {
		if got := zielPfad(f.adresse, f.verz); got != f.will {
			t.Errorf("zielPfad(%q, %q) = %q, erwartet %q", f.adresse, f.verz, got, f.will)
		}
	}
}

func TestZielPfadBleibtImArchiv(t *testing.T) {
	// Ein Verweis darf nicht aus dem Archiv herausführen. Er kann hier zwar
	// nichts anrichten -- es wird nichts auf die Platte geschrieben --, aber
	// er soll auch nichts treffen.
	if p := zielPfad("../../etc/passwd", "a/b"); strings.HasPrefix(p, "..") {
		t.Fatalf("Pfad führt aus dem Archiv: %q", p)
	}
}

func packe(t *testing.T, dateien map[string]string) ([]byte, int64) {
	t.Helper()
	var puffer bytes.Buffer
	zw := zip.NewWriter(&puffer)
	for name, inhalt := range dateien {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(inhalt))
	}
	zw.Close()
	return puffer.Bytes(), int64(puffer.Len())
}

func TestArchivTrenntMarkdownVonBeilagen(t *testing.T) {
	roh, groesse := packe(t, map[string]string{
		"notizen/eins.md":      "# Eins",
		"notizen/bild.png":     "\x89PNG",
		"__MACOSX/notizen/._x": "müll",
		".DS_Store":            "müll",
	})
	md, beilagen, _ := archivLesen(bytes.NewReader(roh), groesse)
	if len(md) != 1 || md[0].pfad != "notizen/eins.md" {
		t.Fatalf("Markdown falsch erkannt: %+v", md)
	}
	if len(beilagen) != 1 {
		t.Fatalf("Beilagen falsch: %+v", beilagen)
	}
	if _, da := beilagen["notizen/bild.png"]; !da {
		t.Fatalf("Bild fehlt: %+v", beilagen)
	}
}

func TestArchivWeistPfadeMitPunktenAb(t *testing.T) {
	roh, groesse := packe(t, map[string]string{"../draussen.md": "# Nein"})
	md, _, warnungen := archivLesen(bytes.NewReader(roh), groesse)
	if len(md) != 0 {
		t.Fatalf("Datei außerhalb des Archivs übernommen: %+v", md)
	}
	if len(warnungen) == 0 {
		t.Fatal("kein Hinweis auf den verworfenen Eintrag")
	}
}

func TestArchivIstSortiert(t *testing.T) {
	roh, groesse := packe(t, map[string]string{
		"b/zwei.md": "# Zwei",
		"a/eins.md": "# Eins",
		"oben.md":   "# Oben",
	})
	md, _, _ := archivLesen(bytes.NewReader(roh), groesse)
	if len(md) != 3 || md[0].pfad != "a/eins.md" || md[2].pfad != "oben.md" {
		t.Fatalf("Reihenfolge falsch: %+v", md)
	}
}

func TestIstMarkdown(t *testing.T) {
	for _, ja := range []string{"a.md", "A.MD", "b.markdown", "c.txt"} {
		if !istMarkdown(ja) {
			t.Errorf("%s sollte Markdown sein", ja)
		}
	}
	for _, nein := range []string{"a.png", "b.pdf", "c"} {
		if istMarkdown(nein) {
			t.Errorf("%s sollte kein Markdown sein", nein)
		}
	}
}
