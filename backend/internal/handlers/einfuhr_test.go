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
	// zurück, der Export maskiert wieder, am Text darf sich nichts ändern.
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
	// nichts anrichten, es wird nichts auf die Platte geschrieben --, aber
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

func TestRundlaufCodeBehaeltEinrueckung(t *testing.T) {
	// Der Editor hat keinen Codeblock; der Export schreibt jede Zeile als
	// Codestück. Die Einrückung darf dabei nicht verloren gehen, in Code
	// ist sie kein Schmuck.
	md := "```go\nfunc a() {\n    b()\n}\n```\n"
	einmal := hinUndZurueck(t, md)
	if !strings.Contains(einmal, "    b()") {
		t.Fatalf("Einrückung nach dem ersten Lauf weg:\n%s", einmal)
	}
	zweimal := hinUndZurueck(t, einmal)
	if !strings.Contains(zweimal, "    b()") {
		t.Fatalf("Einrückung nach dem zweiten Lauf weg:\n%s", zweimal)
	}
}

func TestRundlaufCodeMitRandLeerzeichen(t *testing.T) {
	// Ein Leerzeichen an beiden Enden nimmt jeder Markdown-Leser wieder weg.
	// Der Export muss deshalb füllen, sonst schrumpft der Code bei jedem Lauf.
	md := "Vorher `  eng  ` nachher.\n"
	zurueck := hinUndZurueck(t, hinUndZurueck(t, md))
	if !strings.Contains(zurueck, "  eng  ") {
		t.Fatalf("Randleerzeichen verloren:\n%s", zurueck)
	}
}

func TestNotionKennungFaelltWeg(t *testing.T) {
	faelle := map[string]string{
		"Wochenplan 8f3a1b2c4d5e6f708192a3b4c5d6e7f8": "Wochenplan",
		"Ein Ordner-8f3a1b2c4d5e6f708192a3b4c5d6e7f8": "Ein Ordner",
		"Ganz normal":  "Ganz normal",
		"Bericht 2026": "Bericht 2026",
		"Nicht hex zzza1b2c4d5e6f708192a3b4c5d6e7f8": "Nicht hex zzza1b2c4d5e6f708192a3b4c5d6e7f8",
	}
	for rein, raus := range faelle {
		if got := sauberterTitel(rein); got != raus {
			t.Errorf("sauberterTitel(%q) = %q, erwartet %q", rein, got, raus)
		}
	}
}

func TestPlanBautDenBaum(t *testing.T) {
	plan := planen([]einfuhrDatei{
		{pfad: "INHALT.md", inhalt: []byte("# Deckblatt\n")},
		{pfad: "Projekte/index.md", inhalt: []byte("# Alle Projekte\n")},
		{pfad: "Projekte/Ofen.md", inhalt: []byte("# Ofen\n")},
		{pfad: "Notizen/eins.md", inhalt: []byte("# Eins\n")},
	})
	if len(plan) != 5 {
		t.Fatalf("erwartet fünf Seiten (drei Dateien, ein Ordner ohne Index, ein Deckblatt), bekam %d", len(plan))
	}
	baum := baumAusPlan(plan)
	if len(baum) != 1 || baum[0].Titel != "Deckblatt" {
		t.Fatalf("Deckblatt nicht oben: %+v", baum)
	}
	// Unter dem Deckblatt: die beiden Ordner.
	namen := map[string]bool{}
	for _, k := range baum[0].Kinder {
		namen[k.Titel] = true
	}
	if !namen["Alle Projekte"] || !namen["Notizen"] {
		t.Fatalf("Ordnerseiten fehlen: %+v", baum[0].Kinder)
	}
}

func TestPlanOhneDeckblattHaengtObenAn(t *testing.T) {
	plan := planen([]einfuhrDatei{
		{pfad: "eins.md", inhalt: []byte("# Eins\n")},
		{pfad: "zwei.md", inhalt: []byte("# Zwei\n")},
	})
	baum := baumAusPlan(plan)
	if len(baum) != 2 {
		t.Fatalf("erwartet zwei Wurzeln, bekam %+v", baum)
	}
}

func TestPlanLiestAuchHTML(t *testing.T) {
	plan := planen([]einfuhrDatei{
		{pfad: "seite.html", inhalt: []byte("<html><body><h1>Aus HTML</h1><p>Text</p></body></html>")},
	})
	if len(plan) != 1 || plan[0].titel != "Aus HTML" {
		t.Fatalf("HTML nicht gelesen: %+v", plan)
	}
}

func TestPlanNimmtIndexHTMLAlsDeckblatt(t *testing.T) {
	// Eine Confluence-Ausfuhr legt ihr Deckblatt als index.html ab. Ohne das
	// hinge es als gewöhnliche Seite neben dem Ordner statt darüber.
	plan := planen([]einfuhrDatei{
		{pfad: "index.html", inhalt: []byte("<h1>Startseite</h1>")},
		{pfad: "Technik/netzplan.html", inhalt: []byte("<h1>Netzplan</h1>")},
	})
	baum := baumAusPlan(plan)
	if len(baum) != 1 || baum[0].Titel != "Startseite" {
		t.Fatalf("Deckblatt nicht oben: %+v", baum)
	}
	if len(baum[0].Kinder) != 1 || baum[0].Kinder[0].Titel != "Technik" {
		t.Fatalf("Ordner nicht darunter: %+v", baum[0].Kinder)
	}
}

func TestPlanNimmtNotionOrdnernotizDaneben(t *testing.T) {
	// Notion legt die Notiz zum Ordner NEBEN den Ordner, nicht hinein. Ohne
	// diesen Fall entstünde sie zweimal: als leere Ordnerseite und als Datei
	// daneben.
	plan := planen([]einfuhrDatei{
		{pfad: "Wochenplan 8f3a1b2c4d5e6f708192a3b4c5d6e7f8.md", inhalt: []byte("# Wochenplan\n")},
		{pfad: "Wochenplan 8f3a1b2c4d5e6f708192a3b4c5d6e7f8/Unterpunkt 1234567890abcdef1234567890abcdef.md",
			inhalt: []byte("# Unterpunkt\n")},
	})
	if len(plan) != 2 {
		t.Fatalf("erwartet zwei Seiten, bekam %d: %+v", len(plan), plan)
	}
	baum := baumAusPlan(plan)
	if len(baum) != 1 || baum[0].Titel != "Wochenplan" {
		t.Fatalf("Ordnernotiz nicht als Ordnerseite: %+v", baum)
	}
	if len(baum[0].Kinder) != 1 || baum[0].Kinder[0].Titel != "Unterpunkt" {
		t.Fatalf("Unterseite fehlt: %+v", baum[0])
	}
}
