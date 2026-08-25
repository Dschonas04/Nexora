package dok

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

const beispiel = `[
 {"type":"heading","props":{"level":1},"content":[{"type":"text","text":"Übersicht"}]},
 {"type":"paragraph","content":[
   {"type":"text","text":"Ein Absatz mit "},
   {"type":"text","text":"fett","styles":{"bold":true}},
   {"type":"text","text":" und "},
   {"type":"text","text":"kursiv","styles":{"italic":true}},
   {"type":"text","text":" sowie einem sehr langen Wort: Donaudampfschifffahrtsgesellschaftskapitaensmuetze."}]},
 {"type":"bulletListItem","content":[{"type":"text","text":"Erster Punkt"}],
  "children":[{"type":"bulletListItem","content":[{"type":"text","text":"Untereintrag"}]}]},
 {"type":"numberedListItem","content":[{"type":"text","text":"Eins"}]},
 {"type":"numberedListItem","content":[{"type":"text","text":"Zwei"}]},
 {"type":"checkListItem","props":{"checked":true},"content":[{"type":"text","text":"Erledigt"}]},
 {"type":"quote","content":[{"type":"text","text":"Ein Zitat über zwei Zeilen, das lang genug ist, um wirklich umzubrechen und nicht nur so zu tun."}]},
 {"type":"codeBlock","props":{"language":"go"},"content":[{"type":"text","text":"func main() {\n\tprintln(\"hallo\")\n}"}]},
 {"type":"table","content":{"rows":[
   {"cells":[[{"type":"text","text":"Dienst"}],[{"type":"text","text":"Adresse"}]]},
   {"cells":[[{"type":"text","text":"Nexora"}],[{"type":"text","text":"10.0.2.43:3000"}]]}]}},
 {"type":"image","props":{"url":"/api/x.png","name":"Plan"}}]`

func lade(t *testing.T) Dokument {
	t.Helper()
	return AusInhalt(json.RawMessage(beispiel), "Testseite")
}

func TestEinlesenErkenntAlleArten(t *testing.T) {
	d := lade(t)
	arten := map[Art]int{}
	for _, a := range d.Absatz {
		arten[a.Art]++
	}
	for art, name := range map[Art]string{
		ArtUeberschrift: "Überschrift", ArtAbsatz: "Absatz", ArtAufzaehlung: "Aufzählung",
		ArtNummer: "Nummer", ArtAufgabe: "Aufgabe", ArtZitat: "Zitat",
		ArtCode: "Code", ArtTabelle: "Tabelle", ArtDatei: "Datei",
	} {
		if arten[art] == 0 {
			t.Errorf("keine %s eingelesen", name)
		}
	}
}

func TestNummerierungUndTiefe(t *testing.T) {
	d := lade(t)
	var nummern []int
	tiefeGefunden := false
	for _, a := range d.Absatz {
		if a.Art == ArtNummer {
			nummern = append(nummern, a.Nummer)
		}
		if a.Art == ArtAufzaehlung && a.Stufe == 1 {
			tiefeGefunden = true
		}
	}
	if len(nummern) != 2 || nummern[0] != 1 || nummern[1] != 2 {
		t.Errorf("Nummerierung %v, erwartet [1 2]", nummern)
	}
	if !tiefeGefunden {
		t.Error("Untereintrag hat keine Tiefe 1")
	}
}

// Umlaute müssen als WinAnsi-Bytes ankommen, nicht als Fragezeichen: das ist
// der einzige Grund, warum die Schriften überhaupt mit dieser Kodierung
// angemeldet werden.
func TestWinAnsiUmlaute(t *testing.T) {
	got := nachWinAnsi("Prüfspur – Größe €")
	if bytes.Contains(got, []byte("?")) {
		t.Fatalf("Zeichen verloren: % x", got)
	}
	if got[2] != 0xFC { // ü
		t.Errorf("ü kam als %#x an", got[2])
	}
	if !bytes.Contains(got, []byte{0x80}) {
		t.Error("Eurozeichen fehlt")
	}
}

func TestUmbruchBleibtInDerBreite(t *testing.T) {
	lang := strings.Repeat("Wort ", 200)
	zeilen := umbrechen([]wort{{lang, fNormal, 10.5, false, false, false}}, satzBreite)
	if len(zeilen) < 5 {
		t.Fatalf("nur %d Zeilen, da wurde nicht umbrochen", len(zeilen))
	}
	for i, z := range zeilen {
		var b float64
		for _, w := range z {
			b += textBreite(w.text, w.schrift, w.groesse)
		}
		// Ein Leerzeichen am Zeilenende darf überstehen, mehr nicht.
		if b > satzBreite+6 {
			t.Errorf("Zeile %d ist %.1f breit, erlaubt sind %.1f", i, b, satzBreite)
		}
	}
}

// Ein einzelnes Wort, das breiter ist als die Zeile, muss hart getrennt
// werden, sonst läuft es über den Rand und ist zur Hälfte weg.
func TestUeberlangesWortWirdGetrennt(t *testing.T) {
	zeilen := umbrechen([]wort{{strings.Repeat("A", 400), fNormal, 10.5, false, false, false}}, satzBreite)
	if len(zeilen) < 2 {
		t.Fatal("überlanges Wort wurde nicht getrennt")
	}
	for _, z := range zeilen {
		var b float64
		for _, w := range z {
			b += textBreite(w.text, w.schrift, w.groesse)
		}
		if b > satzBreite+1 {
			t.Errorf("Zeile ist %.1f breit, erlaubt sind %.1f", b, satzBreite)
		}
	}
}

func TestPDFIstEinPDF(t *testing.T) {
	roh := PDF(lade(t))
	if !bytes.HasPrefix(roh, []byte("%PDF-1.4")) {
		t.Fatal("kein PDF-Kopf")
	}
	if !bytes.Contains(roh, []byte("%%EOF")) {
		t.Fatal("kein Dateiende")
	}
	if !bytes.Contains(roh, []byte("/WinAnsiEncoding")) {
		t.Error("Schriften ohne WinAnsi, Umlaute kämen nicht an")
	}
	// Die Querverweistabelle muss auf echte Objektanfänge zeigen, sonst
	// öffnet kein Betrachter die Datei.
	i := bytes.LastIndex(roh, []byte("startxref"))
	if i < 0 {
		t.Fatal("startxref fehlt")
	}
	if len(roh) < 800 {
		t.Errorf("verdächtig klein: %d Bytes", len(roh))
	}
	if os.Getenv("PDF_SCHREIBEN") != "" {
		os.WriteFile(os.Getenv("PDF_SCHREIBEN"), roh, 0o644)
	}
}

func TestWordIstEinZipMitDokument(t *testing.T) {
	roh, err := Word(lade(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(roh, []byte("PK")) {
		t.Fatal("kein ZIP")
	}
	z, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		t.Fatal(err)
	}
	noetig := map[string]bool{
		"[Content_Types].xml": false, "_rels/.rels": false,
		"word/document.xml": false, "word/styles.xml": false,
		"word/_rels/document.xml.rels": false,
	}
	var dokument string
	for _, f := range z.File {
		if _, ok := noetig[f.Name]; ok {
			noetig[f.Name] = true
		}
		if f.Name == "word/document.xml" {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			dokument = string(b)
		}
	}
	for name, da := range noetig {
		if !da {
			t.Errorf("%s fehlt im Paket", name)
		}
	}
	// Jeder Teil muss für sich wohlgeformtes XML sein, sonst meldet Word die
	// Datei als beschädigt und öffnet sie gar nicht.
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, ".xml") && !strings.HasSuffix(f.Name, ".rels") {
			continue
		}
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		d := xml.NewDecoder(bytes.NewReader(b))
		for {
			_, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s ist kein gültiges XML: %v", f.Name, err)
			}
		}
	}
	for _, will := range []string{"Übersicht", "Erster Punkt", "func main()", "10.0.2.43:3000", "<w:tbl>"} {
		if !strings.Contains(dokument, will) {
			t.Errorf("%q fehlt im Dokument", will)
		}
	}
	// Leerzeichen am Rand eines Laufs dürfen nicht wegfallen, sonst klebt
	// "mit fett und" zusammen.
	if !strings.Contains(dokument, `xml:space="preserve"`) {
		t.Error("xml:space fehlt, Word wirft Randleerzeichen weg")
	}
}

// Ein spitzes Klammerzeichen im Text darf die Datei nicht zerlegen.
func TestSonderzeichenBrechenDasDokumentNicht(t *testing.T) {
	d := AusInhalt(json.RawMessage(
		`[{"type":"paragraph","content":[{"type":"text","text":"a < b & c > d \"e\""}]}]`), "T & <U>")
	roh, err := Word(d)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name != "word/document.xml" {
			continue
		}
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		dec := xml.NewDecoder(bytes.NewReader(b))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Fatalf("kaputtes XML: %v", err)
			}
		}
	}
	t.Fatal("document.xml nicht gefunden")
}
