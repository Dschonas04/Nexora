package einlesen

import (
	"encoding/json"
	"strings"
	"testing"
)

func htmlJSON(t *testing.T, b []Block) string {
	t.Helper()
	roh, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(roh)
}

func TestHTMLTitelAusUeberschrift(t *testing.T) {
	titel, b := LiesHTML(`<html><head><title>Ablage : Falscher Titel</title></head>
		<body><h1>Der richtige Titel</h1><p>Text.</p></body></html>`)
	if titel != "Der richtige Titel" {
		t.Fatalf("Titel = %q", titel)
	}
	// The heading must not appear in the body a second time.
	if s := htmlJSON(t, b); strings.Contains(s, "Der richtige Titel") {
		t.Fatalf("Titel steht doppelt: %s", s)
	}
}

func TestHTMLTitelAusKopfOhneAblage(t *testing.T) {
	// Confluence writes "Space name : Page name" into the header.
	titel, _ := LiesHTML(`<html><head><title>Technik : Netzplan</title></head><body><p>x</p></body></html>`)
	if titel != "Netzplan" {
		t.Fatalf("Titel = %q", titel)
	}
}

func TestHTMLAuszeichnungen(t *testing.T) {
	_, b := LiesHTML(`<p>Ein <strong>fetter</strong> und <em>schräger</em> und <code>fester</code> Text.</p>`)
	s := htmlJSON(t, b)
	for _, will := range []string{`"bold":true`, `"italic":true`, `"code":true`} {
		if !strings.Contains(s, will) {
			t.Fatalf("%s fehlt: %s", will, s)
		}
	}
}

func TestHTMLLeerzeichenZwischenStuecken(t *testing.T) {
	// Without respecting whitespace this glued "boldand" together.
	_, b := LiesHTML(`<p>Ein <b>fetter</b> und normaler Text.</p>`)
	if got := NurText(inhaltVon(b[0])); got != "Ein fetter und normaler Text." {
		t.Fatalf("Text = %q", got)
	}
}

func TestHTMLListeVerschachtelt(t *testing.T) {
	_, b := LiesHTML(`<ul><li>Eins<ul><li>Unter</li></ul></li><li>Zwei</li></ul>`)
	if len(b) != 2 {
		t.Fatalf("erwartet zwei Einträge, bekam %d: %s", len(b), htmlJSON(t, b))
	}
	if len(b[0].Children) != 1 || b[0].Children[0].Type != "bulletListItem" {
		t.Fatalf("Untereintrag fehlt: %s", htmlJSON(t, b))
	}
}

func TestHTMLNummerierteListe(t *testing.T) {
	_, b := LiesHTML(`<ol><li>Erstens</li><li>Zweitens</li></ol>`)
	if b[0].Type != "numberedListItem" {
		t.Fatalf("keine Nummerierung: %s", htmlJSON(t, b))
	}
}

func TestHTMLAufgabenliste(t *testing.T) {
	_, b := LiesHTML(`<ul><li><input type="checkbox" checked> fertig</li><li><input type="checkbox"> offen</li></ul>`)
	if b[0].Type != "checkListItem" || b[0].Props["checked"] != true {
		t.Fatalf("Haken falsch: %s", htmlJSON(t, b))
	}
	if b[1].Props["checked"] != false {
		t.Fatalf("leeres Kästchen falsch: %s", htmlJSON(t, b))
	}
}

func TestHTMLTabelleMitKopfzeile(t *testing.T) {
	_, b := LiesHTML(`<table><thead><tr><th>Name</th><th>Wert</th></tr></thead>
		<tbody><tr><td>a</td><td>1</td></tr></tbody></table>`)
	if b[0].Type != "table" {
		t.Fatalf("keine Tabelle: %s", htmlJSON(t, b))
	}
	inhalt := b[0].Content.(TabellenInhalt)
	if len(inhalt.Rows) != 2 || NurText(inhalt.Rows[1].Cells[1]) != "1" {
		t.Fatalf("Tabelleninhalt falsch: %s", htmlJSON(t, b))
	}
	// Header cells are bold, otherwise an imported table looks headless.
	if !inhalt.Rows[0].Cells[0][0].Styles["bold"] {
		t.Fatalf("Kopfzelle nicht ausgezeichnet: %s", htmlJSON(t, b))
	}
}

func TestHTMLTabelleWirdRechteckig(t *testing.T) {
	_, b := LiesHTML(`<table><tr><td>a</td><td>b</td><td>c</td></tr><tr><td>1</td></tr></table>`)
	inhalt := b[0].Content.(TabellenInhalt)
	if len(inhalt.Rows[1].Cells) != 3 {
		t.Fatalf("kurze Zeile nicht aufgefüllt: %s", htmlJSON(t, b))
	}
}

func TestHTMLCodeBlockBehaeltZeilen(t *testing.T) {
	_, b := LiesHTML("<pre><code>func a() {\n    b()\n}</code></pre>")
	if len(b) != 3 {
		t.Fatalf("erwartet drei Zeilen, bekam %d: %s", len(b), htmlJSON(t, b))
	}
	// Nicht über NurText prüfen, das schneidet Ränder ab, weil es Titel
	// liefert. Im Codeblock ist die Einrückung der Punkt.
	if got := inhaltVon(b[1])[0].Text; got != "    b()" {
		t.Fatalf("Einrückung verloren: %q", got)
	}
}

func TestHTMLVerweisUndBild(t *testing.T) {
	_, b := LiesHTML(`<p>Siehe <a href="/seite.html">dort</a>.</p><p><img src="bilder/x.png" alt="Ein Bild"></p>`)
	teile := inhaltVon(b[0])
	if teile[1].Type != "link" || teile[1].Href != "/seite.html" {
		t.Fatalf("Verweis falsch: %s", htmlJSON(t, b))
	}
	if b[1].Type != "image" || b[1].Props["url"] != "bilder/x.png" {
		t.Fatalf("Bild falsch: %s", htmlJSON(t, b))
	}
}

func TestHTMLZitatWirdKursiv(t *testing.T) {
	_, b := LiesHTML(`<blockquote><p>Ein Zitat</p></blockquote>`)
	if !inhaltVon(b[0])[0].Styles["italic"] {
		t.Fatalf("Zitat nicht ausgezeichnet: %s", htmlJSON(t, b))
	}
}

func TestHTMLSkriptUndStilFliegenRaus(t *testing.T) {
	_, b := LiesHTML(`<body><script>var a = 1;</script><style>p { color: red }</style><p>Nur das hier.</p></body>`)
	s := htmlJSON(t, b)
	if strings.Contains(s, "var a") || strings.Contains(s, "color") {
		t.Fatalf("Beiwerk übernommen: %s", s)
	}
}

func TestHTMLLeeresDokument(t *testing.T) {
	_, b := LiesHTML(`<html><body></body></html>`)
	if len(b) != 1 || b[0].Type != "paragraph" {
		t.Fatalf("leeres Dokument = %s", htmlJSON(t, b))
	}
}

func TestHTMLUeberschriftStufeBegrenzt(t *testing.T) {
	_, b := LiesHTML(`<h1>Titel</h1><h5>Tief</h5>`)
	s := htmlJSON(t, b)
	if !strings.Contains(s, `"level":3`) {
		t.Fatalf("Stufe nicht begrenzt: %s", s)
	}
}
