// Bilder im PDF.
//
// Bis hierher nannte das PDF ein Bild nur beim Namen und verwies auf seine
// Adresse -- in einem Wiki voller Skizzen und Fotos ist das eine
// Inhaltsangabe, kein Abbild. Der Satz kommt ohne fremde Pakete aus, und das
// bleibt so: ein Bild wird entpackt, auf eine vernuenftige Kantenlaenge
// gebracht und als roher RGB-Strom eingebettet.
//
// Warum roh und nicht das JPEG durchgereicht: ein PDF kann DCTDecode, aber nur
// fuer die Farbraeume, die es kennt, und ein CMYK- oder Graustufen-JPEG waere
// dann falschfarbig statt eingebettet. Der Umweg ueber die Bildpakete der
// Standardbibliothek trifft jedes Format gleich richtig; bezahlt wird er mit
// Groesse, und dagegen steht die Begrenzung der Kantenlaenge.
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

// maxKante ist die groesste Kantenlaenge in Bildpunkten, die eingebettet wird.
// Ein Foto aus einer Kamera hat gut 6000 Punkte; auf einer A4-Seite sind davon
// keine 1400 zu sehen, alles darueber waere Gewicht ohne Bild.
const maxKante = 1400

// pdfBild ist ein fertig aufbereitetes Bild: RGB, Punkt fuer Punkt, gepackt.
type pdfBild struct {
	breite, hoehe int
	strom         []byte
}

// bildAufbereiten entpackt ein Bild und bringt es in die Form, die das PDF
// braucht. Was sich nicht entpacken laesst, gilt als kein Bild -- der Satz
// faellt dann auf die Verweiszeile zurueck.
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

	// Durchsichtigkeit gibt es hier nicht: sie waere eine eigene Maske im PDF.
	// Stattdessen liegt das Bild auf Weiss, wie auf Papier. Ein Logo mit
	// durchsichtigem Grund sieht damit aus wie gedruckt und nicht wie ein
	// schwarzer Kasten.
	rgb := make([]byte, 0, b*h*3)
	for y := feld.Min.Y; y < feld.Max.Y; y++ {
		for x := feld.Min.X; x < feld.Max.X; x++ {
			r, g, bl, a := bild.At(x, y).RGBA()
			if a == 0 {
				rgb = append(rgb, 255, 255, 255)
				continue
			}
			// Die Werte kommen mit dem Alpha vormultipliziert; auf Weiss gelegt
			// heisst das: Farbe plus der Rest, der durchscheint.
			rest := 0xffff - a
			rgb = append(rgb,
				byte((r+rest)>>8),
				byte((g+rest)>>8),
				byte((bl+rest)>>8))
		}
	}
	return &pdfBild{breite: b, hoehe: h, strom: packen(rgb)}, true
}

// bildBytes liefert die Bytes hinter dem, was im Dokument steht: entweder die
// Datei selbst, oder eine Datenadresse, wie sie aus einem eingelesenen
// Word-Dokument stammt.
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

// verkleinern rechnet ein Bild auf hoechstens kante Punkte je Seite herunter.
//
// Naechster Nachbar und kein gewichteter Mittelwert: fuer eine Skizze oder einen
// Bildschirmausschnitt reicht das, und die Alternative waere ein Paket mehr.
// Bilder, die schon klein genug sind, werden nicht angefasst.
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

// bildSetzen stellt ein Bild in den Satz.
//
// Die Groesse: 96 Punkte je Zoll, wie im Browser, also drei Viertel Punkt je
// Bildpunkt. Was breiter ist als der Satzspiegel, wird auf ihn gebracht; was
// hoeher ist als eine Seite, auf die Seitenhoehe. Ein Bild faengt lieber auf
// einer neuen Seite an, als in zwei Haelften zerschnitten zu werden.
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
