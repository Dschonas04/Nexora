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

// Ein Absatz mit allem, was der Editor an einem Wort anbringen kann.
const bunterAbsatz = `[{"type":"paragraph","content":[
	{"type":"text","text":"normal ","styles":{}},
	{"type":"text","text":"markiert","styles":{"backgroundColor":"yellow"}},
	{"type":"text","text":" und ","styles":{}},
	{"type":"text","text":"farbig","styles":{"textColor":"red"}},
	{"type":"text","text":" und ","styles":{}},
	{"type":"text","text":"beides","styles":{"textColor":"blue","backgroundColor":"green"}}
]}]`

// Die Namen aus dem Editor muessen ueberhaupt erst im Dokument ankommen. Genau
// hier ging es bisher verloren: gelesen wurden nur die Ja/Nein-Stile, und eine
// Markierung ist keiner.
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
	// "default" ist kein Name, sondern die Abwesenheit eines Namens.
	e := AusInhaltMitBildern(json.RawMessage(
		`[{"type":"paragraph","content":[{"type":"text","text":"x","styles":{"textColor":"default"}}]}]`), "", nil)
	if e.Absatz[0].Text[0].Farbe != "" {
		t.Error("default wurde als Farbe gelesen")
	}
}

// Im PDF steht die Markierung als gefuellter Kasten vor dem Text und die
// Schriftfarbe als Farboperator. Geprueft wird auf die Operatoren im
// Inhaltsstrom -- unkomprimiert, damit der Test lesbar bleibt.
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
	// Und der Kasten muss gefuellt sein, sonst waere es ein Umriss.
	if !strings.Contains(roh, " re f ") {
		t.Error("kein gefuellter Kasten im Inhaltsstrom")
	}
	// Nach einem gefaerbten Wort steht wieder Schwarz, sonst faerbte sich der
	// Rest der Seite mit.
	if !strings.Contains(roh, "0 0 0 rg") {
		t.Error("die Farbe wird nicht zurueckgesetzt")
	}
}

// inhaltsstrom packt die Seiteninhalte wieder aus. Sie liegen im PDF
// zusammengedrueckt (zlib), und ein Test, der im gepackten Strom nach
// Operatoren sucht, findet nie etwas.
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

// Word markiert mit einem Namen aus seiner festen Palette, nicht mit einem
// Farbwert. Steht dort ein freier Wert, zeigt Word gar nichts an.
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

// Ein unbekannter Name faerbt nicht. Lieber gewoehnlicher Satz als eine
// geratene Farbe.
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
