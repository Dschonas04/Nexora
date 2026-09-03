// Images in the Word document.
//
// A docx stores its images in the archive: as a file under word/media, as a
// relationship in the rels and as a drawing in the document text. If any of
// the three are missing Word reports the file as corrupted — therefore they
// are handled together here.
//
// Unlike the PDF path the bytes are inserted unchanged. Word understands PNG,
// JPEG and GIF natively and resampling here would only reduce sharpness.
package dok

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// wordBildTeil describes an image as it appears in the archive.
type wordBildTeil struct {
	name  string // filename under word/media
	kenn  string // relationship id, rIdN
	daten []byte
	// Dimensions in EMU, the unit Word uses: 914400 per inch.
	breiteEMU, hoeheEMU int64
}

// The text column of an A4 page with a 2 cm margin, in EMU. A wider image is
// scaled to this width, otherwise it would extend off the page.
const satzBreiteEMU = 5731200

// bildTeilAnlegen prepares an image for the archive. Without a readable
// dimension there is no image: Word requires the dimensions in the document
// and they cannot be guessed.
func bildTeilAnlegen(daten []byte, nummer int) (wordBildTeil, bool) {
	roh, ok := bildBytes(daten)
	if !ok {
		return wordBildTeil{}, false
	}
	konfig, art, err := image.DecodeConfig(bytes.NewReader(roh))
	if err != nil || konfig.Width <= 0 || konfig.Height <= 0 {
		return wordBildTeil{}, false
	}
	endung := art
	if endung == "jpeg" {
		endung = "jpg"
	}

	// 96 pixels per inch as in the browser: 9525 EMU per pixel.
	breite := int64(konfig.Width) * 9525
	hoehe := int64(konfig.Height) * 9525
	if breite > satzBreiteEMU {
		hoehe = hoehe * satzBreiteEMU / breite
		breite = satzBreiteEMU
	}

	return wordBildTeil{
		name:      fmt.Sprintf("bild%d.%s", nummer, endung),
		kenn:      fmt.Sprintf("rIdBild%d", nummer),
		daten:     roh,
		breiteEMU: breite,
		hoeheEMU:  hoehe,
	}, true
}

// bildXML setzt die Zeichnung in den Text. Der Aufbau ist die kleinste Form,
// die Word annimmt: ein Absatz, darin ein Lauf, darin eine eingebettete
// Zeichnung mit Ausdehnung und Verweis auf die Beziehung.
func bildXML(t wordBildTeil, einzug int) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:ind w:left="%d"/><w:spacing w:after="120"/></w:pPr>`+
		`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"`+
		` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">`+
		`<wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="%s"/>`+
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">`+
		`<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:nvPicPr><pic:cNvPr id="%d" name="%s"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:embed="%s"/>`+
		`<a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`,
		einzug, t.breiteEMU, t.hoeheEMU, nummerAus(t.kenn), wxml(t.name),
		nummerAus(t.kenn), wxml(t.name), t.kenn, t.breiteEMU, t.hoeheEMU)
}

// nummerAus zieht die laufende Nummer aus der Beziehungskennung. Word verlangt
// fuer jede Zeichnung eine eigene Zahl; sie muss nur eindeutig sein.
func nummerAus(kenn string) int {
	n := 0
	for _, c := range kenn {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n + 1
}

// bildTypen sind die Eintraege, die [Content_Types].xml je Endung braucht.
func bildTypen(teile []wordBildTeil) string {
	gesehen := map[string]bool{}
	var b strings.Builder
	for _, t := range teile {
		endung := t.name[strings.LastIndex(t.name, ".")+1:]
		if gesehen[endung] {
			continue
		}
		gesehen[endung] = true
		typ := "image/" + endung
		if endung == "jpg" {
			typ = "image/jpeg"
		}
		fmt.Fprintf(&b, `<Default Extension="%s" ContentType="%s"/>`, endung, typ)
	}
	return b.String()
}

// bildBeziehungen sind die Zeilen fuer word/_rels/document.xml.rels.
func bildBeziehungen(teile []wordBildTeil) string {
	var b strings.Builder
	for _, t := range teile {
		fmt.Fprintf(&b,
			`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`,
			t.kenn, t.name)
	}
	return b.String()
}
