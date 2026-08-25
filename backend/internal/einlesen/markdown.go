// Reading Markdown back in.
//
// handlers/markdown.go writes documents out; this reads them in. The two are
// deliberately not one reversible mapping. What arrives here may have been
// written anywhere, an Obsidian vault, a Notion export, a git wiki, a folder
// of notes, so the reader has to cope with ordinary Markdown, not only with
// what this system's own writer produces.
//
// It implements a subset of CommonMark, and the subset is chosen by what the
// editor can actually hold. BlockNote 0.15 knows paragraphs, headings in three
// levels, the three kinds of list, tables, images and seven inline styles. It
// has no code block, no quote block and no divider. Those therefore arrive as
// marked-up paragraphs: the frame is lost, every character is kept. Emitting a
// block type the editor does not know would be worse than lossy, the page
// would refuse to open.
//
// Nothing here fails. An unreadable construct becomes text, because an import
// that swallows a document is worse than one that imports it imperfectly.
package einlesen

import (
	"regexp"
	"strings"
)

// Inline ist ein Textstück im Editor-Dokument. Alle Felder tragen omitempty:
// BlockNote nimmt unvollständige Blöcke an und ergänzt die Vorgaben selbst, und
// eine Ausgabe ohne leere Felder lässt sich von Hand lesen.
type Inline struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Styles  map[string]bool `json:"styles,omitempty"`
	Href    string          `json:"href,omitempty"`
	Content []Inline        `json:"content,omitempty"`
}

// Block ist ein Knoten des Dokuments. Content ist bewusst any: bei den meisten
// Blöcken steht dort eine Liste von Inline-Stücken, bei einer Tabelle ein
// Objekt mit Zeilen.
type Block struct {
	Type     string         `json:"type"`
	Props    map[string]any `json:"props,omitempty"`
	Content  any            `json:"content,omitempty"`
	Children []Block        `json:"children,omitempty"`
}

// TabellenInhalt ist die Gestalt, die BlockNote für Tabellen erwartet.
type TabellenInhalt struct {
	Type string          `json:"type"`
	Rows []TabellenZeile `json:"rows"`
}

type TabellenZeile struct {
	Cells [][]Inline `json:"cells"`
}

// Kopf ist der Vorspann einer Datei, die Angaben, die vor dem Text stehen und
// nicht zum Text gehören.
type Kopf struct {
	Titel string
	Tags  []string
	Icon  string
}

// knoten hält den Baum während des Lesens. Block.Children ist eine Liste von
// Werten, keine von Zeigern; in eine wachsende Liste hinein zu verschachteln
// hieße, mit Zeigern zu arbeiten, die der nächste Anhang ungültig macht. Also
// wird erst ein Zeigerbaum gebaut und am Ende einmal umgeschrieben.
type knoten struct {
	blk    Block
	kinder []*knoten
}

func (k *knoten) bauen() Block {
	b := k.blk
	for _, kind := range k.kinder {
		b.Children = append(b.Children, kind.bauen())
	}
	return b
}

var (
	ueberschriftMuster = regexp.MustCompile(`^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)
	// Ohne Rückverweis, den Go nicht kennt: drei gleiche Zeichen, je Sorte einmal.
	trennerMuster     = regexp.MustCompile(`^ {0,3}(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$`)
	aufzaehlungMuster = regexp.MustCompile(`^(\s*)([-*+])\s+(.*)$`)
	nummerMuster      = regexp.MustCompile(`^(\s*)(\d{1,9})[.)]\s+(.*)$`)
	hakenMuster       = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)
	zaunMuster        = regexp.MustCompile("^ {0,3}(```+|~~~+)\\s*(\\S*)")
	trennzeileMuster  = regexp.MustCompile(`^ {0,3}\|?\s*:?-{2,}:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	bildAlleinMuster  = regexp.MustCompile(`^!\[([^\]]*)\]\(\s*(<[^>]*>|[^\s)]+)(?:\s+"[^"]*")?\s*\)$`)
)

// Lies wandelt eine Markdown-Datei in Blöcke um.
//
// Der zurückgegebene Titel ist die Überschrift, die ganz oben stand, sie wird
// aus dem Text entfernt, weil der Titel in Nexora ein eigenes Feld ist und
// sonst zweimal auf der Seite stünde. Steht dort keine, bleibt er leer und der
// Aufrufer nimmt den Dateinamen.
func Lies(md string) (string, Kopf, []Block) {
	kopf, rest := Vorspann(md)
	zeilen := strings.Split(strings.ReplaceAll(strings.ReplaceAll(rest, "\r\n", "\n"), "\r", "\n"), "\n")

	titel := kopf.Titel
	// Führende Leerzeilen weg, damit die Überschrift auch dann als erste Zeile
	// gilt, wenn hinter dem Vorspann noch eine Leerzeile stand.
	for len(zeilen) > 0 && strings.TrimSpace(zeilen[0]) == "" {
		zeilen = zeilen[1:]
	}
	if len(zeilen) > 0 {
		if m := ueberschriftMuster.FindStringSubmatch(zeilen[0]); m != nil && len(m[1]) == 1 {
			if titel == "" {
				titel = NurText(inlineAus(m[2], nil))
			}
			zeilen = zeilen[1:]
		}
	}
	return titel, kopf, bloeckeAus(zeilen)
}

// Vorspann liest den YAML-Kopf zwischen zwei Zeilen aus drei Strichen.
//
// Bewusst kein YAML-Leser: gebraucht werden drei Angaben, und eine Bibliothek
// für Dateien einzubinden, die ohnehin meist keinen Vorspann haben, steht in
// keinem Verhältnis. Was nicht verstanden wird, wird übergangen, der Kopf
// darf den Import einer Datei nicht verhindern.
func Vorspann(md string) (Kopf, string) {
	var k Kopf
	s := strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return k, md
	}
	ende := strings.Index(s[4:], "\n---")
	if ende < 0 {
		return k, md
	}
	block := s[4 : 4+ende]
	rest := s[4+ende:]
	// Hinter dem schließenden --- den Rest der Zeile abschneiden.
	if i := strings.Index(rest[1:], "\n"); i >= 0 {
		rest = rest[i+2:]
	} else {
		rest = ""
	}

	letzterSchluessel := ""
	for _, z := range strings.Split(block, "\n") {
		// Ein Listeneintrag gehört zum Schlüssel darüber: tags:\n  - eins
		if eintrag := strings.TrimSpace(z); strings.HasPrefix(eintrag, "- ") && letzterSchluessel == "tags" {
			k.Tags = append(k.Tags, saubereMarke(eintrag[2:]))
			continue
		}
		doppel := strings.Index(z, ":")
		if doppel < 0 {
			continue
		}
		schluessel := strings.ToLower(strings.TrimSpace(z[:doppel]))
		wert := strings.TrimSpace(z[doppel+1:])
		letzterSchluessel = normSchluessel(schluessel)
		switch letzterSchluessel {
		case "titel":
			k.Titel = saubereMarke(wert)
		case "icon":
			k.Icon = saubereMarke(wert)
		case "tags":
			wert = strings.Trim(wert, "[]")
			for _, t := range strings.Split(wert, ",") {
				if t = saubereMarke(t); t != "" {
					k.Tags = append(k.Tags, t)
				}
			}
		}
	}
	return k, rest
}

// normSchluessel bringt die Schreibweisen der üblichen Werkzeuge auf einen
// Nenner. Obsidian schreibt "tags", Notion "Tags", Jekyll "title".
func normSchluessel(s string) string {
	switch s {
	case "title", "titel", "name":
		return "titel"
	case "tags", "tag", "schlagworte", "schlagwoerter", "keywords":
		return "tags"
	case "icon", "emoji", "symbol":
		return "icon"
	}
	return s
}

func saubereMarke(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(strings.TrimPrefix(s, "#"))
}

// NurText zieht den reinen Text aus Inline-Stücken. Wird für Titel gebraucht,
// die im Dokument ausgezeichnet waren, ein Seitentitel trägt keine Fettung.
func NurText(teile []Inline) string {
	var b strings.Builder
	for _, t := range teile {
		if len(t.Content) > 0 {
			b.WriteString(NurText(t.Content))
		}
		b.WriteString(t.Text)
	}
	return strings.TrimSpace(b.String())
}
