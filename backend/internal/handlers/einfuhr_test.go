package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"nexora/internal/einlesen"
)

// hinUndZurueck is the check that counts: what the export writes the import has
// to read back as the same thing. Whoever only tests the reader on its own does
// not notice the two sides drifting apart.
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
	// The export escapes the asterisk, the import takes the escape back, the
	// export escapes again; the text itself must not change.
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
	// A link must not lead out of the archive. It can do no harm here, nothing is
	// written to disk, but it shall hit nothing either.
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
	// The editor has no code block; the export writes every line as a piece of
	// code. The indent must not be lost in the process, in code it is not
	// decoration.
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
	// A space at both ends is taken away again by every Markdown reader. The
	// export therefore has to pad, otherwise the code shrinks with every run.
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
	// Below the cover page: the two folders.
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
	// A Confluence export files its cover page as index.html. Without this it
	// would hang beside the folder as an ordinary page instead of above it.
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
	// Notion files the note about a folder BESIDE the folder, not inside it.
	// Without this case it would come into being twice: as an empty folder page
	// and as the file next to it.
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

// koerperVon returns the plain text of a page's blocks, so a test can ask
// whether a body survived at all without knowing the block format.
func koerperVon(sp *einfuhrSeite) string {
	roh, err := json.Marshal(sp.bloecke)
	if err != nil {
		return ""
	}
	return MarkdownAusInhalt(roh)
}

// Every page that has a file behind it must carry that file's text. This is
// the check for the complaint that an import "loses" pages: they are there,
// but empty, and an empty page looks like a lost one.
func TestPlanJedeSeiteBehaeltIhrenText(t *testing.T) {
	// The shape of a real Notion export: the note about a folder lies BESIDE
	// the folder, every name carries a 32 digit id, and it goes three levels
	// deep.
	dateien := []einfuhrDatei{
		{pfad: "Handbuch 1111111111111111111111111111aaaa.md",
			inhalt: []byte("# Handbuch\n\nDas Handbuch sagt hallo.\n")},
		{pfad: "Handbuch 1111111111111111111111111111aaaa/Technik 2222222222222222222222222222bbbb.md",
			inhalt: []byte("# Technik\n\nHier steht der Netzplan.\n")},
		{pfad: "Handbuch 1111111111111111111111111111aaaa/Technik 2222222222222222222222222222bbbb/Router 3333333333333333333333333333cccc.md",
			inhalt: []byte("# Router\n\nDer Router hat zwei Netze.\n")},
		{pfad: "Handbuch 1111111111111111111111111111aaaa/Notizen 4444444444444444444444444444dddd.md",
			inhalt: []byte("# Notizen\n\nEine lose Notiz.\n")},
	}
	plan := planen(dateien)

	will := map[string]string{
		"Handbuch": "Das Handbuch sagt hallo.",
		"Technik":  "Hier steht der Netzplan.",
		"Router":   "Der Router hat zwei Netze.",
		"Notizen":  "Eine lose Notiz.",
	}
	if len(plan) != len(will) {
		t.Fatalf("erwartet %d Seiten, bekam %d", len(will), len(plan))
	}
	for _, sp := range plan {
		text, gesucht := will[sp.titel]
		if !gesucht {
			t.Errorf("unerwartete Seite %q", sp.titel)
			continue
		}
		if k := koerperVon(sp); !strings.Contains(k, text) {
			t.Errorf("Seite %q hat ihren Text verloren, bekam %q", sp.titel, k)
		}
		delete(will, sp.titel)
	}
	for titel := range will {
		t.Errorf("Seite %q fehlt ganz", titel)
	}
}

// Two folders whose notes carry the same title must not take each other's
// text. The pages differ only by their id, and an import that keys on the
// title alone would empty one of them.
func TestPlanGleicherTitelZweiOrdner(t *testing.T) {
	plan := planen([]einfuhrDatei{
		{pfad: "Kunde A 1111111111111111111111111111aaaa.md",
			inhalt: []byte("# Rechnung\n\nRechnung von Kunde A.\n")},
		{pfad: "Kunde A 1111111111111111111111111111aaaa/Rechnung 5555555555555555555555555555eeee.md",
			inhalt: []byte("# Rechnung\n\nErste Rechnung.\n")},
		{pfad: "Kunde B 2222222222222222222222222222bbbb.md",
			inhalt: []byte("# Rechnung\n\nRechnung von Kunde B.\n")},
		{pfad: "Kunde B 2222222222222222222222222222bbbb/Rechnung 6666666666666666666666666666ffff.md",
			inhalt: []byte("# Rechnung\n\nZweite Rechnung.\n")},
	})
	if len(plan) != 4 {
		t.Fatalf("erwartet vier Seiten, bekam %d", len(plan))
	}
	gefunden := map[string]bool{}
	for _, sp := range plan {
		gefunden[strings.TrimSpace(koerperVon(sp))] = true
	}
	for _, text := range []string{"Rechnung von Kunde A.", "Erste Rechnung.", "Rechnung von Kunde B.", "Zweite Rechnung."} {
		if !gefunden[text] {
			t.Errorf("Text %q kam nirgends an", text)
		}
	}
}

// A folder note that lies INSIDE the folder and one that lies BESIDE it are
// two conventions for the same thing. When an archive holds both, one page
// must keep each text -- neither may fall away silently.
func TestPlanOrdnernotizDrinnenUndDaneben(t *testing.T) {
	plan := planen([]einfuhrDatei{
		{pfad: "Ordner.md", inhalt: []byte("# Ordner daneben\n\nText von daneben.\n")},
		{pfad: "Ordner/Ordner.md", inhalt: []byte("# Ordner drinnen\n\nText von drinnen.\n")},
	})
	gefunden := map[string]bool{}
	for _, sp := range plan {
		gefunden[strings.TrimSpace(koerperVon(sp))] = true
	}
	for _, text := range []string{"Text von daneben.", "Text von drinnen."} {
		if !gefunden[text] {
			t.Errorf("Text %q fiel weg, Plan: %d Seiten", text, len(plan))
		}
	}
}

// A page whose body holds nothing but a table must not come out empty: Notion
// files a database view that way, and a table is the whole content there.
func TestPlanSeiteNurAusTabelle(t *testing.T) {
	plan := planen([]einfuhrDatei{
		{pfad: "Bestand 7777777777777777777777777777aaaa.md",
			inhalt: []byte("# Bestand\n\n| Ding | Zahl |\n| --- | --- |\n| Schraube | 12 |\n")},
	})
	if len(plan) != 1 {
		t.Fatalf("erwartet eine Seite, bekam %d", len(plan))
	}
	if k := koerperVon(plan[0]); !strings.Contains(k, "Schraube") {
		t.Errorf("Tabelle verloren, bekam %q", k)
	}
}
