package dok

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// probePNG liefert ein kleines Bild als PNG-Bytes.
func probePNG(t *testing.T, breite, hoehe int) []byte {
	t.Helper()
	b := image.NewRGBA(image.Rect(0, 0, breite, hoehe))
	for y := 0; y < hoehe; y++ {
		for x := 0; x < breite; x++ {
			b.Set(x, y, color.RGBA{R: 200, G: 60, B: 60, A: 255})
		}
	}
	var puffer bytes.Buffer
	if err := png.Encode(&puffer, b); err != nil {
		t.Fatal(err)
	}
	return puffer.Bytes()
}

func TestBildAufbereitenLiestEinPNG(t *testing.T) {
	bild, ok := bildAufbereiten(probePNG(t, 8, 4))
	if !ok {
		t.Fatal("nicht gelesen")
	}
	if bild.breite != 8 || bild.hoehe != 4 {
		t.Fatalf("Maße: %dx%d", bild.breite, bild.hoehe)
	}
	if len(bild.strom) == 0 {
		t.Fatal("leerer Strom")
	}
}

// Eine Datenadresse, wie sie aus einem eingelesenen Word-Dokument stammt, muss
// denselben Weg gehen wie eine Datei.
func TestBildAufbereitenLiestEineDatenadresse(t *testing.T) {
	adresse := "data:image/png;base64," + base64.StdEncoding.EncodeToString(probePNG(t, 4, 4))
	if _, ok := bildAufbereiten([]byte(adresse)); !ok {
		t.Fatal("Datenadresse nicht gelesen")
	}
}

// Was kein Bild ist, ist kein Bild: der Satz faellt dann auf die Verweiszeile
// zurueck, statt eine leere Flaeche zu setzen.
func TestBildAufbereitenLehntUnsinnAb(t *testing.T) {
	if _, ok := bildAufbereiten([]byte("kein Bild")); ok {
		t.Fatal("Unsinn als Bild angenommen")
	}
}

// Ein grosses Bild wird auf eine vernuenftige Kantenlaenge gebracht, sonst waegt
// ein Foto aus der Kamera das ganze Dokument nieder.
func TestVerkleinernHaeltDieKante(t *testing.T) {
	gross := image.NewRGBA(image.Rect(0, 0, 4000, 2000))
	klein := verkleinern(gross, 1400)
	if klein.Bounds().Dx() != 1400 || klein.Bounds().Dy() != 700 {
		t.Fatalf("Maße: %v", klein.Bounds())
	}
	// Ein Bild, das schon klein genug ist, wird nicht angefasst.
	winzig := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if verkleinern(winzig, 1400) != image.Image(winzig) {
		t.Fatal("kleines Bild wurde neu gerechnet")
	}
}

// dokumentMitBild baut ein Dokument, in dem ein Bildblock steht.
func dokumentMitBild(t *testing.T) Dokument {
	t.Helper()
	daten := probePNG(t, 20, 10)
	roh := json.RawMessage(`[{"type":"paragraph","content":[{"type":"text","text":"Davor","styles":{}}]},
		{"type":"image","props":{"url":"/api/pages/a/attachments/b","name":"Plan","caption":"Der Aufbau"}}]`)
	return AusInhaltMitBildern(roh, "Mit Bild", func(adresse string) ([]byte, bool) {
		if adresse == "/api/pages/a/attachments/b" {
			return daten, true
		}
		return nil, false
	})
}

// Das PDF traegt das Bild wirklich bei sich, als eingebettetes Objekt und nicht
// als Zeile mit einem Namen darin.
func TestPDFBettetDasBildEin(t *testing.T) {
	roh := PDF(dokumentMitBild(t))
	if !bytes.Contains(roh, []byte("/Subtype /Image")) {
		t.Fatal("kein Bildobjekt im PDF")
	}
	// Der Seitenstrom ist gepackt, der Aufruf steht also nicht im Klartext in
	// der Datei; er wird ausgepackt gesucht.
	if !bytes.Contains([]byte(seitenstroeme(t, roh)), []byte("/Im0 Do")) {
		t.Fatal("das Bild wird nirgends aufgerufen")
	}
	if !bytes.Contains(roh, []byte("/XObject")) {
		t.Fatal("das Bild steht nicht in den Betriebsmitteln")
	}
}

// seitenstroeme packt die Seiteninhalte eines PDF aus, damit ein Test hinein
// sehen kann.
func seitenstroeme(t *testing.T, roh []byte) string {
	t.Helper()
	var b strings.Builder
	rest := roh
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
		if z, err := zlib.NewReader(bytes.NewReader(rest[:j])); err == nil {
			var aus bytes.Buffer
			aus.ReadFrom(z)
			z.Close()
			b.Write(aus.Bytes())
		}
		rest = rest[j:]
	}
	return b.String()
}

// Ohne Bilddaten bleibt es bei der Verweiszeile. Eine Seite, deren Bild nicht
// mehr da ist, soll sagen, was fehlt, statt eine Luecke zu lassen.
func TestPDFOhneBildBleibtBeiDerZeile(t *testing.T) {
	roh := PDF(AusInhaltMitBildern(json.RawMessage(
		`[{"type":"image","props":{"url":"/api/x","name":"Plan"}}]`), "Ohne", nil))
	if bytes.Contains(roh, []byte("/Subtype /Image")) {
		t.Fatal("Bildobjekt ohne Bilddaten")
	}
}

// Word legt das Bild als Datei ins Archiv, nennt seinen Typ und verweist darauf.
// Fehlt eines der drei, meldet Word die Datei als beschaedigt.
func TestWordBettetDasBildEin(t *testing.T) {
	roh, err := Word(dokumentMitBild(t))
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		t.Fatal(err)
	}
	teile := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		b.ReadFrom(r)
		r.Close()
		teile[f.Name] = b.String()
	}

	if _, ok := teile["word/media/bild0.png"]; !ok {
		t.Fatalf("keine Bilddatei im Archiv: %v", teile)
	}
	if !strings.Contains(teile["[Content_Types].xml"], `Extension="png"`) {
		t.Fatal("der Typ der Bilddatei fehlt")
	}
	if !strings.Contains(teile["word/_rels/document.xml.rels"], "media/bild0.png") {
		t.Fatal("die Beziehung zum Bild fehlt")
	}
	if !strings.Contains(teile["word/document.xml"], `r:embed="rIdBild0"`) {
		t.Fatal("der Text ruft das Bild nicht auf")
	}
	// Die Unterschrift steht darunter, der Name nicht: der steht schon im Bild.
	if !strings.Contains(teile["word/document.xml"], "Der Aufbau") {
		t.Fatal("die Bildunterschrift fehlt")
	}
}

// Der eigene Word-Leser findet das Bild in der eigenen Word-Datei wieder.
//
// Das ist die schaerfste Probe, die ohne Word zu haben ist: die Datei muss so
// gebaut sein, dass ein Leser, der die Beziehungen und die Zeichnung wirklich
// verfolgt, das Bild findet -- und nicht nur so, dass die Teile da sind.
func TestWordRundlaufFindetDasBild(t *testing.T) {
	roh, err := Word(dokumentMitBild(t))
	if err != nil {
		t.Fatal(err)
	}
	zurueck, err := AusWord(roh)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range zurueck.Absatz {
		if strings.HasPrefix(a.Bild, "data:image/png;base64,") {
			return
		}
	}
	t.Fatal("kein Bild im zurückgelesenen Dokument")
}
