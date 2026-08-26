package dok

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// bauDocx assembles a .docx from its parts. Written by hand instead of taken
// from a fixture file: the cases below are about single elements of the format,
// and a binary in the repository would hide exactly which one is under test.
func bauDocx(t *testing.T, teile map[string]string, medien map[string][]byte) []byte {
	t.Helper()
	var puffer bytes.Buffer
	zw := zip.NewWriter(&puffer)
	for name, inhalt := range teile {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(inhalt)); err != nil {
			t.Fatal(err)
		}
	}
	for name, daten := range medien {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(daten); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return puffer.Bytes()
}

func dokumentXML(body string) string {
	return `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		body + `</w:body></w:document>`
}

func wLaufXML(text string) string {
	return `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r>`
}

// Text inside a link, a tracked insertion, a content control or a smart tag is
// text of the document. Reading only the direct runs of a paragraph dropped all
// four, and a sentence lost its middle without anybody noticing.
func TestWordLiestVerpackteLaeufe(t *testing.T) {
	body := `<w:p>` + wLaufXML("Siehe ") +
		`<w:hyperlink r:id="rId9" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` + wLaufXML("die Anleitung") + `</w:hyperlink>` +
		wLaufXML(" und ") +
		`<w:ins>` + wLaufXML("das Neue") + `</w:ins>` +
		wLaufXML(", ") +
		`<w:sdt><w:sdtContent>` + wLaufXML("das Feld") + `</w:sdtContent></w:sdt>` +
		wLaufXML(" sowie ") +
		`<w:smartTag>` + wLaufXML("den Namen") + `</w:smartTag>` +
		`<w:del>` + wLaufXML("das Gestrichene") + `</w:del>` +
		`</w:p>`
	rels := `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
	  <Relationship Id="rId9" Target="https://example.org/anleitung" TargetMode="External"/></Relationships>`

	d, err := AusWord(bauDocx(t, map[string]string{
		"word/document.xml":            dokumentXML(body),
		"word/_rels/document.xml.rels": rels,
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Absatz) != 1 {
		t.Fatalf("erwartet 1 Absatz, bekam %d", len(d.Absatz))
	}
	text := nurText(d.Absatz[0].Text)
	for _, will := range []string{"Siehe", "die Anleitung", "das Neue", "das Feld", "den Namen"} {
		if !strings.Contains(text, will) {
			t.Errorf("%q fehlt in %q", will, text)
		}
	}
	if strings.Contains(text, "Gestrichene") {
		t.Errorf("gestrichener Text steht noch da: %q", text)
	}
	var verweis string
	for _, s := range d.Absatz[0].Text {
		if s.Text == "die Anleitung" {
			verweis = s.Verweis
		}
	}
	if verweis != "https://example.org/anleitung" {
		t.Errorf("Verweis fehlt oder falsch: %q", verweis)
	}
}

// A numbered list has to stay numbered. Which it is stands in numbering.xml,
// two steps away from the paragraph; without reading it every instruction in
// five steps came out as five bullets.
func TestWordNummerierungUndVerschachtelung(t *testing.T) {
	absatz := func(numID, ebene, text string) string {
		return `<w:p><w:pPr><w:numPr><w:ilvl w:val="` + ebene + `"/><w:numId w:val="` + numID + `"/></w:numPr></w:pPr>` + wLaufXML(text) + `</w:p>`
	}
	body := absatz("1", "0", "Erstens") + absatz("1", "1", "Untereintrag") + absatz("1", "0", "Zweitens") +
		absatz("2", "0", "Punkt")
	num := `<?xml version="1.0"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	  <w:abstractNum w:abstractNumId="10"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl><w:lvl w:ilvl="1"><w:numFmt w:val="lowerLetter"/></w:lvl></w:abstractNum>
	  <w:abstractNum w:abstractNumId="20"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>
	  <w:num w:numId="1"><w:abstractNumId w:val="10"/></w:num>
	  <w:num w:numId="2"><w:abstractNumId w:val="20"/></w:num></w:numbering>`

	d, err := AusWord(bauDocx(t, map[string]string{
		"word/document.xml":  dokumentXML(body),
		"word/numbering.xml": num,
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Absatz) != 4 {
		t.Fatalf("erwartet 4 Absaetze, bekam %d", len(d.Absatz))
	}
	will := []struct {
		art    Art
		stufe  int
		nummer int
	}{{ArtNummer, 0, 1}, {ArtNummer, 1, 1}, {ArtNummer, 0, 2}, {ArtAufzaehlung, 0, 0}}
	for i, w := range will {
		a := d.Absatz[i]
		if a.Art != w.art || a.Stufe != w.stufe || a.Nummer != w.nummer {
			t.Errorf("Absatz %d: Art %v Stufe %d Nummer %d, erwartet %v/%d/%d",
				i, a.Art, a.Stufe, a.Nummer, w.art, w.stufe, w.nummer)
		}
	}

	// Der Untereintrag gehoert unter seinen Punkt und nicht daneben.
	bl := NachBloecken(d)
	if len(bl) != 3 {
		t.Fatalf("erwartet 3 Bloecke auf oberster Ebene, bekam %d", len(bl))
	}
	kinder, _ := bl[0]["children"].([]map[string]any)
	if len(kinder) != 1 {
		t.Fatalf("Untereintrag haengt nicht am ersten Punkt: %v", bl[0])
	}
}

// A manual line break is a line break. Swallowing it ran two lines together,
// and an address block became one long line.
func TestWordUmbruchTrenntAbsatz(t *testing.T) {
	body := `<w:p>` + wLaufXML("Zeile eins") + `<w:r><w:br/></w:r>` + wLaufXML("Zeile zwei") + `</w:p>`
	d, err := AusWord(bauDocx(t, map[string]string{"word/document.xml": dokumentXML(body)}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Absatz) != 2 {
		t.Fatalf("erwartet 2 Absaetze, bekam %d", len(d.Absatz))
	}
	if nurText(d.Absatz[0].Text) != "Zeile eins" || nurText(d.Absatz[1].Text) != "Zeile zwei" {
		t.Errorf("falsch getrennt: %q / %q", nurText(d.Absatz[0].Text), nurText(d.Absatz[1].Text))
	}
}

// Ein eingebettetes Bild reist als Datenadresse mit. Ohne das fehlte im
// gelesenen Dokument genau die Stelle, um die es oft geht.
func TestWordBildKommtMit(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	body := `<w:p><w:r><w:drawing><wp:inline xmlns:wp="x"><wp:docPr name="Bild 1" descr="Der Aufbau"/>` +
		`<a:blip xmlns:a="y" r:embed="rId5" xmlns:r="z"/></wp:inline></w:drawing></w:r></w:p>`
	rels := `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
	  <Relationship Id="rId5" Target="media/bild1.png"/></Relationships>`

	d, err := AusWord(bauDocx(t, map[string]string{
		"word/document.xml":            dokumentXML(body),
		"word/_rels/document.xml.rels": rels,
	}, map[string][]byte{"word/media/bild1.png": png}))
	if err != nil {
		t.Fatal(err)
	}
	var bild *Absatz
	for i := range d.Absatz {
		if d.Absatz[i].Art == ArtDatei {
			bild = &d.Absatz[i]
		}
	}
	if bild == nil {
		t.Fatal("kein Bild im Dokument")
	}
	if !strings.HasPrefix(bild.Bild, "data:image/png;base64,") {
		t.Errorf("Bild fehlt oder ist kein PNG: %.40q", bild.Bild)
	}
	if nurText(bild.Text) != "Der Aufbau" {
		t.Errorf("Beschreibung fehlt: %q", nurText(bild.Text))
	}

	bl := NachBloecken(d)
	var gefunden bool
	for _, b := range bl {
		if b["type"] == "image" {
			gefunden = true
		}
	}
	if !gefunden {
		t.Error("kein Bildblock fuer den Editor")
	}
}

// Not every writer puts the list into the paragraph. python-docx and
// LibreOffice put it into the style, and reading the paragraph alone turned a
// numbered instruction into a row of ordinary lines. The document title has a
// style of its own as well and arrived as a plain paragraph.
func TestWordListeAusFormatvorlage(t *testing.T) {
	mitStil := func(stil, text string) string {
		return `<w:p><w:pPr><w:pStyle w:val="` + stil + `"/></w:pPr>` + wLaufXML(text) + `</w:p>`
	}
	body := mitStil("Title", "Handbuch") + mitStil("ListNumber", "Erstens") +
		mitStil("ListNumber2", "Darunter") + mitStil("ListBullet", "Ein Punkt")
	vorlagen := `<?xml version="1.0"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	  <w:style w:styleId="Title"><w:name w:val="Title"/></w:style>
	  <w:style w:styleId="ListNumber"><w:name w:val="List Number"/><w:pPr><w:numPr><w:numId w:val="5"/></w:numPr></w:pPr></w:style>
	  <w:style w:styleId="ListNumber2"><w:name w:val="List Number 2"/><w:pPr><w:numPr><w:numId w:val="6"/></w:numPr></w:pPr></w:style>
	  <w:style w:styleId="ListBullet"><w:name w:val="List Bullet"/><w:pPr><w:numPr><w:numId w:val="7"/></w:numPr></w:pPr></w:style></w:styles>`
	num := `<?xml version="1.0"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	  <w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl><w:lvl w:ilvl="1"><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum>
	  <w:abstractNum w:abstractNumId="2"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>
	  <w:num w:numId="5"><w:abstractNumId w:val="1"/></w:num>
	  <w:num w:numId="6"><w:abstractNumId w:val="1"/></w:num>
	  <w:num w:numId="7"><w:abstractNumId w:val="2"/></w:num></w:numbering>`

	d, err := AusWord(bauDocx(t, map[string]string{
		"word/document.xml":  dokumentXML(body),
		"word/styles.xml":    vorlagen,
		"word/numbering.xml": num,
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if d.Titel != "Handbuch" {
		t.Errorf("Titel ist %q, erwartet \"Handbuch\"", d.Titel)
	}
	if len(d.Absatz) != 3 {
		t.Fatalf("erwartet 3 Absaetze, bekam %d", len(d.Absatz))
	}
	will := []struct {
		art   Art
		stufe int
	}{{ArtNummer, 0}, {ArtNummer, 1}, {ArtAufzaehlung, 0}}
	for i, w := range will {
		if d.Absatz[i].Art != w.art || d.Absatz[i].Stufe != w.stufe {
			t.Errorf("Absatz %d: Art %v Stufe %d, erwartet %v/%d",
				i, d.Absatz[i].Art, d.Absatz[i].Stufe, w.art, w.stufe)
		}
	}
}
