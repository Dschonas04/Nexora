// Word output (.docx).
//
// A docx file is a ZIP with XML inside. That can be written with archive/zip and
// encoding/xml from the standard library; a third party package would mainly
// bring bulk for this purpose.
//
// Bullet and numbered lists are written with a typeset character and an indent,
// not through Word's numbering definitions. That is a deliberate decision:
// numbering.xml is the part of the format Word takes offence at fastest, and a
// file Word reports as damaged is worthless, while a list that reads correctly
// as text but does not renumber itself on a click is not.
package dok

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// wxml escapes text for XML.
func wxml(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
	)
	// Control characters are not allowed in XML 1.0 and make the file unreadable;
	// Word then refuses to open it at all.
	var b strings.Builder
	for _, c := range s {
		if c < 0x20 && c != '\t' && c != '\n' {
			continue
		}
		b.WriteRune(c)
	}
	return r.Replace(b.String())
}

// lauf writes one run of text as w:r.
func lauf(s Stueck, groesse int, immerFett bool) string {
	var eig strings.Builder
	if s.Fett || immerFett {
		eig.WriteString("<w:b/>")
	}
	if s.Kursiv {
		eig.WriteString("<w:i/>")
	}
	if s.Durch {
		eig.WriteString("<w:strike/>")
	}
	if s.Unter || s.Verweis != "" {
		eig.WriteString(`<w:u w:val="single"/>`)
	}
	if s.Fest {
		eig.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/>`)
	}
	if s.Verweis != "" {
		eig.WriteString(`<w:color w:val="1A6FBF"/>`)
	} else if c, ok := schriftfarbe(s.Farbe); ok {
		fmt.Fprintf(&eig, `<w:color w:val="%s"/>`, c.hex())
	}
	// Word does not mark with a color value but with a name from a fixed
	// palette. For that reason the mapping is in farben.go instead of the
	// hex value: an arbitrary value could be present in the XML but Word
	// would ignore it.
	if marker, ok := wordMarker[s.Hintergrund]; ok {
		fmt.Fprintf(&eig, `<w:highlight w:val="%s"/>`, marker)
	}
	// Half points: w:sz uses half-point units.
	fmt.Fprintf(&eig, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, groesse*2, groesse*2)

	// xml:space="preserve" is not cosmetic: without the attribute Word throws
	// away leading and trailing spaces, and "a **b** c" becomes "a**b**c".
	return fmt.Sprintf(`<w:r><w:rPr>%s</w:rPr><w:t xml:space="preserve">%s</w:t></w:r>`,
		eig.String(), wxml(s.Text))
}

// absatzXML setzt einen Absatz.
func absatzXML(a Absatz) string {
	const grund = 11

	rahmen := func(eig, inhalt string) string {
		return "<w:p><w:pPr>" + eig + "</w:pPr>" + inhalt + "</w:p>"
	}
	laeufe := func(groesse int, fett bool) string {
		var b strings.Builder
		for _, s := range a.Text {
			b.WriteString(lauf(s, groesse, fett))
		}
		return b.String()
	}
	einzug := func(stufe int, zusatz int) string {
		// Word counts in twips: 1/20 point, 1440 to an inch.
		return fmt.Sprintf(`<w:ind w:left="%d" w:hanging="%d"/>`, stufe*360+zusatz, zusatz)
	}

	switch a.Art {
	case ArtUeberschrift:
		groessen := map[int]int{1: 20, 2: 16, 3: 14, 4: 13, 5: 12, 6: 11}
		g := groessen[a.Stufe]
		if g == 0 {
			g = 12
		}
		return rahmen(
			fmt.Sprintf(`<w:pStyle w:val="Heading%d"/><w:spacing w:before="240" w:after="120"/>`, a.Stufe),
			laeufe(g, true))

	case ArtAufzaehlung, ArtNummer, ArtAufgabe:
		marke := "•\t"
		switch a.Art {
		case ArtNummer:
			marke = fmt.Sprintf("%d.\t", a.Nummer)
		case ArtAufgabe:
			marke = "☐\t"
			if a.Erledigt {
				marke = "☒\t"
			}
		}
		return rahmen(
			einzug(a.Stufe+1, 360)+`<w:spacing w:after="40"/>`,
			lauf(Stueck{Text: marke}, grund, false)+laeufe(grund, false))

	case ArtCode:
		var b strings.Builder
		for _, z := range a.Zeilen {
			b.WriteString(rahmen(
				`<w:shd w:val="clear" w:fill="F2F2F2"/><w:spacing w:after="0"/>`+
					fmt.Sprintf(`<w:ind w:left="%d"/>`, a.Stufe*360+120),
				lauf(Stueck{Text: z, Fest: true}, 10, false)))
		}
		return b.String()

	case ArtZitat:
		return rahmen(
			fmt.Sprintf(`<w:ind w:left="%d"/>`, a.Stufe*360+480)+
				`<w:pBdr><w:left w:val="single" w:sz="18" w:space="8" w:color="C8C8C8"/></w:pBdr>`,
			laeufe(grund, false))

	case ArtTabelle:
		return tabelleXML(a.Tabelle)

	case ArtTrenner:
		return `<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:color="C8C8C8"/></w:pBdr></w:pPr></w:p>`

	case ArtDatei:
		return rahmen(fmt.Sprintf(`<w:ind w:left="%d"/>`, a.Stufe*360),
			lauf(Stueck{Text: "Datei: "}, grund, false)+laeufe(grund, false))

	default:
		if strings.TrimSpace(NurText(a.Text)) == "" {
			return "<w:p/>"
		}
		return rahmen(
			fmt.Sprintf(`<w:ind w:left="%d"/><w:spacing w:after="120"/>`, a.Stufe*360),
			laeufe(grund, false))
	}
}

func tabelleXML(zeilen [][]string) string {
	if len(zeilen) == 0 || len(zeilen[0]) == 0 {
		return ""
	}
	spalten := len(zeilen[0])
	// 9026 twips is the type width of an A4 page with the margins set below;
	// divided evenly.
	spaltenBreite := 9026 / spalten

	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/>` +
		`<w:tblW w:w="0" w:type="auto"/><w:tblBorders>`)
	for _, seite := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		fmt.Fprintf(&b, `<w:%s w:val="single" w:sz="4" w:color="C8C8C8"/>`, seite)
	}
	b.WriteString(`</w:tblBorders></w:tblPr><w:tblGrid>`)
	for i := 0; i < spalten; i++ {
		fmt.Fprintf(&b, `<w:gridCol w:w="%d"/>`, spaltenBreite)
	}
	b.WriteString(`</w:tblGrid>`)

	for zi, z := range zeilen {
		b.WriteString("<w:tr>")
		for i := 0; i < spalten; i++ {
			inhalt := ""
			if i < len(z) {
				inhalt = z[i]
			}
			schattierung := ""
			if zi == 0 {
				schattierung = `<w:shd w:val="clear" w:fill="F2F2F2"/>`
			}
			fmt.Fprintf(&b,
				`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s</w:tcPr>`+
					`<w:p><w:pPr><w:spacing w:after="0"/></w:pPr>%s</w:p></w:tc>`,
				spaltenBreite, schattierung, lauf(Stueck{Text: inhalt, Fett: zi == 0}, 10, false))
		}
		b.WriteString("</w:tr>")
	}
	b.WriteString("</w:tbl>")
	// Word needs a paragraph after a table, otherwise the next table attaches
	// directly to it and both merge while editing.
	b.WriteString("<w:p/>")
	return b.String()
}

// koerperMitBildern setzt einen Absatz und legt ein Bild, wenn es eines ist,
// zugleich als Teil des Archivs an.
//
// Die Bildteile wachsen dabei mit: der Text nennt sie ueber ihre
// Beziehungskennung, und dieselbe Liste fuellt spaeter word/media und die rels.
func koerperMitBildern(a Absatz, teile *[]wordBildTeil) string {
	if a.Art == ArtDatei && len(a.BildDaten) > 0 {
		if t, ok := bildTeilAnlegen(a.BildDaten, len(*teile)); ok {
			*teile = append(*teile, t)
			xml := bildXML(t, a.Stufe*360)
			// Die Unterschrift darunter, klein und kursiv. Der Name steht schon
			// im Bild und wird nicht wiederholt.
			if u := bildUnterschrift(a.Text); u != "" {
				xml += "<w:p><w:pPr>" +
					fmt.Sprintf(`<w:ind w:left="%d"/><w:spacing w:after="160"/>`, a.Stufe*360) +
					"</w:pPr>" + lauf(Stueck{Text: u, Kursiv: true}, 9, false) + "</w:p>"
			}
			return xml
		}
	}
	return absatzXML(a)
}

// Word writes a document as .docx.
func Word(d Dokument) ([]byte, error) {
	return WordMehrere([]Dokument{d})
}

// WordMehrere writes several pages into ONE document, separated by a page break.
func WordMehrere(docs []Dokument) ([]byte, error) {
	var koerper strings.Builder
	// The images referenced in the text. They are collected while composing
	// the body because only then it becomes clear which images can actually
	// be embedded.
	var bilder []wordBildTeil
	for i, d := range docs {
		if i > 0 {
			koerper.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
		if d.Titel != "" {
			// As a first level heading, not as a bold paragraph: Word then shows it
			// in the navigation pane, and on reading it back it can be recognised as
			// a title. Before it was merely large and bold, the same thing to the
			// eye, nothing to any program.
			koerper.WriteString(`<w:p><w:pPr><w:pStyle w:val="Heading1"/>` +
				`<w:spacing w:after="240"/></w:pPr>` +
				lauf(Stueck{Text: d.Titel, Fett: true}, 24, false) + `</w:p>`)
		}
		for _, a := range d.Absatz {
			koerper.WriteString(koerperMitBildern(a, &bilder))
		}
	}
	// Page setup: A4 portrait with a 2 cm margin all round (in twips).
	koerper.WriteString(`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
		`<w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"` +
		` w:header="708" w:footer="708" w:gutter="0"/></w:sectPr>`)

	dokument := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>` + koerper.String() + `</w:body></w:document>`

	typen := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
` + bildTypen(bilder) + `</Types>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	dokRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
` + bildBeziehungen(bilder) + `</Relationships>`

	// Only the styles referred to above. Word adds whatever it misses when
	// saving; what stands here has to be right though, a reference to a style that
	// does not exist is an error in the file.
	var vorlagen strings.Builder
	vorlagen.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults><w:rPrDefault><w:rPr>
<w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/>
</w:rPr></w:rPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
<w:style w:type="table" w:styleId="TableGrid"><w:name w:val="Table Grid"/></w:style>`)
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&vorlagen,
			`<w:style w:type="paragraph" w:styleId="Heading%d"><w:name w:val="heading %d"/>`+
				`<w:basedOn w:val="Normal"/><w:qFormat/></w:style>`, i, i)
	}
	vorlagen.WriteString(`</w:styles>`)

	var puffer bytes.Buffer
	z := zip.NewWriter(&puffer)
	for _, teil := range []struct{ name, inhalt string }{
		{"[Content_Types].xml", typen},
		{"_rels/.rels", rels},
		{"word/_rels/document.xml.rels", dokRels},
		{"word/document.xml", dokument},
		{"word/styles.xml", vorlagen.String()},
	} {
		w, err := z.Create(teil.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(teil.inhalt)); err != nil {
			return nil, err
		}
	}
	// The image files themselves. Use STORE not DEFLATE: PNG and JPEG are
	// already compressed, a second pass wastes time with no benefit.
	for _, b := range bilder {
		w, err := z.CreateHeader(&zip.FileHeader{Name: "word/media/" + b.name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(b.daten); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return puffer.Bytes(), nil
}
