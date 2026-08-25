// PDF-Ausgabe, von Hand geschrieben.
//
// Kein Fremdpaket: ein PDF mit den vierzehn Grundschriften ist ein
// überschaubares Format, und die Alternative wäre eine Abhängigkeit, die für
// einen Textexport mehr mitbringt, als hier je gebraucht wird. Was fehlt, fehlt
// bewusst, eingebettete Schriften und eingebettete Bilder. Beides würde die
// Datei um ein Vielfaches aufblähen; Bilder erscheinen stattdessen als
// benannter Verweis.
//
// Kodiert wird nach WinAnsi. Umlaute kommen damit an; was darin nicht vorkommt
// etwa Zeichen aus anderen Schriftsystemen, wird zu einem Fragezeichen.
// Das ist die Grenze der Grundschriften, nicht ein Versehen.
package dok

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
)

// Maße in Punkt (1/72 Zoll). A4 hoch.
const (
	seiteBreite = 595.28
	seiteHoehe  = 841.89
	randLinks   = 56.0
	randRechts  = 56.0
	randOben    = 64.0
	randUnten   = 56.0
	satzBreite  = seiteBreite - randLinks - randRechts
)

// Font identifiers inside the PDF. The names show up in the content stream.
const (
	fNormal     = "F1"
	fFett       = "F2"
	fKursiv     = "F3"
	fFettKursiv = "F4"
	fFest       = "F5"
	fFestFett   = "F6"
)

func breitenTabelle(f string) *[256]int16 {
	switch f {
	case fFett:
		return &breitenFett
	case fKursiv:
		return &breitenKursiv
	case fFettKursiv:
		return &breitenFettKursiv
	case fFest:
		return &breitenFest
	case fFestFett:
		return &breitenFestFett
	default:
		return &breitenNormal
	}
}

// textBreite measures a string in points.
func textBreite(s string, schrift string, groesse float64) float64 {
	tab := breitenTabelle(schrift)
	summe := 0
	for _, b := range nachWinAnsi(s) {
		w := tab[b]
		if w == 0 {
			w = 500 // unknown character: average width, so nothing overflows
		}
		summe += int(w)
	}
	return float64(summe) * groesse / 1000
}

// nachWinAnsi wandelt UTF-8 in die Kodierung, mit der die Schriften im PDF
// angemeldet sind.
func nachWinAnsi(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 128:
			out = append(out, byte(r))
		case r <= 255 && r >= 160:
			out = append(out, byte(r))
		default:
			if b, ok := winAnsiSonder[r]; ok {
				out = append(out, b)
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

// Characters between 128 and 159, where WinAnsi differs from Unicode.
var winAnsiSonder = map[rune]byte{
	'€': 128, '‚': 130, 'ƒ': 131, '„': 132, '…': 133,
	'†': 134, '‡': 135, 'ˆ': 136, '‰': 137, 'Š': 138,
	'‹': 139, 'Œ': 140, 'Ž': 142, '‘': 145, '’': 146,
	'“': 147, '”': 148, '•': 149, '–': 150, '—': 151,
	'˜': 152, '™': 153, 'š': 154, '›': 155, 'œ': 156,
	'ž': 158, 'Ÿ': 159,
	// Weiche Zeilenumbrüche und geschützte Leerzeichen sollen Leerzeichen sein
	// und keine Fragezeichen.
	' ': 32, '​': 32,
}

// pdfZeichenkette escapes what carries meaning inside a PDF string.
func pdfZeichenkette(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, c := range nachWinAnsi(s) {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			b.WriteString("\\r")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(')')
	return b.String()
}

// wort is a piece of text with its font and colour, ready to be set.
type wort struct {
	text    string
	schrift string
	groesse float64
	verweis bool // set in colour and underlined
	durch   bool
	unter   bool
}

// setzer builds the content stream page by page.
type setzer struct {
	seiten []*bytes.Buffer
	akt    *bytes.Buffer
	y      float64 // current baseline, measured from the top
	titel  string
	fuss   string
}

func neuerSetzer(titel, fuss string) *setzer {
	s := &setzer{titel: titel, fuss: fuss}
	s.neueSeite()
	return s
}

func (s *setzer) neueSeite() {
	s.akt = &bytes.Buffer{}
	s.seiten = append(s.seiten, s.akt)
	s.y = seiteHoehe - randOben
}

// platzPruefen makes sure hoehe points still fit on the page.
func (s *setzer) platzPruefen(hoehe float64) {
	if s.y-hoehe < randUnten {
		s.neueSeite()
	}
}

// zeileSetzen writes one line that has already been wrapped.
func (s *setzer) zeileSetzen(woerter []wort, x, zeilenHoehe float64) {
	s.platzPruefen(zeilenHoehe)
	s.y -= zeilenHoehe
	lauf := x
	for _, w := range woerter {
		if w.text == "" {
			continue
		}
		breite := textBreite(w.text, w.schrift, w.groesse)
		if w.verweis {
			// 0.10 0.35 0.65: the same muted blue the interface uses.
			fmt.Fprintf(s.akt, "0.10 0.35 0.65 rg\n")
		}
		fmt.Fprintf(s.akt, "BT /%s %.1f Tf %.2f %.2f Td %s Tj ET\n",
			w.schrift, w.groesse, lauf, s.y, pdfZeichenkette(w.text))
		if w.verweis || w.unter {
			fmt.Fprintf(s.akt, "%.2f %.2f %.2f %.2f re f\n",
				lauf, s.y-1.6, breite, 0.6)
		}
		if w.durch {
			fmt.Fprintf(s.akt, "%.2f %.2f %.2f %.2f re f\n",
				lauf, s.y+w.groesse*0.28, breite, 0.6)
		}
		if w.verweis {
			fmt.Fprintf(s.akt, "0 0 0 rg\n")
		}
		lauf += breite
	}
}

// umbrechen verteilt Textstücke auf Zeilen, die in breite passen.
//
// Umbrochen wird an Leerzeichen. Ein einzelnes Wort, das länger ist als die
// Zeile, eine lange Adresse etwa, wird hart getrennt, sonst liefe es über
// den Rand hinaus und wäre halb weg.
func umbrechen(stuecke []wort, breite float64) [][]wort {
	var zeilen [][]wort
	var zeile []wort
	var lauf float64

	neueZeile := func() {
		zeilen = append(zeilen, zeile)
		zeile = nil
		lauf = 0
	}

	for _, st := range stuecke {
		// Der Trenner bleibt am vorigen Wort hängen, damit Leerzeichen nicht
		// verloren gehen.
		teile := zerlegeMitLeerzeichen(st.text)
		for _, t := range teile {
			w := textBreite(t, st.schrift, st.groesse)
			if lauf+w > breite && len(zeile) > 0 && strings.TrimSpace(t) != "" {
				neueZeile()
			}
			// Still too long: break it hard.
			for textBreite(t, st.schrift, st.groesse) > breite {
				schnitt := len(t)
				for schnitt > 1 && textBreite(t[:schnitt], st.schrift, st.groesse) > breite {
					schnitt--
				}
				zeile = append(zeile, wort{t[:schnitt], st.schrift, st.groesse, st.verweis, st.durch, st.unter})
				neueZeile()
				t = t[schnitt:]
			}
			if t == "" {
				continue
			}
			if lauf == 0 && strings.TrimSpace(t) == "" {
				// No line may start with a space.
				continue
			}
			zeile = append(zeile, wort{t, st.schrift, st.groesse, st.verweis, st.durch, st.unter})
			lauf += textBreite(t, st.schrift, st.groesse)
		}
	}
	if len(zeile) > 0 {
		zeilen = append(zeilen, zeile)
	}
	if len(zeilen) == 0 {
		zeilen = append(zeilen, nil)
	}
	return zeilen
}

// zerlegeMitLeerzeichen schneidet an Wortgrenzen und behält die Leerzeichen am
// Ende des jeweiligen Wortes.
func zerlegeMitLeerzeichen(s string) []string {
	var out []string
	akt := strings.Builder{}
	imLeerraum := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			imLeerraum = true
			akt.WriteRune(' ')
			continue
		}
		if imLeerraum {
			out = append(out, akt.String())
			akt.Reset()
			imLeerraum = false
		}
		akt.WriteRune(r)
	}
	if akt.Len() > 0 {
		out = append(out, akt.String())
	}
	return out
}

// schriftFuer picks the face for a run of text.
func schriftFuer(s Stueck) string {
	switch {
	case s.Fest && s.Fett:
		return fFestFett
	case s.Fest:
		return fFest
	case s.Fett && s.Kursiv:
		return fFettKursiv
	case s.Fett:
		return fFett
	case s.Kursiv:
		return fKursiv
	}
	return fNormal
}

func alsWoerter(st []Stueck, groesse float64, immerFett bool) []wort {
	out := make([]wort, 0, len(st))
	for _, s := range st {
		if immerFett {
			s.Fett = true
		}
		out = append(out, wort{
			text:    s.Text,
			schrift: schriftFuer(s),
			groesse: groesse,
			verweis: s.Verweis != "",
			durch:   s.Durch,
			unter:   s.Unter,
		})
	}
	return out
}

// Font sizes of the heading levels.
var ueberschriftGroesse = map[int]float64{1: 20, 2: 16, 3: 14, 4: 12.5, 5: 11.5, 6: 11}

// PDF typesets a document and returns the finished file.
func PDF(d Dokument) []byte {
	return PDFMehrere([]Dokument{d}, d.Titel)
}

// PDFMehrere setzt mehrere Seiten in EIN PDF, jede auf einer neuen Seite.
//
// Für einen ganzen Space ist das nützlicher als ein Archiv voller Einzeldateien:
// ein Dokument lässt sich durchblättern, drucken und weiterreichen. Wer die
// Seiten einzeln braucht, nimmt den Markdown-Export.
func PDFMehrere(docs []Dokument, fuss string) []byte {
	s := neuerSetzer(fuss, fuss)
	for i, d := range docs {
		if i > 0 {
			s.neueSeite()
		}
		if d.Titel != "" {
			s.y -= 6
			for _, z := range umbrechen([]wort{{d.Titel, fFett, 24, false, false, false}}, satzBreite) {
				s.zeileSetzen(z, randLinks, 30)
			}
			s.y -= 14
		}
		for _, a := range d.Absatz {
			s.absatzSetzen(a)
		}
	}
	return s.fertig()
}

func (s *setzer) absatzSetzen(a Absatz) {
	const grund = 10.5
	const zeilenHoehe = 15.5
	einzug := randLinks + float64(a.Stufe)*16
	if a.Art == ArtUeberschrift {
		einzug = randLinks
	}
	breite := seiteBreite - randRechts - einzug

	switch a.Art {
	case ArtUeberschrift:
		groesse := ueberschriftGroesse[a.Stufe]
		if groesse == 0 {
			groesse = 12
		}
		s.y -= 10
		// Eine Überschrift allein am Seitenfuß ist ein Versprechen, das die
		// Seite nicht mehr einlöst, dann lieber gleich umbrechen.
		s.platzPruefen(groesse*1.5 + 24)
		for _, z := range umbrechen(alsWoerter(a.Text, groesse, true), satzBreite) {
			s.zeileSetzen(z, randLinks, groesse*1.35)
		}
		s.y -= 5

	case ArtAufzaehlung, ArtNummer, ArtAufgabe:
		marke := "•  "
		switch a.Art {
		case ArtNummer:
			marke = fmt.Sprintf("%d.  ", a.Nummer)
		case ArtAufgabe:
			marke = "[ ]  "
			if a.Erledigt {
				marke = "[x]  "
			}
		}
		markenBreite := textBreite(marke, fNormal, grund)
		zeilen := umbrechen(alsWoerter(a.Text, grund, false), breite-markenBreite)
		for i, z := range zeilen {
			if i == 0 {
				z = append([]wort{{marke, fNormal, grund, false, false, false}}, z...)
				s.zeileSetzen(z, einzug, zeilenHoehe)
			} else {
				// Folgezeilen richten sich an der Textspalte aus, nicht an der
				// Marke, sonst steht der Text unter dem Punkt statt daneben.
				s.zeileSetzen(z, einzug+markenBreite, zeilenHoehe)
			}
		}

	case ArtCode:
		s.y -= 4
		for _, roh := range a.Zeilen {
			// Codezeilen werden nicht umbrochen, sondern bei Bedarf hart
			// getrennt: ein Umbruch an Leerzeichen würde Einrückung erfinden,
			// die im Code nicht steht.
			rest := roh
			for {
				schnitt := len(rest)
				for schnitt > 1 && textBreite(rest[:schnitt], fFest, 9.5) > breite-10 {
					schnitt--
				}
				s.platzPruefen(13)
				s.y -= 13
				// A tint behind it, so the block reads as a block.
				fmt.Fprintf(s.akt, "0.95 0.95 0.95 rg %.2f %.2f %.2f %.2f re f 0 0 0 rg\n",
					einzug, s.y-3.5, breite, 13.0)
				fmt.Fprintf(s.akt, "BT /%s 9.5 Tf %.2f %.2f Td %s Tj ET\n",
					fFest, einzug+5, s.y, pdfZeichenkette(rest[:schnitt]))
				if schnitt >= len(rest) {
					break
				}
				rest = rest[schnitt:]
			}
		}
		s.y -= 8

	case ArtZitat:
		zeilen := umbrechen(alsWoerter(a.Text, grund, false), breite-14)
		for _, z := range zeilen {
			s.platzPruefen(zeilenHoehe)
			// Der Balken wird vor der Zeile gezeichnet, weil zeileSetzen y
			// bereits verschiebt.
			fmt.Fprintf(s.akt, "0.75 0.75 0.75 rg %.2f %.2f 2.5 %.2f re f 0 0 0 rg\n",
				einzug, s.y-zeilenHoehe+2, zeilenHoehe)
			s.zeileSetzen(z, einzug+12, zeilenHoehe)
		}
		s.y -= 6

	case ArtTabelle:
		s.tabelleSetzen(a.Tabelle, einzug, breite)

	case ArtTrenner:
		s.platzPruefen(14)
		s.y -= 8
		fmt.Fprintf(s.akt, "0.8 0.8 0.8 RG %.2f %.2f m %.2f %.2f l S 0 0 0 RG\n",
			einzug, s.y, einzug+breite, s.y)
		s.y -= 8

	case ArtDatei:
		w := alsWoerter(a.Text, grund, false)
		w = append([]wort{{"Datei: ", fNormal, grund, false, false, false}}, w...)
		for _, z := range umbrechen(w, breite) {
			s.zeileSetzen(z, einzug, zeilenHoehe)
		}
		s.y -= 4

	default:
		if len(a.Text) == 0 || strings.TrimSpace(NurText(a.Text)) == "" {
			s.y -= 7 // Leerzeile
			return
		}
		for _, z := range umbrechen(alsWoerter(a.Text, grund, false), breite) {
			s.zeileSetzen(z, einzug, zeilenHoehe)
		}
		s.y -= 6
	}
}

// tabelleSetzen zeichnet ein einfaches Gitter mit gleich breiten Spalten.
//
// Gleich breit und nicht nach Inhalt bemessen: eine Spaltenbreite aus dem
// Inhalt zu errechnen klingt besser, führt aber bei einer langen Zelle dazu,
// dass alle anderen auf ein paar Punkte zusammenschnurren. Gleiche Breiten sind
// immer lesbar.
func (s *setzer) tabelleSetzen(zeilen [][]string, x, breite float64) {
	if len(zeilen) == 0 {
		return
	}
	spalten := len(zeilen[0])
	if spalten == 0 {
		return
	}
	spaltenBreite := breite / float64(spalten)
	s.y -= 6

	for zi, z := range zeilen {
		schrift := fNormal
		if zi == 0 {
			schrift = fFett
		}
		// The tallest cell decides the row height.
		umbrochen := make([][][]wort, spalten)
		hoch := 1
		for i := 0; i < spalten; i++ {
			inhalt := ""
			if i < len(z) {
				inhalt = z[i]
			}
			umbrochen[i] = [][]wort{}
			for _, zz := range umbrechen([]wort{{inhalt, schrift, 9.5, false, false, false}}, spaltenBreite-8) {
				umbrochen[i] = append(umbrochen[i], zz)
			}
			if len(umbrochen[i]) > hoch {
				hoch = len(umbrochen[i])
			}
		}
		zellHoehe := float64(hoch) * 12.5
		s.platzPruefen(zellHoehe + 4)

		obenY := s.y
		for i := 0; i < spalten; i++ {
			zx := x + float64(i)*spaltenBreite
			s.y = obenY
			for _, zz := range umbrochen[i] {
				s.y -= 12.5
				lauf := zx + 4
				for _, w := range zz {
					fmt.Fprintf(s.akt, "BT /%s %.1f Tf %.2f %.2f Td %s Tj ET\n",
						w.schrift, w.groesse, lauf, s.y+3, pdfZeichenkette(w.text))
					lauf += textBreite(w.text, w.schrift, w.groesse)
				}
			}
			// The cell border.
			fmt.Fprintf(s.akt, "0.8 0.8 0.8 RG %.2f %.2f %.2f %.2f re S 0 0 0 RG\n",
				zx, obenY-zellHoehe, spaltenBreite, zellHoehe)
		}
		s.y = obenY - zellHoehe
	}
	s.y -= 10
}

// fertig assembles the PDF file from the pages that were set.
func (s *setzer) fertig() []byte {
	// A footer on every page: title on the left, page number on the right.
	for i, seite := range s.seiten {
		text := fmt.Sprintf("%d von %d", i+1, len(s.seiten))
		b := textBreite(text, fNormal, 8.5)
		fmt.Fprintf(seite, "0.5 0.5 0.5 rg\n")
		if s.fuss != "" {
			fmt.Fprintf(seite, "BT /%s 8.5 Tf %.2f %.2f Td %s Tj ET\n",
				fNormal, randLinks, randUnten-24, pdfZeichenkette(kuerzen(s.fuss, 70)))
		}
		fmt.Fprintf(seite, "BT /%s 8.5 Tf %.2f %.2f Td %s Tj ET\n",
			fNormal, seiteBreite-randRechts-b, randUnten-24, pdfZeichenkette(text))
		fmt.Fprintf(seite, "0 0 0 rg\n")
	}

	var out bytes.Buffer
	// Objektnummern werden fortlaufend vergeben; ihre Byteposition landet in
	// der Querverweistabelle am Ende.
	var pos []int
	objekt := func(inhalt string) {
		pos = append(pos, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(pos), inhalt)
	}
	objektRoh := func(kopf string, strom []byte) {
		pos = append(pos, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nstream\n", len(pos), kopf)
		out.Write(strom)
		out.WriteString("\nendstream\nendobj\n")
	}

	out.WriteString("%PDF-1.4\n")
	// Ein Kommentar mit hohen Bytes sagt jedem Werkzeug, dass die Datei binär
	// ist und nicht zeilenweise umkodiert werden darf.
	out.WriteString("%\xe2\xe3\xcf\xd3\n")

	anzahl := len(s.seiten)
	// 1: catalog, 2: page tree, 3..: fonts, then page plus content per page.
	ersteSchrift := 3
	ersteSeite := ersteSchrift + 6

	objekt("<< /Type /Catalog /Pages 2 0 R >>")

	var kinder strings.Builder
	for i := 0; i < anzahl; i++ {
		fmt.Fprintf(&kinder, "%d 0 R ", ersteSeite+i*2)
	}
	objekt(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", anzahl, strings.TrimSpace(kinder.String())))

	schriften := []struct{ kennung, name string }{
		{fNormal, "Helvetica"}, {fFett, "Helvetica-Bold"},
		{fKursiv, "Helvetica-Oblique"}, {fFettKursiv, "Helvetica-BoldOblique"},
		{fFest, "Courier"}, {fFestFett, "Courier-Bold"},
	}
	for _, f := range schriften {
		objekt(fmt.Sprintf(
			"<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding /WinAnsiEncoding >>", f.name))
	}

	var ressourcen strings.Builder
	ressourcen.WriteString("<< /Font << ")
	for i, f := range schriften {
		fmt.Fprintf(&ressourcen, "/%s %d 0 R ", f.kennung, ersteSchrift+i)
	}
	ressourcen.WriteString(">> >>")

	for i, seite := range s.seiten {
		inhaltNr := ersteSeite + i*2 + 1
		objekt(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources %s /Contents %d 0 R >>",
			seiteBreite, seiteHoehe, ressourcen.String(), inhaltNr))
		gepackt := packen(seite.Bytes())
		objektRoh(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>", len(gepackt)), gepackt)
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(pos)+1)
	for _, p := range pos {
		fmt.Fprintf(&out, "%010d 00000 n \n", p)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(pos)+1, xref)
	return out.Bytes()
}

func packen(roh []byte) []byte {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(roh)
	w.Close()
	return b.Bytes()
}

func kuerzen(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
