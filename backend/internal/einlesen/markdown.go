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
	"encoding/json"
	"regexp"
	"strings"
)

// Inline is a piece of text in the editor document. All fields carry omitempty:
// BlockNote accepts incomplete blocks and fills in the defaults itself, and an
// output without empty fields can be read by hand.
type Inline struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Styles  map[string]bool `json:"styles"`
	Href    string          `json:"href,omitempty"`
	Content []Inline        `json:"content,omitempty"`
}

// MarshalJSON always writes the styles, even when there are none. BlockNote
// reads them with Object.entries; a missing or null field makes that throw, and
// the editor then refuses the whole document. The page an import produced would
// stay blank, and with it everything around it.
func (i Inline) MarshalJSON() ([]byte, error) {
	type roh Inline
	k := roh(i)
	if k.Styles == nil {
		k.Styles = map[string]bool{}
	}
	return json.Marshal(k)
}

// Block is a node of the document. Content is deliberately any: for most blocks
// it holds a list of inline pieces, for a table an object with rows.
type Block struct {
	Type     string         `json:"type"`
	Props    map[string]any `json:"props,omitempty"`
	Content  any            `json:"content,omitempty"`
	Children []Block        `json:"children,omitempty"`
}

// TabellenInhalt is the shape BlockNote expects for tables.
type TabellenInhalt struct {
	Type string          `json:"type"`
	Rows []TabellenZeile `json:"rows"`
}

type TabellenZeile struct {
	Cells [][]Inline `json:"cells"`
}

// Kopf is the front matter of a file, the entries standing before the text and
// not belonging to it.
type Kopf struct {
	Titel string
	Tags  []string
	Icon  string
}

// knoten holds the tree while reading. Block.Children is a list of values, not
// of pointers; nesting into a growing list would mean working with pointers that
// the next append invalidates. So a tree of pointers is built first and
// rewritten once at the end.
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
	// No backreference, which Go does not have: three equal characters, one per kind.
	trennerMuster     = regexp.MustCompile(`^ {0,3}(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$`)
	aufzaehlungMuster = regexp.MustCompile(`^(\s*)([-*+])\s+(.*)$`)
	nummerMuster      = regexp.MustCompile(`^(\s*)(\d{1,9})[.)]\s+(.*)$`)
	hakenMuster       = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)
	zaunMuster        = regexp.MustCompile("^ {0,3}(```+|~~~+)\\s*(\\S*)")
	trennzeileMuster  = regexp.MustCompile(`^ {0,3}\|?\s*:?-{2,}:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	bildAlleinMuster  = regexp.MustCompile(`^!\[([^\]]*)\]\(\s*(<[^>]*>|[^\s)]+)(?:\s+"[^"]*")?\s*\)$`)
)

// Lies turns a Markdown file into blocks.
//
// The returned title is the heading that stood at the very top; it is removed
// from the text because the title is a field of its own in Nexora and would
// otherwise stand twice on the page. If there is none it stays empty and the
// caller takes the file name.
func Lies(md string) (string, Kopf, []Block) {
	kopf, rest := Vorspann(md)
	zeilen := strings.Split(strings.ReplaceAll(strings.ReplaceAll(rest, "\r\n", "\n"), "\r", "\n"), "\n")

	titel := kopf.Titel
	// Drop leading blank lines so that the heading counts as the first line even
	// when a blank line followed the front matter.
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

// Vorspann reads the YAML head between two lines of three dashes.
//
// Deliberately no YAML parser: three entries are needed, and pulling in a
// library for files that mostly have no front matter at all is out of
// proportion. Whatever is not understood is skipped; the head must not prevent
// a file from being imported.
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
	// Cut off the rest of the line after the closing fence.
	if i := strings.Index(rest[1:], "\n"); i >= 0 {
		rest = rest[i+2:]
	} else {
		rest = ""
	}

	letzterSchluessel := ""
	for _, z := range strings.Split(block, "\n") {
		// A list entry belongs to the key above it: tags:\n  - one
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

// normSchluessel brings the spellings of the common tools onto one
// denominator. Obsidian writes "tags", Notion "Tags", Jekyll "title".
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

// NurText pulls the plain text out of inline pieces. Needed for titles that
// were styled in the document; a page title carries no bold.
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
