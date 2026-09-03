// Package dok brings a stored page document into formats that have a fixed page
// layout: PDF and Word.
//
// Why an intermediate form of its own and not the detour through Markdown:
// Markdown knows no margins, no font sizes and no page break. Whoever wants to
// typeset a PDF needs exactly that. The conversion to Markdown therefore stays
// where it is, it answers a different question.
//
// What is read is the same editor document. It is the same JSON tree, but with a
// different goal: here what counts is what a piece of text LOOKS like (bold,
// monospaced, linked), not which characters one would have to write into a file.
package dok

import (
	"encoding/json"
	"strings"
)

// Art says what a paragraph is. The formatters decide font size, indent and
// spacing from it.
type Art int

const (
	ArtAbsatz Art = iota
	ArtUeberschrift
	ArtAufzaehlung
	ArtNummer
	ArtAufgabe
	ArtCode
	ArtZitat
	ArtTabelle
	ArtDatei // Bild, Ton, Video oder Anhang, als Verweiszeile
	ArtTrenner
)

// Stueck is a run of text with one consistent appearance.
type Stueck struct {
	Text    string
	Fett    bool
	Kursiv  bool
	Fest    bool // code inside running text
	Durch   bool
	Unter   bool
	Verweis string // the address, when the run is a link
	// Farbe and Hintergrund hold the names from the editor: "yellow", "red",
	// "blue". Not a color value, because the name survives a theme change;
	// translation happens when rendering, see farben.go.
	Farbe       string
	Hintergrund string
}

// Absatz is one line or block of the document.
type Absatz struct {
	Art      Art
	Stufe    int    // heading: 1..6. Lists: nesting depth from 0.
	Nummer   int    // laufende Nummer bei nummerierten Listen
	Erledigt bool   // abgehakte Aufgabe
	Sprache  string // Codeblock
	Text     []Stueck
	Zeilen   []string   // Codeblock, zeilenweise
	Tabelle  [][]string // a table, rows of cells
	// Bild is a picture embedded in the document as a data URL. Word files
	// store their images inside the archive, so there is no external address
	// to refer to; without this the image would be lost when round-tripping
	// through the editor.
	//
	// On the way out the same field carries the image bytes so PDF and Word
	// can embed the image instead of merely naming it.
	Bild string
	// BildDaten contains the raw bytes of an image when the caller could
	// obtain them. If the field is empty we fall back to the reference line —
	// a line stating what is missing and where it lives is more honest than an
	// empty area.
	BildDaten []byte
}

// Dokument is one page in typesetter form.
type Dokument struct {
	Titel  string
	Absatz []Absatz
}

// Reading the editor document

type knoten struct {
	Type     string          `json:"type"`
	Props    map[string]any  `json:"props"`
	Content  json.RawMessage `json:"content"`
	Children []knoten        `json:"children"`
}

type stueckJSON struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Styles  map[string]any  `json:"styles"`
	Href    string          `json:"href"`
	Content json.RawMessage `json:"content"`
	Props   map[string]any  `json:"props"`
}

// Bildquelle fetches the bytes for an image address.
//
// As a callback rather than a finished list: the addresses live in the
// document and only the caller knows how to obtain the file behind them —
// via the storage backend, via a data URL in the text, or not at all. A nil
// callback means no images; then the reference line remains as before.
type Bildquelle func(adresse string) ([]byte, bool)

// AusInhaltMitBildern reads a stored document and fetches the bytes for each
// image. As everywhere in this conversion: an unknown block type becomes a
// paragraph rather than an error, because an incomplete exported document is
// more useful than one that refuses to export. A nil callback means: no
// images.
func AusInhaltMitBildern(roh json.RawMessage, titel string, hol Bildquelle) Dokument {
	d := Dokument{Titel: titel}
	var knoten []knoten
	if err := json.Unmarshal(roh, &knoten); err != nil {
		return d
	}
	d.Absatz = lies(knoten, 0, hol)
	return d
}

func lies(knoten []knoten, tiefe int, hol Bildquelle) []Absatz {
	var out []Absatz
	nummer := 0
	for _, k := range knoten {
		a := Absatz{Stufe: tiefe}
		switch k.Type {
		case "heading":
			a.Art = ArtUeberschrift
			a.Stufe = 1
			if v, ok := k.Props["level"].(float64); ok && v >= 1 && v <= 6 {
				a.Stufe = int(v)
			}
			a.Text = stuecke(k.Content)
			nummer = 0

		case "bulletListItem", "toggleListItem":
			a.Art = ArtAufzaehlung
			a.Text = stuecke(k.Content)
			nummer = 0

		case "numberedListItem":
			a.Art = ArtNummer
			nummer++
			if v, ok := k.Props["start"].(float64); ok && nummer == 1 && v >= 1 {
				nummer = int(v)
			}
			a.Nummer = nummer
			a.Text = stuecke(k.Content)

		case "checkListItem":
			a.Art = ArtAufgabe
			if v, ok := k.Props["checked"].(bool); ok {
				a.Erledigt = v
			}
			a.Text = stuecke(k.Content)
			nummer = 0

		case "codeBlock":
			a.Art = ArtCode
			a.Sprache, _ = k.Props["language"].(string)
			// Tabs become spaces: the base fonts of a PDF have no tab character, it
			// would vanish without replacement, and in code of all places the indent
			// carries meaning.
			a.Zeilen = strings.Split(strings.ReplaceAll(rohtext(k.Content), "\t", "    "), "\n")
			nummer = 0

		case "quote":
			a.Art = ArtZitat
			a.Text = stuecke(k.Content)
			nummer = 0

		case "table":
			a.Art = ArtTabelle
			a.Tabelle = tabelle(k.Content)
			nummer = 0

		case "image", "video", "audio", "file":
			a.Art = ArtDatei
			name, _ := k.Props["name"].(string)
			unterschrift, _ := k.Props["caption"].(string)
			adresse, _ := k.Props["url"].(string)
			if name == "" {
				name = unterschrift
			}
			if name == "" {
				name = k.Type
			}
			// Das Bild selbst, wenn es zu beschaffen ist. Sonst bleibt es bei
			// Name und Adresse: eine Zeile, die sagt, was fehlt und wo es liegt,
			// ist ehrlicher als eine leere Flaeche.
			if k.Type == "image" && hol != nil && adresse != "" {
				if daten, ok := hol(adresse); ok {
					a.BildDaten = daten
				}
			}
			a.Text = []Stueck{{Text: name, Verweis: adresse}}
			if unterschrift != "" && unterschrift != name {
				a.Text = append(a.Text, Stueck{Text: " – " + unterschrift, Kursiv: true})
			}
			nummer = 0

		case "divider", "horizontalRule":
			a.Art = ArtTrenner
			nummer = 0

		default:
			a.Art = ArtAbsatz
			a.Text = stuecke(k.Content)
			nummer = 0
		}

		// An empty paragraph without children is a blank line in the editor. The
		// formatters turn it into spacing; leaving it out would be wrong, because
		// then the text would close up.
		out = append(out, a)

		if len(k.Children) > 0 {
			kindTiefe := tiefe
			switch k.Type {
			case "bulletListItem", "numberedListItem", "checkListItem", "toggleListItem":
				kindTiefe = tiefe + 1
			}
			out = append(out, lies(k.Children, kindTiefe, hol)...)
		}
	}
	return out
}

// stuecke splits a block's content into runs with their appearance.
func stuecke(roh json.RawMessage) []Stueck {
	if len(roh) == 0 {
		return nil
	}
	var teile []stueckJSON
	if err := json.Unmarshal(roh, &teile); err != nil {
		// A single string, or an object with content, which is how the cells of a
		// table arrive.
		var s string
		if json.Unmarshal(roh, &s) == nil {
			return []Stueck{{Text: s}}
		}
		var huelle struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(roh, &huelle) == nil && len(huelle.Content) > 0 {
			return stuecke(huelle.Content)
		}
		return nil
	}

	var out []Stueck
	for _, t := range teile {
		switch t.Type {
		case "link":
			for _, s := range stuecke(t.Content) {
				s.Verweis = t.Href
				out = append(out, s)
			}
		case "mention":
			name, _ := t.Props["title"].(string)
			if name == "" {
				name, _ = t.Props["name"].(string)
			}
			if name == "" {
				name = t.Text
			}
			out = append(out, Stueck{Text: name, Unter: true})
		default:
			an := func(n string) bool {
				v, ok := t.Styles[n].(bool)
				return ok && v
			}
			// Farbe und Markierung stehen als Name da und nicht als Ja/Nein.
			// "default" heisst: nichts gesetzt, und das ist etwas anderes als
			// ein unbekannter Name -- beides ergibt hier aber dasselbe, naemlich
			// den gewoehnlichen Satz.
			name := func(n string) string {
				v, ok := t.Styles[n].(string)
				if !ok || v == "" || v == "default" {
					return ""
				}
				return v
			}
			out = append(out, Stueck{
				Text:        t.Text,
				Fett:        an("bold"),
				Kursiv:      an("italic"),
				Fest:        an("code"),
				Durch:       an("strike"),
				Unter:       an("underline"),
				Farbe:       name("textColor"),
				Hintergrund: name("backgroundColor"),
			})
		}
	}
	return out
}

func rohtext(roh json.RawMessage) string {
	var teile []stueckJSON
	if err := json.Unmarshal(roh, &teile); err != nil {
		var s string
		if json.Unmarshal(roh, &s) == nil {
			return s
		}
		return ""
	}
	var b strings.Builder
	for _, t := range teile {
		b.WriteString(t.Text)
	}
	return b.String()
}

func tabelle(roh json.RawMessage) [][]string {
	var inhalt struct {
		Rows []struct {
			Cells []json.RawMessage `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(roh, &inhalt); err != nil {
		return nil
	}
	breite := 0
	for _, z := range inhalt.Rows {
		if len(z.Cells) > breite {
			breite = len(z.Cells)
		}
	}
	if breite == 0 {
		return nil
	}
	out := make([][]string, 0, len(inhalt.Rows))
	for _, z := range inhalt.Rows {
		zeile := make([]string, breite)
		for i := range zeile {
			if i < len(z.Cells) {
				var b strings.Builder
				for _, s := range stuecke(z.Cells[i]) {
					b.WriteString(s.Text)
				}
				zeile[i] = strings.ReplaceAll(b.String(), "\n", " ")
			}
		}
		out = append(out, zeile)
	}
	return out
}

// NurText returns a paragraph's text without any formatting.
func NurText(st []Stueck) string {
	var b strings.Builder
	for _, s := range st {
		b.WriteString(s.Text)
	}
	return b.String()
}
