// Eine Word-Datei lesen und in Editorblöcke verwandeln.
//
// Das Gegenstück zu docx.go. Zusammen ergeben sie einen Kreis: eine .docx
// öffnen, im Editor ändern, wieder als .docx ablegen.
//
// Was dabei überlebt und was nicht, ist die wichtigste Aussage dieser Datei.
// Übernommen werden Überschriften, Absätze, Aufzählungen, nummerierte Listen,
// Tabellen und die Auszeichnungen fett, kursiv, unterstrichen, durchgestrichen.
// Alles Übrige nicht: Kopf- und Fußzeilen, Formatvorlagen, Kommentare,
// Änderungsverfolgung, Bilder, Spalten, Rahmen, Schriftarten, Farben.
//
// Das ist keine Nachlässigkeit, sondern die Grenze des Vorhabens: der Editor
// kennt zehn Blockarten, Word kennt hunderte. Wer eine Word-Datei hier
// bearbeitet und zurückschreibt, bekommt ein sauberes Dokument mit ihrem
// Inhalt -- nicht dieselbe Datei mit einer geänderten Zeile. Deshalb sagt die
// Oberfläche das vorher.
package dok

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// wordDatei ist der Teil einer .docx, der den Text trägt.
const wordHauptteil = "word/document.xml"

// AusWord liest eine .docx und gibt sie als Dokument zurück.
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
		// Begrenzt: eine .docx mit einem entpackt gigantischen document.xml
		// wäre sonst ein Weg, den Speicher vollzuschreiben.
		xmlRoh, err = io.ReadAll(io.LimitReader(auf, 64<<20))
		auf.Close()
		if err != nil {
			return Dokument{}, errors.New("Hauptteil nicht lesbar")
		}
		break
	}
	if len(xmlRoh) == 0 {
		return Dokument{}, errors.New("kein Hauptteil in der Datei -- ist es wirklich eine .docx?")
	}
	return wordZuDokument(xmlRoh)
}

// Die Struktur von document.xml, soweit sie hier zählt.
type wDokument struct {
	Body wBody `xml:"body"`
}

type wBody struct {
	// Reihenfolge zählt: Absätze und Tabellen wechseln sich ab. Deshalb ein
	// gemeinsames Feld statt zweier Listen -- sonst stünden alle Tabellen am
	// Ende.
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
			// Leere Absätze am Stück zusammenfassen: Word setzt sie gern als
			// Abstandshalter, und daraus im Editor je eine leere Zeile zu
			// machen bläht das Dokument auf.
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

	// Der Titel ist die erste Überschrift der Ebene 1, sonst bleibt er leer und
	// der Aufrufer nimmt den Dateinamen.
	for i, a := range raus.Absatz {
		if a.Art == ArtUeberschrift && a.Stufe == 1 {
			raus.Titel = nurText(a.Text)
			raus.Absatz = append(raus.Absatz[:i], raus.Absatz[i+1:]...)
			break
		}
	}
	return raus, nil
}

// wordAbsatz macht aus einem <w:p> einen Absatz. Der zweite Rückgabewert sagt,
// ob es ein Punkt einer nummerierten Liste ist.
func wordAbsatz(p wInhalt) (Absatz, bool) {
	a := Absatz{Art: ArtAbsatz}
	stil := strings.ToLower(p.Eigenschaften.Stil.Wert)

	switch {
	case strings.HasPrefix(stil, "heading"), strings.HasPrefix(stil, "berschrift"),
		strings.HasPrefix(stil, "überschrift"):
		a.Art = ArtUeberschrift
		a.Stufe = 1
		// Die Ziffer am Ende des Stilnamens ist die Ebene: "heading2".
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
		// numId sagt: der Absatz gehört zu einer Liste. Ob Punkte oder Ziffern,
		// steht in numbering.xml -- einer eigenen Datei mit eigener Struktur.
		// Sie zusätzlich zu lesen wäre viel Aufwand für eine Unterscheidung,
		// die man im Editor mit einem Klick ändert. Deshalb: Aufzählung, außer
		// der Stil sagt ausdrücklich etwas anderes.
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
			// <w:u w:val="none"/> heißt ausdrücklich NICHT unterstrichen.
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

// NachBloecken übersetzt ein Dokument in die Blöcke, die der Editor versteht.
//
// Nur die Typen, die BlockNote kennt: Absatz, Überschrift (Ebene 1 bis 3),
// Aufzählung, nummerierte Liste, Tabelle. Ein unbekannter wird ein Absatz --
// ein Block, den der Editor nicht kennt, ließe die Seite gar nicht erst
// öffnen, und das wäre der schlechtere Ausgang.
func NachBloecken(d Dokument) []map[string]any {
	bloecke := []map[string]any{}
	for _, a := range d.Absatz {
		switch a.Art {
		case ArtUeberschrift:
			stufe := a.Stufe
			// BlockNote kennt drei Ebenen. Tiefere werden zur dritten statt
			// verworfen: die Gliederung leidet, der Text bleibt.
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
		// Ein leeres Dokument: der Editor verlangt mindestens einen Block.
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
	// Rechteckig machen: BlockNote verlangt in jeder Zeile gleich viele
	// Zellen, Word erlaubt verbundene und damit fehlende.
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
