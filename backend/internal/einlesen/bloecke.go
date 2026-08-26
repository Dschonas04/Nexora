package einlesen

import (
	"strconv"
	"strings"
)

// stapelEintrag holds an open list item together with the column its marker
// stood in. An item indented further is its child.
type stapelEintrag struct {
	einzug int
	k      *knoten
}

// bloeckeAus reads the lines of a document.
//
// Line by line and without looking back further than one line: Markdown is line
// oriented, and a reader that can say at any moment which state it is in stays
// changeable. A complete CommonMark reader would be ten times the code for cases
// that do not occur in notes.
func bloeckeAus(zeilen []string) []Block {
	var wurzel []*knoten
	var stapel []stapelEintrag

	// anhaengen inserts a list item in the right place: below the last open
	// item that is indented less far.
	anhaengen := func(k *knoten, einzug int) {
		for len(stapel) > 0 && stapel[len(stapel)-1].einzug >= einzug {
			stapel = stapel[:len(stapel)-1]
		}
		if len(stapel) == 0 {
			ersterDerListe(k, wurzel)
			wurzel = append(wurzel, k)
		} else {
			eltern := stapel[len(stapel)-1].k
			ersterDerListe(k, eltern.kinder)
			eltern.kinder = append(eltern.kinder, k)
		}
		stapel = append(stapel, stapelEintrag{einzug: einzug, k: k})
	}
	// gerade ends the list: the next block sits at the far left again.
	gerade := func(k *knoten) {
		stapel = stapel[:0]
		wurzel = append(wurzel, k)
	}

	for i := 0; i < len(zeilen); i++ {
		z := zeilen[i]
		leer := strings.TrimSpace(z) == ""

		if leer {
			// A blank line ends a paragraph but not a list: one may stand between
			// two list items, and the list carries on.
			continue
		}

		// A code fence. Checked first, because inside the fence no other rule
		// applies: a "# " there is a hash, not a heading.
		if m := zaunMuster.FindStringSubmatch(z); m != nil {
			zaun := m[1][:1]
			var code []string
			i++
			for ; i < len(zeilen); i++ {
				if t := strings.TrimSpace(zeilen[i]); strings.HasPrefix(t, zaun+zaun+zaun) && strings.Trim(t, zaun) == "" {
					break
				}
				code = append(code, zeilen[i])
			}
			for _, b := range codeBloecke(code, m[2]) {
				gerade(&knoten{blk: b})
			}
			continue
		}

		// A heading.
		if m := ueberschriftMuster.FindStringSubmatch(z); m != nil {
			stufe := len(m[1])
			// The editor knows three levels. Merging deeper ones loses structure,
			// turning them into paragraphs would lose the heading altogether, and
			// in a three-level view a fourth level hardly shows anyway.
			if stufe > 3 {
				stufe = 3
			}
			gerade(&knoten{blk: Block{
				Type:    "heading",
				Props:   map[string]any{"level": stufe},
				Content: inlineAus(m[2], nil),
			}})
			continue
		}

		// A horizontal rule. The editor has none; three hyphens are the closest
		// thing that looks like one.
		if trennerMuster.MatchString(z) {
			gerade(&knoten{blk: Block{
				Type:    "paragraph",
				Content: []Inline{{Type: "text", Text: "———"}},
			}})
			continue
		}

		// A table: a row of vertical bars whose successor is the separator row.
		// Without that second condition every sentence containing a vertical bar
		// would be a table.
		if strings.Contains(z, "|") && i+1 < len(zeilen) && trennzeileMuster.MatchString(zeilen[i+1]) {
			var roh []string
			roh = append(roh, z)
			i++ // skip the separator row
			for i+1 < len(zeilen) && strings.Contains(zeilen[i+1], "|") && strings.TrimSpace(zeilen[i+1]) != "" {
				i++
				roh = append(roh, zeilen[i])
			}
			gerade(&knoten{blk: tabelle(roh)})
			continue
		}

		// A quote. The editor has no quote block, so the content is set in
		// italics and the ">" disappears. Several quote lines in a row belong to
		// one quote.
		if strings.HasPrefix(strings.TrimLeft(z, " "), ">") {
			var zitat []string
			for i < len(zeilen) && strings.HasPrefix(strings.TrimLeft(zeilen[i], " "), ">") {
				t := strings.TrimLeft(zeilen[i], " ")
				t = strings.TrimPrefix(t, ">")
				zitat = append(zitat, strings.TrimPrefix(t, " "))
				i++
			}
			i--
			for _, b := range bloeckeAus(zitat) {
				gerade(&knoten{blk: kursivMachen(b)})
			}
			continue
		}

		// Listeneintrag.
		if k, einzug, ok := listenEintrag(z); ok {
			anhaengen(k, einzug)
			continue
		}

		// A paragraph. Every following line belongs to it until a blank line or
		// a block of another kind arrives.
		// Collected raw, untrimmed: two spaces at the end of a line are a hard
		// break in Markdown, and trimming first loses it.
		var absatz []string
		absatz = append(absatz, z)
		for i+1 < len(zeilen) {
			n := zeilen[i+1]
			if strings.TrimSpace(n) == "" || beginntBlock(n) {
				break
			}
			i++
			absatz = append(absatz, n)
		}
		text := absatzText(absatz)

		// An indented line consisting entirely of a code span comes from a code
		// block. The editor has none; the export therefore writes every line as
		// `text`, and the indentation ends up in front of it instead of inside.
		// If it does not move into the code span, every indented code block
		// loses its levels when read back in, and indentation is not decoration
		// in code.
		if len(absatz) == 1 && len(stapel) == 0 {
			if einzug := absatz[0][:einzugVon(absatz[0])]; einzug != "" {
				if inneres, marke, ok := ganzCode(text); ok {
					text = marke + strings.ReplaceAll(einzug, "\t", "    ") + inneres + marke
				}
			}
		}

		// A line consisting of nothing but an image becomes an image block.
		// Wrapped in a paragraph it would be a link one has to click; what was
		// meant is an image one can see.
		if m := bildAlleinMuster.FindStringSubmatch(text); m != nil {
			gerade(&knoten{blk: bildBlock(m[1], m[2])})
			continue
		}

		// Continuation of a list item: indented prose below an item belongs to
		// it, not beside the list.
		if len(stapel) > 0 && einzugVon(z) > stapel[len(stapel)-1].einzug {
			letzter := stapel[len(stapel)-1].k
			letzter.blk.Content = append(letzter.blk.Content.([]Inline),
				append([]Inline{{Type: "text", Text: "\n"}}, inlineAus(text, nil)...)...)
			continue
		}

		gerade(&knoten{blk: Block{Type: "paragraph", Content: inlineAus(text, nil)}})
	}

	// A document without a single block would not be valid initial content for
	// the editor; an empty paragraph is the empty page.
	if len(wurzel) == 0 {
		return []Block{{Type: "paragraph"}}
	}
	out := make([]Block, 0, len(wurzel))
	for _, k := range wurzel {
		out = append(out, k.bauen())
	}
	return out
}

// ersterDerListe drops the start number when a numbered item already stands
// before it.
//
// BlockNote counts a list itself and only reads the number on the first item. If
// it stood on every item, each one would start a new list after saving, and
// "5. 6. 7." would turn into "5. 5. 5.".
func ersterDerListe(k *knoten, geschwister []*knoten) {
	if k.blk.Type != "numberedListItem" || len(geschwister) == 0 {
		return
	}
	if geschwister[len(geschwister)-1].blk.Type != "numberedListItem" {
		return
	}
	delete(k.blk.Props, "start")
	if len(k.blk.Props) == 0 {
		k.blk.Props = nil
	}
}

// ganzCode reports whether a line consists of nothing but a code span, and
// returns its content together with the backtick run that fences it.
func ganzCode(z string) (string, string, bool) {
	n := 0
	for n < len(z) && z[n] == '`' {
		n++
	}
	if n == 0 || len(z) < 2*n+1 {
		return "", "", false
	}
	marke := z[:n]
	if !strings.HasSuffix(z, marke) {
		return "", "", false
	}
	inneres := z[n : len(z)-n]
	// A run of backticks of the same length inside would mean the line consists
	// of several spans rather than one.
	if strings.Contains(inneres, marke) {
		return "", "", false
	}
	return inneres, marke, true
}

// beginntBlock reports whether a line starts a new block. Without it a list
// following a paragraph without a blank line would vanish into the paragraph.
func beginntBlock(z string) bool {
	if ueberschriftMuster.MatchString(z) || trennerMuster.MatchString(z) ||
		zaunMuster.MatchString(z) || aufzaehlungMuster.MatchString(z) ||
		nummerMuster.MatchString(z) {
		return true
	}
	return strings.HasPrefix(strings.TrimLeft(z, " "), ">")
}

func einzugVon(z string) int {
	return len(z) - len(strings.TrimLeft(z, " \t"))
}

// listenEintrag recognises bullets, numbers and checkboxes.
func listenEintrag(z string) (*knoten, int, bool) {
	if m := aufzaehlungMuster.FindStringSubmatch(z); m != nil {
		einzug := len(strings.ReplaceAll(m[1], "\t", "    "))
		rest := m[3]
		// "- [x] Text" is a checkbox, not a bullet.
		if h := hakenMuster.FindStringSubmatch(rest); h != nil {
			return &knoten{blk: Block{
				Type:    "checkListItem",
				Props:   map[string]any{"checked": h[1] == "x" || h[1] == "X"},
				Content: inlineAus(h[2], nil),
			}}, einzug, true
		}
		return &knoten{blk: Block{Type: "bulletListItem", Content: inlineAus(rest, nil)}}, einzug, true
	}
	if m := nummerMuster.FindStringSubmatch(z); m != nil {
		einzug := len(strings.ReplaceAll(m[1], "\t", "    "))
		props := map[string]any{}
		// A list starting at 5 should start at 5. BlockNote only remembers that
		// on the first item; on every other one the number would be wrong,
		// because the editor counts by itself.
		if n, err := strconv.Atoi(m[2]); err == nil && n != 1 {
			props["start"] = n
		}
		if len(props) == 0 {
			props = nil
		}
		return &knoten{blk: Block{Type: "numberedListItem", Props: props, Content: inlineAus(m[3], nil)}}, einzug, true
	}
	return nil, 0, false
}

// absatzText joins the lines of a paragraph.
//
// A soft break, a line that simply ends, becomes a space, as Markdown wants.
// Two spaces or a backslash at the end are a hard break and stay one: the editor
// then holds a line break inside the paragraph, and the export writes it back as
// "  \n". Without that distinction an address or a poem would run together into
// a single line on import.
func absatzText(zeilen []string) string {
	var b strings.Builder
	hartVorher := false
	for i, z := range zeilen {
		hart := strings.HasSuffix(z, "  ") || strings.HasSuffix(z, "\\")
		z = strings.TrimSpace(strings.TrimSuffix(strings.TrimRight(z, " "), "\\"))
		if i > 0 {
			if hartVorher {
				b.WriteString("\n")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(z)
		hartVorher = hart
	}
	return b.String()
}

// codeBloecke turns the lines of a code fence into paragraphs in a fixed-width
// face.
//
// The editor of this version has no code block. One paragraph per line keeps the
// breaks and the order; putting everything into one paragraph would look more
// contained in the editor but could no longer be edited line by line. The
// language tag is lost on the way, since no field the editor knows holds it.
func codeBloecke(zeilen []string, sprache string) []Block {
	var out []Block
	if sprache != "" {
		out = append(out, Block{
			Type:    "paragraph",
			Content: []Inline{{Type: "text", Text: sprache, Styles: map[string]bool{"code": true, "italic": true}}},
		})
	}
	for _, z := range zeilen {
		if z == "" {
			out = append(out, Block{Type: "paragraph"})
			continue
		}
		out = append(out, Block{
			Type:    "paragraph",
			Content: []Inline{{Type: "text", Text: z, Styles: map[string]bool{"code": true}}},
		})
	}
	return out
}

// kursivMachen sets a whole block in italics, the stand-in for the quote block
// the editor does not have.
func kursivMachen(b Block) Block {
	if teile, ok := b.Content.([]Inline); ok {
		for i := range teile {
			if teile[i].Styles == nil {
				teile[i].Styles = map[string]bool{}
			}
			teile[i].Styles["italic"] = true
		}
		b.Content = teile
	}
	for i := range b.Children {
		b.Children[i] = kursivMachen(b.Children[i])
	}
	return b
}

// tabelle reads the lines of a table. Every row is padded to the width of the
// widest one: a row with fewer cells than the header would otherwise leave the
// editor with an incomplete table.
func tabelle(zeilen []string) Block {
	var roh [][]string
	breite := 0
	for _, z := range zeilen {
		z := zellenTeilen(z)
		if len(z) > breite {
			breite = len(z)
		}
		roh = append(roh, z)
	}
	if breite == 0 {
		return Block{Type: "paragraph"}
	}
	inhalt := TabellenInhalt{Type: "tableContent"}
	for _, z := range roh {
		zeile := TabellenZeile{Cells: make([][]Inline, breite)}
		for i := range zeile.Cells {
			if i < len(z) {
				zeile.Cells[i] = inlineAus(z[i], nil)
			} else {
				zeile.Cells[i] = []Inline{}
			}
		}
		inhalt.Rows = append(inhalt.Rows, zeile)
	}
	return Block{Type: "table", Content: inhalt}
}

// zellenTeilen splits on vertical bars, but not on escaped ones: "\|" is a bar
// in the text, not a cell boundary.
func zellenTeilen(z string) []string {
	z = strings.TrimSpace(z)
	z = strings.TrimPrefix(z, "|")
	z = strings.TrimSuffix(z, "|")
	var zellen []string
	var b strings.Builder
	for i := 0; i < len(z); i++ {
		if z[i] == '\\' && i+1 < len(z) && z[i+1] == '|' {
			b.WriteByte('|')
			i++
			continue
		}
		if z[i] == '|' {
			zellen = append(zellen, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteByte(z[i])
	}
	zellen = append(zellen, strings.TrimSpace(b.String()))
	return zellen
}

// bildBlock builds the image block. The address keeps its shape; whether it
// points at a file inside the archive and has to be rewritten is decided by the
// caller, not by the reader.
func bildBlock(alt, adresse string) Block {
	adresse = strings.TrimSuffix(strings.TrimPrefix(adresse, "<"), ">")
	return Block{
		Type: "image",
		Props: map[string]any{
			"url":     adresse,
			"name":    alt,
			"caption": alt,
		},
	}
}
