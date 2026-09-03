// Images in the PDF.

// Until now the PDF only referred to an image by name and its address — in a
// wiki full of sketches and photos that is an index, not a depiction. The
// typesetting does not use external packages: an image is unpacked, resized
// to a reasonable edge length and embedded as a raw RGB stream.

// Why raw and not passing through the original JPEG: a PDF can use
// DCTDecode but only for color spaces it understands; a CMYK or grayscale
// JPEG would then be embedded with wrong colors. Decoding via the image
// packages in the standard library handles every format correctly; the
// downside is size, which is mitigated by limiting the edge length.
package dok

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// maxKante is the largest edge length in image pixels that will be embedded.
// A camera photo has about 6000 pixels; on an A4 page fewer than 1400 are
// visible, so anything above that would be weight without image content.
const maxKante = 1400

// pdfBild is a fully prepared image: RGB, pixel by pixel, packed.
type pdfBild struct {
	breite, hoehe int
	strom         []byte
}

// bildAufbereiten unpacks an image and converts it into the form the PDF
// needs. Anything that cannot be unpacked is treated as no image — the
// typesetting then falls back to the reference line.
func bildAufbereiten(daten []byte) (*pdfBild, bool) {
	roh, ok := bildBytes(daten)
	if !ok {
		return nil, false
	}
	bild, _, err := image.Decode(bytes.NewReader(roh))
	if err != nil {
		return nil, false
	}
	bild = verkleinern(bild, maxKante)

	feld := bild.Bounds()
	b, h := feld.Dx(), feld.Dy()
	if b <= 0 || h <= 0 {
		return nil, false
	}

	// There is no transparency here: that would be a separate mask in the
	// PDF. Instead the image is composited over white, like on paper. A
	// logo with a transparent background therefore looks printed rather
	// than like a black box.
	rgb := make([]byte, 0, b*h*3)
	for y := feld.Min.Y; y < feld.Max.Y; y++ {
		for x := feld.Min.X; x < feld.Max.X; x++ {
			r, g, bl, a := bild.At(x, y).RGBA()
			if a == 0 {
				rgb = append(rgb, 255, 255, 255)
				continue
			}
			// The values are premultiplied by alpha; composited onto white
			// that means: color plus the remainder that shows through.
			rest := 0xffff - a
			rgb = append(rgb,
				byte((r+rest)>>8),
				byte((g+rest)>>8),
				byte((bl+rest)>>8))
		}
	}
	return &pdfBild{breite: b, hoehe: h, strom: packen(rgb)}, true
}

// bildBytes returns the bytes behind what is stored in the document: either
// the file itself or a data URL as found in an imported Word document.
func bildBytes(daten []byte) ([]byte, bool) {
	if len(daten) == 0 {
		return nil, false
	}
	s := string(daten)
	if !strings.HasPrefix(s, "data:") {
		return daten, true
	}
	i := strings.Index(s, ",")
	if i < 0 || !strings.Contains(s[:i], "base64") {
		return nil, false
	}
	entpackt, err := base64.StdEncoding.DecodeString(s[i+1:])
	if err != nil {
		return nil, false
	}
	return entpackt, true
}

// verkleinern scales an image down to at most kante pixels on its longest
// side.

// Nearest-neighbor sampling, not a weighted average: sufficient for a sketch
// or a screenshot and avoids adding another dependency. Images already small
// enough are left unchanged.
func verkleinern(bild image.Image, kante int) image.Image {
	feld := bild.Bounds()
	b, h := feld.Dx(), feld.Dy()
	if b <= kante && h <= kante {
		return bild
	}
	neuB, neuH := b, h
	if b >= h {
		neuB = kante
		neuH = h * kante / b
	} else {
		neuH = kante
		neuB = b * kante / h
	}
	if neuB < 1 {
		neuB = 1
	}
	if neuH < 1 {
		neuH = 1
	}
	klein := image.NewRGBA(image.Rect(0, 0, neuB, neuH))
	for y := 0; y < neuH; y++ {
		for x := 0; x < neuB; x++ {
			klein.Set(x, y, bild.At(feld.Min.X+x*b/neuB, feld.Min.Y+y*h/neuH))
		}
	}
	return klein
}

// bildSetzen places an image in the layout.

// Size: 96 pixels per inch, as in the browser, so three quarters of a point
// per image pixel. If wider than the text column it is scaled to it; if
// taller than a page it is scaled to the page height. An image prefers to
// start on a new page rather than be split in two.
func (s *setzer) bildSetzen(bild *pdfBild, einzug, breite float64) {
	nummer := len(s.bilder)
	s.bilder = append(s.bilder, bild)

	b := float64(bild.breite) * 0.75
	h := float64(bild.hoehe) * 0.75
	if b > breite {
		h *= breite / b
		b = breite
	}
	if hoch := seiteHoehe - randOben - randUnten; h > hoch {
		b *= hoch / h
		h = hoch
	}

	s.y -= 6
	s.platzPruefen(h + 6)
	s.y -= h

	// q und Q klammern die Verschiebung ein, sonst gilt sie fuer alles, was
	// danach auf dieser Seite gesetzt wird.
	s.schreibeBild(nummer, einzug, s.y, b, h)
	s.y -= 8
}
