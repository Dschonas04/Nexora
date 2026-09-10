package dok

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// A paragraph with everything the editor can attach to a word.
const bunterAbsatz = `[{"type":"paragraph","content":[
	{"type":"text","text":"normal ","styles":{}},
	{"type":"text","text":"markiert","styles":{"backgroundColor":"yellow"}},
	{"type":"text","text":" und ","styles":{}},
	{"type":"text","text":"farbig","styles":{"textColor":"red"}},
	{"type":"text","text":" und ","styles":{}},
	{"type":"text","text":"beides","styles":{"textColor":"blue","backgroundColor":"green"}}
]}]`

// The names from the editor have to reach the document in the first place.
// This is exactly where it used to get lost: only the yes/no styles were read,
// and a highlight is not one of those.
func TestMarkierungKommtImDokumentAn(t *testing.T) {
	d := AusInhaltMitBildern(json.RawMessage(bunterAbsatz), "Bunt", nil)
	if len(d.Absatz) == 0 {
		t.Fatal("kein Absatz gelesen")
	}
	nach := map[string]Stueck{}
	for _, st := range d.Absatz[0].Text {
		nach[strings.TrimSpace(st.Text)] = st
	}
	if nach["markiert"].Hintergrund != "yellow" {
		t.Errorf("Markierung fehlt: %+v", nach["markiert"])
	}
	if nach["farbig"].Farbe != "red" {
		t.Errorf("Schriftfarbe fehlt: %+v", nach["farbig"])
	}
	if nach["beides"].Farbe != "blue" || nach["beides"].Hintergrund != "green" {
		t.Errorf("beides fehlt: %+v", nach["beides"])
	}
	// "default" is not a name but the absence of one.
	e := AusInhaltMitBildern(json.RawMessage(
		`[{"type":"paragraph","content":[{"type":"text","text":"x","styles":{"textColor":"default"}}]}]`), "", nil)
	if e.Absatz[0].Text[0].Farbe != "" {
		t.Error("default wurde als Farbe gelesen")
	}
}

// In the PDF the highlight is a filled box behind the text and the type colour
// is a colour operator. The check looks for the operators in the content
// stream -- uncompressed, so the test stays readable.
func TestMarkierungStehtImPDF(t *testing.T) {
	d := AusInhaltMitBildern(json.RawMessage(bunterAbsatz), "Bunt", nil)
	roh := inhaltsstrom(t, PDF(d))

	gelb, _ := hintergrundfarbe("yellow")
	if !strings.Contains(roh, gelb.pdfFarbe()) {
		t.Error("die gelbe Markierung fehlt im PDF")
	}
	rot, _ := schriftfarbe("red")
	if !strings.Contains(roh, rot.pdfFarbe()) {
		t.Error("die rote Schrift fehlt im PDF")
	}
	// And the box must be filled, otherwise it would be an outline.
	if !strings.Contains(roh, " re f ") {
		t.Error("kein gefuellter Kasten im Inhaltsstrom")
	}
	// After a coloured word black stands again, otherwise the rest of the page
	// would take the colour along.
	if !strings.Contains(roh, "0 0 0 rg") {
		t.Error("die Farbe wird nicht zurueckgesetzt")
	}
}

// inhaltsstrom unpacks the page contents again. In the PDF they lie compressed
// (zlib), and a test that searches the packed stream for operators never finds
// anything.
func inhaltsstrom(t *testing.T, pdf []byte) string {
	t.Helper()
	var raus strings.Builder
	rest := pdf
	for {
		i := bytes.Index(rest, []byte("stream\n"))
		if i < 0 {
			break
		}
		rest = rest[i+len("stream\n"):]
		j := bytes.Index(rest, []byte("\nendstream"))
		if j < 0 {
			break
		}
		roh := rest[:j]
		rest = rest[j:]
		leser, err := zlib.NewReader(bytes.NewReader(roh))
		if err != nil {
			continue // Bilder und anderes, was nicht zlib ist
		}
		aus, err := io.ReadAll(leser)
		leser.Close()
		if err == nil {
			raus.Write(aus)
			raus.WriteByte('\n')
		}
	}
	if raus.Len() == 0 {
		t.Fatal("kein lesbarer Inhaltsstrom im PDF")
	}
	return raus.String()
}

// Word highlights with a name from its fixed palette, not with a colour value.
// Put a free value there and Word shows nothing at all.
func TestMarkierungStehtImWord(t *testing.T) {
	d := AusInhaltMitBildern(json.RawMessage(bunterAbsatz), "Bunt", nil)
	roh, err := Word(d)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		t.Fatal(err)
	}
	var xml string
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(r)
			r.Close()
			xml = string(b)
		}
	}
	if xml == "" {
		t.Fatal("kein document.xml im Archiv")
	}
	if !strings.Contains(xml, `<w:highlight w:val="yellow"/>`) {
		t.Error("die Markierung fehlt im Word-Dokument")
	}
	rot, _ := schriftfarbe("red")
	if !strings.Contains(xml, `<w:color w:val="`+rot.hex()+`"/>`) {
		t.Error("die Schriftfarbe fehlt im Word-Dokument")
	}
}

// An unknown name does not colour. Rather ordinary type than a guessed
// colour.
func TestUnbekannteFarbeFaerbtNicht(t *testing.T) {
	if _, ok := schriftfarbe("mauve"); ok {
		t.Error("mauve sollte unbekannt sein")
	}
	if _, ok := hintergrundfarbe(""); ok {
		t.Error("der leere Name ist keine Farbe")
	}
	c, _ := schriftfarbe("blue")
	if c.hex() != "2382E3" {
		t.Logf("Hex von blue: %s", c.hex())
	}
}
