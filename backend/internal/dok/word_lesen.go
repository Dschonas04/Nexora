// Reading a Word file and turning it into editor blocks.
//
// The counterpart to docx.go. Together they close a circle: open a .docx, edit
// it in the editor, write it back as a .docx.
//
// What survives that trip and what does not is the most important statement in
// this file. Carried over are headings, paragraphs, bullet lists, numbered
// lists, tables and the bold, italic, underline and strikethrough marks.
// Nothing else: headers and footers, styles, comments, tracked changes, images,
// columns, borders, typefaces, colours.
//
// That is not negligence but the boundary of the undertaking: the editor knows
// ten kinds of block, Word knows hundreds. Whoever edits a Word file here and
// writes it back gets a clean document containing its content, not the same file
// with one line changed. Which is why the interface says so beforehand.
package dok

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// wordHauptteil is the part of a .docx that carries the text.
const wordHauptteil = "word/document.xml"

// AusWord reads a .docx and returns it as a Dokument.
func AusWord(roh []byte) (Dokument, error) {
	leser, err := zip.NewReader(bytes.NewReader(roh), int64(len(roh)))
	if err != nil {
		return Dokument{}, errors.New("keine lesbare Word-Datei")
	}
	var xmlRoh []byte
	for _, f := range leser.File {
		if f.Name != wordHauptteil {
			continue
		}
		auf, err := f.Open()
		if err != nil {
			return Dokument{}, errors.New("Hauptteil nicht lesbar")
		}
		// Bounded: a .docx with a gigantic uncompressed document.xml would
		// otherwise be a way to fill memory.
		xmlRoh, err = io.ReadAll(io.LimitReader(auf, 64<<20))
		auf.Close()
		if err != nil {
			return Dokument{}, errors.New("Hauptteil nicht lesbar")
		}
		break
	}
	if len(xmlRoh) == 0 {
		return Dokument{}, errors.New("kein Hauptteil in der Datei. Ist es wirklich eine .docx?")
	}
	return wordZuDokument(xmlRoh)
}

// The shape of document.xml, as far as it matters here.
type wDokument struct {
	Body wBody `xml:"body"`
}

type wBody struct {
	// Order matters: paragraphs and tables alternate. Hence one shared field
	// instead of two lists, which would put every table at the end.
	Inhalt []wInhalt `xml:",any"`
}

type wInhalt struct {
	XMLName xml.Name
	// Absatz
	Eigenschaften wAbsatzEigenschaften `xml:"pPr"`
	Laeufe        []wLauf              `xml:"r"`
	// Tabelle
	Zeilen []wZeile `xml:"tr"`
}

type wAbsatzEigenschaften struct {
	Stil    wWert  `xml:"pStyle"`
	Nummern wNumPr `xml:"numPr"`
}

type wNumPr struct {
	Ebene wWert `xml:"ilvl"`
	ID    wWert `xml:"numId"`
}

type wWert struct {
	Wert string `xml:"val,attr"`
}

type wLauf struct {
	Eigenschaften wLaufEigenschaften `xml:"rPr"`
	Text          []wText            `xml:"t"`
	Umbrueche     []struct{}         `xml:"br"`
	Tab           []struct{}         `xml:"tab"`
}

type wLaufEigenschaften struct {
	Fett   *struct{} `xml:"b"`
	Kursiv *struct{} `xml:"i"`
	Unter  *wWert    `xml:"u"`
	Durch  *struct{} `xml:"strike"`
}

type wText struct {
	Wert string `xml:",chardata"`
}

type wZeile struct {
	Zellen []wZelle `xml:"tc"`
}

type wZelle struct {
	Absaetze []wInhalt `xml:"p"`
}

func wordZuDokument(xmlRoh []byte) (Dokument, error) {
	var d wDokument
	if err := xml.Unmarshal(xmlRoh, &d); err != nil {
		return Dokument{}, errors.New("Word-Datei ist beschädigt")
	}

	var raus Dokument
	nummer := 0
	for _, teil := range d.Body.Inhalt {
		switch teil.XMLName.Local {
		case "p":
			a, istNummer := wordAbsatz(teil)
			if istNummer {
				nummer++
				a.Nummer = nummer
			} else if a.Art != ArtNummer {
				nummer = 0
			}
			// Collapse consecutive empty paragraphs: Word likes to use them as
			// spacers, and turning each into a blank line in the editor bloats the
			// document.
			if a.Art == ArtAbsatz && len(a.Text) == 0 {
				if len(raus.Absatz) > 0 &&
					raus.Absatz[len(raus.Absatz)-1].Art == ArtAbsatz &&
					len(raus.Absatz[len(raus.Absatz)-1].Text) == 0 {
					continue
				}
			}
			raus.Absatz = append(raus.Absatz, a)
		case "tbl":
			t := wordTabelle(teil)
			if len(t) > 0 {
				raus.Absatz = append(raus.Absatz, Absatz{Art: ArtTabelle, Tabelle: t})
			}
			nummer = 0
		}
	}

	// The title is the first level 1 heading; otherwise it stays empty and the
	// caller falls back to the file name.
	for i, a := range raus.Absatz {
		if a.Art == ArtUeberschrift && a.Stufe == 1 {
			raus.Titel = nurText(a.Text)
			raus.Absatz = append(raus.Absatz[:i], raus.Absatz[i+1:]...)
			break
		}
	}
	return raus, nil
}

// wordAbsatz turns one <w:p> into a paragraph. The second return value says
// whether it is an item of a numbered list.
func wordAbsatz(p wInhalt) (Absatz, bool) {
	a := Absatz{Art: ArtAbsatz}
	stil := strings.ToLower(p.Eigenschaften.Stil.Wert)

	switch {
	case strings.HasPrefix(stil, "heading"), strings.HasPrefix(stil, "berschrift"),
		strings.HasPrefix(stil, "überschrift"):
		a.Art = ArtUeberschrift
		a.Stufe = 1
		// The digit at the end of the style name is the level: "heading2".
		for _, z := range stil {
			if z >= '1' && z <= '6' {
				a.Stufe = int(z - '0')
			}
		}
	case strings.Contains(stil, "listparagraph"), strings.Contains(stil, "listenabsatz"):
		a.Art = ArtAufzaehlung
	}

	istNummer := false
	if p.Eigenschaften.Nummern.ID.Wert != "" {
		// numId says the paragraph belongs to a list. Whether bullets or digits
		// is recorded in numbering.xml, a separate file with a structure of its
		// own. Reading that as well would be a lot of work for a distinction one
		// changes in the editor with a single click. So: bullet list, unless the
		// style says otherwise explicitly.
		if a.Art != ArtUeberschrift {
			a.Art = ArtAufzaehlung
		}
		if strings.Contains(stil, "number") || strings.Contains(stil, "nummer") {
			a.Art = ArtNummer
			istNummer = true
		}
	}

	a.Text = wordStuecke(p.Laeufe)
	if a.Art == ArtUeberschrift && len(a.Text) == 0 {
		a.Art = ArtAbsatz
	}
	return a, istNummer
}

func wordStuecke(laeufe []wLauf) []Stueck {
	var raus []Stueck
	for _, l := range laeufe {
		var text strings.Builder
		for _, t := range l.Text {
			text.WriteString(t.Wert)
		}
		for range l.Tab {
			text.WriteString(" ")
		}
		for range l.Umbrueche {
			text.WriteString(" ")
		}
		if text.Len() == 0 {
			continue
		}
		raus = append(raus, Stueck{
			Text:   text.String(),
			Fett:   l.Eigenschaften.Fett != nil,
			Kursiv: l.Eigenschaften.Kursiv != nil,
			Durch:  l.Eigenschaften.Durch != nil,
			// <w:u w:val="none"/> means explicitly NOT underlined.
			Unter: l.Eigenschaften.Unter != nil && l.Eigenschaften.Unter.Wert != "none",
		})
	}
	return raus
}

func wordTabelle(t wInhalt) [][]string {
	var raus [][]string
	for _, z := range t.Zeilen {
		var zeile []string
		for _, zelle := range z.Zellen {
			var text []string
			for _, p := range zelle.Absaetze {
				if s := nurText(wordStuecke(p.Laeufe)); s != "" {
					text = append(text, s)
				}
			}
			zeile = append(zeile, strings.Join(text, " "))
		}
		if len(zeile) > 0 {
			raus = append(raus, zeile)
		}
	}
	return raus
}

func nurText(st []Stueck) string {
	var b strings.Builder
	for _, s := range st {
		b.WriteString(s.Text)
	}
	return strings.TrimSpace(b.String())
}

// NachBloecken translates a document into the blocks the editor understands.
//
// Only the types BlockNote knows: paragraph, heading (levels 1 to 3), bullet
// list, numbered list, table. An unknown one becomes a paragraph, because a
// block the editor does not know would keep the page from opening at all, and
// that would be the worse outcome.
func NachBloecken(d Dokument) []map[string]any {
	bloecke := []map[string]any{}
	for _, a := range d.Absatz {
		switch a.Art {
		case ArtUeberschrift:
			stufe := a.Stufe
			// BlockNote knows three levels. Deeper ones become the third rather
			// than being discarded: the structure suffers, the text remains.
			if stufe < 1 {
				stufe = 1
			}
			if stufe > 3 {
				stufe = 3
			}
			bloecke = append(bloecke, map[string]any{
				"type":    "heading",
				"props":   map[string]any{"level": stufe},
				"content": stueckeNachInhalt(a.Text),
			})
		case ArtAufzaehlung:
			bloecke = append(bloecke, map[string]any{
				"type":    "bulletListItem",
				"content": stueckeNachInhalt(a.Text),
			})
		case ArtNummer:
			bloecke = append(bloecke, map[string]any{
				"type":    "numberedListItem",
				"content": stueckeNachInhalt(a.Text),
			})
		case ArtTabelle:
			bloecke = append(bloecke, tabelleNachBlock(a.Tabelle))
		default:
			bloecke = append(bloecke, map[string]any{
				"type":    "paragraph",
				"content": stueckeNachInhalt(a.Text),
			})
		}
	}
	if len(bloecke) == 0 {
		// An empty document: the editor insists on at least one block.
		bloecke = append(bloecke, map[string]any{"type": "paragraph", "content": []any{}})
	}
	return bloecke
}

func stueckeNachInhalt(st []Stueck) []any {
	inhalt := []any{}
	for _, s := range st {
		if s.Text == "" {
			continue
		}
		stile := map[string]any{}
		if s.Fett {
			stile["bold"] = true
		}
		if s.Kursiv {
			stile["italic"] = true
		}
		if s.Unter {
			stile["underline"] = true
		}
		if s.Durch {
			stile["strike"] = true
		}
		if s.Fest {
			stile["code"] = true
		}
		inhalt = append(inhalt, map[string]any{
			"type": "text", "text": s.Text, "styles": stile,
		})
	}
	return inhalt
}

func tabelleNachBlock(zeilen [][]string) map[string]any {
	// Make it rectangular: BlockNote wants the same number of cells in every
	// row, while Word allows merged and therefore missing ones.
	breite := 0
	for _, z := range zeilen {
		if len(z) > breite {
			breite = len(z)
		}
	}
	rows := []any{}
	for _, z := range zeilen {
		cells := []any{}
		for i := 0; i < breite; i++ {
			var text string
			if i < len(z) {
				text = z[i]
			}
			inhalt := []any{}
			if text != "" {
				inhalt = append(inhalt, map[string]any{
					"type": "text", "text": text, "styles": map[string]any{},
				})
			}
			cells = append(cells, inhalt)
		}
		rows = append(rows, map[string]any{"cells": cells})
	}
	return map[string]any{
		"type": "table",
		"content": map[string]any{
			"type": "tableContent",
			"rows": rows,
		},
	}
}
