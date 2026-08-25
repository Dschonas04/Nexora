// Word-Ausgabe (.docx).
//
// Eine docx-Datei ist ein ZIP mit XML darin. Das lässt sich mit archive/zip und
// encoding/xml aus der Standardbibliothek schreiben; ein Fremdpaket brächte für
// diesen Zweck vor allem Umfang mit.
//
// Aufzählungen werden mit einem gesetzten Zeichen und Einzug geschrieben, nicht
// über die Nummerierungsdefinitionen von Word. Das ist eine bewusste
// Entscheidung: numbering.xml ist der Teil des Formats, an dem sich Word am
// schnellsten stört, und eine Datei, die Word als beschädigt meldet, ist
// wertlos, eine Liste, die als Text richtig dasteht, aber nicht per Klick
// weiternummeriert, ist es nicht.
package dok

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// wxml maskiert Text für XML.
func wxml(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
	)
	// Steuerzeichen sind in XML 1.0 nicht erlaubt und machen die Datei
	// unlesbar, Word öffnet sie dann gar nicht erst.
	var b strings.Builder
	for _, c := range s {
		if c < 0x20 && c != '\t' && c != '\n' {
			continue
		}
		b.WriteRune(c)
	}
	return r.Replace(b.String())
}

// lauf schreibt ein Textstück als w:r.
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
	}
	// Halbe Punkte: w:sz zählt in Halbpunkten.
	fmt.Fprintf(&eig, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, groesse*2, groesse*2)

	// xml:space="preserve" ist nicht kosmetisch: ohne das Attribut wirft Word
	// führende und folgende Leerzeichen weg, und aus "a **b** c" wird "a**b**c".
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
		// Word rechnet in Twips: 1/20 Punkt, 1440 auf ein Zoll.
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
	// 9026 Twips ist die Satzbreite einer A4-Seite mit den unten gesetzten
	// Rändern; gleichmäßig geteilt.
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
	// Word braucht nach einer Tabelle einen Absatz, sonst hängt die nächste
	// Tabelle direkt daran und beide verschmelzen beim Bearbeiten.
	b.WriteString("<w:p/>")
	return b.String()
}

// Word schreibt ein Dokument als .docx.
func Word(d Dokument) ([]byte, error) {
	return WordMehrere([]Dokument{d})
}

// WordMehrere schreibt mehrere Seiten in EIN Dokument, getrennt durch einen
// Seitenumbruch.
func WordMehrere(docs []Dokument) ([]byte, error) {
	var koerper strings.Builder
	for i, d := range docs {
		if i > 0 {
			koerper.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
		if d.Titel != "" {
			// Als Überschrift der ersten Ebene, nicht als fetter Absatz: Word
			// zeigt sie dann im Navigationsbereich, und beim Wiedereinlesen
			// ist sie als Titel zu erkennen. Vorher war sie nur groß und fett
			// -- für das Auge dasselbe, für jedes Programm nichts.
			koerper.WriteString(`<w:p><w:pPr><w:pStyle w:val="Heading1"/>` +
				`<w:spacing w:after="240"/></w:pPr>` +
				lauf(Stueck{Text: d.Titel, Fett: true}, 24, false) + `</w:p>`)
		}
		for _, a := range d.Absatz {
			koerper.WriteString(absatzXML(a))
		}
	}
	// Seitenformat: A4 hoch mit 2 cm Rand ringsum (in Twips).
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
</Types>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	dokRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

	// Nur die Formatvorlagen, auf die oben verwiesen wird. Word ergänzt beim
	// Speichern selbst, was ihm fehlt; was hier steht, muss aber stimmen,
	// ein Verweis auf eine Vorlage, die es nicht gibt, ist ein Fehler in der
	// Datei.
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
	if err := z.Close(); err != nil {
		return nil, err
	}
	return puffer.Bytes(), nil
}
