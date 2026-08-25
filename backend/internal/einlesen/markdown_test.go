package einlesen

import (
	"encoding/json"
	"strings"
	"testing"
)

// alsJSON macht aus Blöcken das, was in der Datenbank landet, die Tests
// prüfen gegen diese Gestalt, weil genau sie der Editor zu sehen bekommt.
func alsJSON(t *testing.T, b []Block) string {
	t.Helper()
	roh, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Blöcke nicht serialisierbar: %v", err)
	}
	return string(roh)
}

func TestTitelAusErsterUeberschrift(t *testing.T) {
	titel, _, bloecke := Lies("# Mein Bericht\n\nText darunter.\n")
	if titel != "Mein Bericht" {
		t.Fatalf("Titel = %q", titel)
	}
	// Die Überschrift darf nicht zusätzlich im Text stehen.
	if s := alsJSON(t, bloecke); strings.Contains(s, "Mein Bericht") {
		t.Fatalf("Titel steht doppelt: %s", s)
	}
}

func TestTitelAusVorspann(t *testing.T) {
	titel, kopf, _ := Lies("---\ntitle: Aus dem Kopf\ntags: [eins, zwei]\nicon: \"K\"\n---\n\n# Andere Überschrift\n\nText.\n")
	if titel != "Aus dem Kopf" {
		t.Fatalf("der Vorspann muss vorgehen, Titel = %q", titel)
	}
	if len(kopf.Tags) != 2 || kopf.Tags[0] != "eins" || kopf.Tags[1] != "zwei" {
		t.Fatalf("Tags = %v", kopf.Tags)
	}
	if kopf.Icon != "K" {
		t.Fatalf("Icon = %q", kopf.Icon)
	}
}

func TestVorspannAlsListe(t *testing.T) {
	_, kopf, _ := Lies("---\ntags:\n  - projekt\n  - #notiz\n---\nText\n")
	if len(kopf.Tags) != 2 || kopf.Tags[1] != "notiz" {
		t.Fatalf("Tags = %v", kopf.Tags)
	}
}

func TestUeberschriftenStufeBegrenzt(t *testing.T) {
	_, _, b := Lies("Text\n\n##### Tief\n")
	s := alsJSON(t, b)
	// Der Editor kennt nur drei Stufen; eine vierte würde die Seite
	// unlesbar machen, statt sie nur flacher zu zeigen.
	if !strings.Contains(s, `"level":3`) {
		t.Fatalf("Stufe nicht begrenzt: %s", s)
	}
}

func TestListeVerschachtelt(t *testing.T) {
	_, _, b := Lies("- Eins\n  - Unter\n- Zwei\n")
	if len(b) != 2 {
		t.Fatalf("erwartet zwei Einträge auf oberster Ebene, bekam %d", len(b))
	}
	if len(b[0].Children) != 1 || b[0].Children[0].Type != "bulletListItem" {
		t.Fatalf("Untereintrag fehlt: %s", alsJSON(t, b))
	}
}

func TestNummerierteListeMitAnfang(t *testing.T) {
	_, _, b := Lies("5. Fünf\n6. Sechs\n")
	if b[0].Props["start"] != 5 {
		t.Fatalf("Anfang nicht übernommen: %s", alsJSON(t, b))
	}
	// Der Editor zählt selbst weiter; eine zweite Angabe wäre eine zweite
	// Liste, die wieder bei sich anfängt.
	if _, da := b[1].Props["start"]; da {
		t.Fatalf("zweiter Eintrag trägt einen Anfang: %s", alsJSON(t, b))
	}
}

func TestKaestchenListe(t *testing.T) {
	_, _, b := Lies("- [x] erledigt\n- [ ] offen\n")
	if b[0].Type != "checkListItem" || b[0].Props["checked"] != true {
		t.Fatalf("Haken nicht erkannt: %s", alsJSON(t, b))
	}
	if b[1].Props["checked"] != false {
		t.Fatalf("leeres Kästchen falsch: %s", alsJSON(t, b))
	}
}

func TestAuszeichnungen(t *testing.T) {
	_, _, b := Lies("Ein **fetter** und *schräger* und ~~toter~~ Text.\n")
	s := alsJSON(t, b)
	for _, erwartet := range []string{`"bold":true`, `"italic":true`, `"strike":true`} {
		if !strings.Contains(s, erwartet) {
			t.Fatalf("%s fehlt in %s", erwartet, s)
		}
	}
}

func TestVerschachtelteAuszeichnung(t *testing.T) {
	_, _, b := Lies("**fett mit *schräg* darin**\n")
	teile := b[0].Content.([]Inline)
	var gefunden bool
	for _, t2 := range teile {
		if t2.Styles["bold"] && t2.Styles["italic"] {
			gefunden = true
		}
	}
	if !gefunden {
		t.Fatalf("geerbte Auszeichnung fehlt: %s", alsJSON(t, b))
	}
}

func TestUnterstrichImWortBleibt(t *testing.T) {
	_, _, b := Lies("Die Datei mein_lange_datei.txt bleibt heil.\n")
	teile := b[0].Content.([]Inline)
	if len(teile) != 1 || !strings.Contains(teile[0].Text, "mein_lange_datei.txt") {
		t.Fatalf("Unterstrich falsch gelesen: %s", alsJSON(t, b))
	}
}

func TestMaskierungWirdZurueckgenommen(t *testing.T) {
	// Genau so schreibt der Export ein Sternchen heraus, das keines sein soll.
	_, _, b := Lies(`2 \* 3 und \[Klammer\]` + "\n")
	teile := b[0].Content.([]Inline)
	if teile[0].Text != "2 * 3 und [Klammer]" {
		t.Fatalf("Maskierung nicht aufgelöst: %q", teile[0].Text)
	}
}

func TestCodeInZeile(t *testing.T) {
	_, _, b := Lies("Ruf `go test ./...` auf.\n")
	teile := b[0].Content.([]Inline)
	if len(teile) < 2 || !teile[1].Styles["code"] || teile[1].Text != "go test ./..." {
		t.Fatalf("Code nicht erkannt: %s", alsJSON(t, b))
	}
}

func TestCodeMitRueckstrichImInhalt(t *testing.T) {
	_, _, b := Lies("``a ` b``\n")
	teile := b[0].Content.([]Inline)
	if teile[0].Text != "a ` b" {
		t.Fatalf("Rückstrich im Code verloren: %q", teile[0].Text)
	}
}

func TestCodezaunWirdFesteSchrift(t *testing.T) {
	_, _, b := Lies("```go\nfunc a() {}\n\nfunc b() {}\n```\n")
	s := alsJSON(t, b)
	if !strings.Contains(s, `"code":true`) {
		t.Fatalf("Code nicht ausgezeichnet: %s", s)
	}
	if !strings.Contains(s, "func a() {}") || !strings.Contains(s, "func b() {}") {
		t.Fatalf("Codezeilen verloren: %s", s)
	}
	// Die Leerzeile mitten im Code ist Inhalt und muss bleiben.
	if len(b) != 4 {
		t.Fatalf("erwartet Sprache + drei Zeilen, bekam %d: %s", len(b), s)
	}
}

func TestVerweis(t *testing.T) {
	_, _, b := Lies("Siehe [die Seite](/pages/42).\n")
	teile := b[0].Content.([]Inline)
	if teile[1].Type != "link" || teile[1].Href != "/pages/42" {
		t.Fatalf("Verweis falsch: %s", alsJSON(t, b))
	}
	if NurText(teile[1].Content) != "die Seite" {
		t.Fatalf("Beschriftung falsch: %s", alsJSON(t, b))
	}
}

func TestVerweisMitLeerzeichenInSpitzenKlammern(t *testing.T) {
	// So schreibt der Export Dateinamen mit Leerzeichen.
	_, _, b := Lies("[Bericht](<mein bericht.md>)\n")
	teile := b[0].Content.([]Inline)
	if teile[0].Href != "mein bericht.md" {
		t.Fatalf("Adresse falsch: %q", teile[0].Href)
	}
}

func TestWikiVerweisBleibtText(t *testing.T) {
	_, _, b := Lies("Siehe [[Andere Seite]] dazu.\n")
	teile := b[0].Content.([]Inline)
	if len(teile) != 1 || !strings.Contains(teile[0].Text, "[[Andere Seite]]") {
		t.Fatalf("Wiki-Verweis zerlegt: %s", alsJSON(t, b))
	}
}

func TestBildAlleinWirdBildblock(t *testing.T) {
	_, _, b := Lies("![Ein Baum](bilder/baum.png)\n")
	if b[0].Type != "image" || b[0].Props["url"] != "bilder/baum.png" {
		t.Fatalf("kein Bildblock: %s", alsJSON(t, b))
	}
	if b[0].Props["caption"] != "Ein Baum" {
		t.Fatalf("Bildunterschrift fehlt: %s", alsJSON(t, b))
	}
}

func TestTabelle(t *testing.T) {
	_, _, b := Lies("| a | b |\n| --- | --- |\n| 1 | 2 |\n")
	if b[0].Type != "table" {
		t.Fatalf("keine Tabelle: %s", alsJSON(t, b))
	}
	inhalt := b[0].Content.(TabellenInhalt)
	if len(inhalt.Rows) != 2 || len(inhalt.Rows[0].Cells) != 2 {
		t.Fatalf("Tabellenform falsch: %s", alsJSON(t, b))
	}
	if NurText(inhalt.Rows[1].Cells[1]) != "2" {
		t.Fatalf("Zellinhalt falsch: %s", alsJSON(t, b))
	}
}

func TestTabelleMitStrichImText(t *testing.T) {
	_, _, b := Lies("| a | b |\n| --- | --- |\n| x \\| y | z |\n")
	inhalt := b[0].Content.(TabellenInhalt)
	if NurText(inhalt.Rows[1].Cells[0]) != "x | y" {
		t.Fatalf("maskierter Strich falsch: %s", alsJSON(t, b))
	}
}

func TestTabelleWirdRechteckig(t *testing.T) {
	_, _, b := Lies("| a | b | c |\n| --- | --- | --- |\n| 1 |\n")
	inhalt := b[0].Content.(TabellenInhalt)
	if len(inhalt.Rows[1].Cells) != 3 {
		t.Fatalf("kurze Zeile nicht aufgefüllt: %s", alsJSON(t, b))
	}
}

func TestSenkrechterStrichOhneTrennzeileIstKeineTabelle(t *testing.T) {
	_, _, b := Lies("Entweder a | b, das ist die Frage.\n")
	if b[0].Type != "paragraph" {
		t.Fatalf("aus einem Satz wurde eine Tabelle: %s", alsJSON(t, b))
	}
}

func TestWeicherUmbruchWirdLeerzeichen(t *testing.T) {
	_, _, b := Lies("erste Zeile\nzweite Zeile\n")
	teile := b[0].Content.([]Inline)
	if teile[0].Text != "erste Zeile zweite Zeile" {
		t.Fatalf("weicher Umbruch falsch: %q", teile[0].Text)
	}
}

func TestHarterUmbruchBleibt(t *testing.T) {
	_, _, b := Lies("oben  \nunten\n")
	teile := b[0].Content.([]Inline)
	if teile[0].Text != "oben\nunten" {
		t.Fatalf("harter Umbruch verloren: %q", teile[0].Text)
	}
}

func TestZitatWirdKursiv(t *testing.T) {
	_, _, b := Lies("> Ein Zitat\n> geht weiter\n")
	teile := b[0].Content.([]Inline)
	if !teile[0].Styles["italic"] {
		t.Fatalf("Zitat nicht ausgezeichnet: %s", alsJSON(t, b))
	}
	if !strings.Contains(teile[0].Text, "geht weiter") {
		t.Fatalf("zweite Zitatzeile verloren: %s", alsJSON(t, b))
	}
}

func TestListeDirektNachAbsatz(t *testing.T) {
	// Ohne Leerzeile dazwischen, das kommt in echten Notizen dauernd vor.
	_, _, b := Lies("Text darüber\n- Punkt\n")
	if len(b) != 2 || b[1].Type != "bulletListItem" {
		t.Fatalf("Liste im Absatz verschluckt: %s", alsJSON(t, b))
	}
}

func TestLeeresDokumentBleibtOeffenbar(t *testing.T) {
	// Der Editor lehnt eine leere Liste als Anfangsinhalt ab.
	_, _, b := Lies("")
	if len(b) != 1 || b[0].Type != "paragraph" {
		t.Fatalf("leeres Dokument = %s", alsJSON(t, b))
	}
}

func TestTrennerWirdSichtbar(t *testing.T) {
	_, _, b := Lies("oben\n\n---\n\nunten\n")
	if len(b) != 3 {
		t.Fatalf("Trenner verschluckt: %s", alsJSON(t, b))
	}
}
