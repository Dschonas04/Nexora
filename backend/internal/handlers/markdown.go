// Markdown export.
//
// The editor ships a converter of its own, but it is called
// blocksToMarkdownLossy for a reason: it drops what it cannot express, and it
// only works while the editor is loaded in a browser. This one reads the stored
// document directly, so it also serves the cases where no editor is involved,
// an export of a whole space, a scheduled backup, an API caller.
//
// The conversion is deliberately tolerant. An unknown block type becomes a
// paragraph rather than an error: a document that exports incompletely is worth
// more than one that refuses to export at all.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// block is one node of the editor document. Only the fields that matter for
// Markdown: colours and alignment have no counterpart there.
type block struct {
	Type     string          `json:"type"`
	Props    map[string]any  `json:"props"`
	Content  json.RawMessage `json:"content"`
	Children []block         `json:"children"`
}

// inline is a run of text inside a block.
type inline struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Styles  map[string]any  `json:"styles"`
	Href    string          `json:"href"`
	Content json.RawMessage `json:"content"` // for type=link the text lives here
	Props   map[string]any  `json:"props"`   // bei Erwähnungen
}

// MarkdownAusInhalt converts a stored document. Exported because the space
// export needs the same conversion.
func MarkdownAusInhalt(roh json.RawMessage) string {
	var bloecke []block
	if err := json.Unmarshal(roh, &bloecke); err != nil {
		return ""
	}
	var b strings.Builder
	schreibeBloecke(&b, bloecke, "")
	// More than one blank line in a row achieves nothing in Markdown and looks
	// like an accident in the file.
	return strings.TrimSpace(mehrfacheLeerzeilen(b.String())) + "\n"
}

// mehrfacheLeerzeilen thins out blank lines, but only outside code blocks.
//
// The difference is not a nicety: inside a code block two blank lines in a row
// are content. Silently collapsing them into one would mean altering the code
// that was exported.
func mehrfacheLeerzeilen(s string) string {
	var b strings.Builder
	imCode := false
	leer := 0
	for _, z := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(z), "```") {
			imCode = !imCode
		}
		if !imCode && strings.TrimSpace(z) == "" {
			leer++
			if leer > 1 {
				continue
			}
		} else {
			leer = 0
		}
		b.WriteString(z)
		b.WriteString("\n")
	}
	return b.String()
}

// istListe reports whether a block is a list item. It stands as its own
// function because the same enumeration is needed in three places, and one of
// them would one day forget a newly added type.
func istListe(typ string) bool {
	switch typ {
	case "bulletListItem", "numberedListItem", "checkListItem", "toggleListItem":
		return true
	}
	return false
}

// schreibeBloecke walks the list. einzug is the indentation preceding this
// level, a string rather than a depth count, because nested entries do not line
// up on a fixed step but on the width of their parent's marker: under "1. " the
// content starts in column 3, under "- " in column 2. With a fixed step of two
// characters a sub-entry of a numbered list would not be one, but a new list
// beside it.
func schreibeBloecke(b *strings.Builder, bloecke []block, einzug string) {
	nummer := 0
	// Remembers whether the last thing written was a list item. If a paragraph
	// follows without a blank line, every Markdown reader takes it as a
	// continuation of that item and swallows it into the list.
	warListe := false

	for _, bl := range bloecke {
		// Width of this item's marker; only lists pass it on to their children.
		markenBreite := 0

		if !istListe(bl.Type) && warListe {
			b.WriteString("\n")
			warListe = false
		}

		switch bl.Type {
		case "heading":
			stufe := 1
			if v, ok := bl.Props["level"].(float64); ok && v >= 1 && v <= 6 {
				stufe = int(v)
			}
			fmt.Fprintf(b, "\n%s %s\n\n", strings.Repeat("#", stufe), zeile(bl.Content, ""))
			nummer = 0

		case "bulletListItem", "toggleListItem":
			// A toggle list reads as a bullet list; Markdown has no notion of
			// collapsing. The content matters more than the interaction, so it
			// becomes a bullet rather than disappearing.
			marke := "- "
			markenBreite = len(marke)
			fmt.Fprintf(b, "%s%s%s\n", einzug, marke, zeile(bl.Content, einzug+"  "))
			nummer = 0
			warListe = true

		case "numberedListItem":
			nummer++
			// A list that starts at 5 should still start at 5.
			if v, ok := bl.Props["start"].(float64); ok && nummer == 1 && v >= 1 {
				nummer = int(v)
			}
			marke := fmt.Sprintf("%d. ", nummer)
			markenBreite = len(marke)
			fmt.Fprintf(b, "%s%s%s\n", einzug, marke, zeile(bl.Content, einzug+strings.Repeat(" ", markenBreite)))
			warListe = true

		case "checkListItem":
			haken := " "
			if v, ok := bl.Props["checked"].(bool); ok && v {
				haken = "x"
			}
			marke := "- [" + haken + "] "
			// The continuation column sits after "- ", not after the checkbox
			// marker: the checkbox is already part of the content.
			markenBreite = 2
			fmt.Fprintf(b, "%s%s%s\n", einzug, marke, zeile(bl.Content, einzug+"  "))
			nummer = 0
			warListe = true

		case "codeBlock":
			sprache, _ := bl.Props["language"].(string)
			fmt.Fprintf(b, "\n%s```%s\n", einzug, sprache)
			for _, z := range strings.Split(rohText(bl.Content), "\n") {
				fmt.Fprintf(b, "%s%s\n", einzug, z)
			}
			fmt.Fprintf(b, "%s```\n\n", einzug)
			nummer = 0

		case "quote":
			// Multi-line quotes need the > in front of every line, otherwise the
			// quote ends after the first one.
			for _, z := range strings.Split(zeile(bl.Content, ""), "\n") {
				fmt.Fprintf(b, "%s> %s\n", einzug, strings.TrimRight(z, " "))
			}
			b.WriteString("\n")
			nummer = 0

		case "image", "video", "audio", "file":
			adresse, _ := bl.Props["url"].(string)
			name, _ := bl.Props["name"].(string)
			bildunterschrift, _ := bl.Props["caption"].(string)
			if name == "" {
				name = bildunterschrift
			}
			if name == "" {
				name = bl.Type
			}
			if bl.Type == "image" {
				fmt.Fprintf(b, "\n%s![%s](%s)\n", einzug, klammersicher(name), adressSicher(adresse))
			} else {
				// Markdown has no embedding for video, audio and files, so a
				// link is the most honest form.
				fmt.Fprintf(b, "\n%s[%s](%s)\n", einzug, klammersicher(name), adressSicher(adresse))
			}
			if bildunterschrift != "" && bildunterschrift != name {
				fmt.Fprintf(b, "\n%s*%s*\n", einzug, entschaerfen(bildunterschrift))
			}
			b.WriteString("\n")
			nummer = 0

		case "table":
			schreibeTabelle(b, bl, einzug)
			nummer = 0

		case "paragraph":
			t := zeile(bl.Content, einzug)
			if strings.TrimSpace(t) == "" {
				// An empty paragraph is a blank line in the editor. It should be
				// exactly that in Markdown too, not three.
				b.WriteString("\n")
			} else {
				fmt.Fprintf(b, "%s%s\n\n", einzug, t)
			}
			nummer = 0

		default:
			// Unknown type: save the text rather than lose the block.
			if t := zeile(bl.Content, einzug); strings.TrimSpace(t) != "" {
				fmt.Fprintf(b, "%s%s\n\n", einzug, t)
			}
			nummer = 0
		}

		if len(bl.Children) > 0 {
			// Nested lists indent by the width of their own marker, everything
			// else stays on its level: an indented heading would be a code block
			// in Markdown.
			kindEinzug := einzug
			if markenBreite > 0 {
				kindEinzug = einzug + strings.Repeat(" ", markenBreite)
			}
			schreibeBloecke(b, bl.Children, kindEinzug)
			// After the children the parent's note no longer holds: the last
			// thing written was on the level below.
			warListe = istListe(bl.Type)
		}
	}
}

func schreibeTabelle(b *strings.Builder, bl block, einzug string) {
	// BlockNote keeps the table under content.rows, not as a list.
	var inhalt struct {
		Rows []struct {
			Cells []json.RawMessage `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(bl.Content, &inhalt); err != nil || len(inhalt.Rows) == 0 {
		return
	}
	// Bring every row to the same number of columns. A table with merged or
	// missing cells has rows of differing length, and a row with more separators
	// than the header tears the whole table apart in many readers.
	breite := 0
	for _, z := range inhalt.Rows {
		if len(z.Cells) > breite {
			breite = len(z.Cells)
		}
	}
	if breite == 0 {
		return
	}
	b.WriteString("\n")
	for i, zeileD := range inhalt.Rows {
		zellen := make([]string, breite)
		for j := range zellen {
			if j < len(zeileD.Cells) {
				// A line break inside a cell would tear the table apart, and so
				// would a vertical bar, which is why zellSicher escapes it.
				zellen[j] = zellSicher(zeile(zeileD.Cells[j], ""))
			}
		}
		fmt.Fprintf(b, "%s| %s |\n", einzug, strings.Join(zellen, " | "))
		if i == 0 {
			trenner := make([]string, breite)
			for j := range trenner {
				trenner[j] = "---"
			}
			fmt.Fprintf(b, "%s| %s |\n", einzug, strings.Join(trenner, " | "))
		}
	}
	b.WriteString("\n")
}

// zellSicher macht einen Zellentext tabellenfest.
func zellSicher(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

// zeile turns the content of a block into one line and deals with hard breaks.
//
// A break inside a paragraph is a break in the editor, not a new paragraph.
// Taken over raw it would be nothing at all in Markdown: the text would run
// together, or, inside a list item, end the list. Two spaces before the break
// turn it into a hard break, and the following line receives the indentation of
// the continuation column.
func zeile(roh json.RawMessage, fortsetzung string) string {
	t := text(roh)
	if !strings.Contains(t, "\n") {
		return t
	}
	teile := strings.Split(t, "\n")
	for i := range teile {
		if i > 0 {
			teile[i] = fortsetzung + teile[i]
		}
	}
	return strings.Join(teile, "  \n")
}

// text turns a block's content into Markdown, formatting included.
func text(roh json.RawMessage) string {
	if len(roh) == 0 {
		return ""
	}
	var teile []inline
	if err := json.Unmarshal(roh, &teile); err != nil {
		// Two other shapes occur. First a plain string. Second an object with a
		// content field, which is how the editor has been returning table cells
		// for a while. Without that second case every cell would stay empty: the
		// table would be there, but without content.
		var s string
		if json.Unmarshal(roh, &s) == nil {
			return s
		}
		var huelle struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(roh, &huelle) == nil && len(huelle.Content) > 0 {
			return text(huelle.Content)
		}
		return ""
	}

	var b strings.Builder
	for _, t := range teile {
		switch t.Type {
		case "link":
			b.WriteString("[" + klammersicher(text(t.Content)) + "](" + adressSicher(t.Href) + ")")
		case "mention":
			// Mentions show a name in the editor with a page behind it. As a
			// bare name the reference would be gone.
			name, _ := t.Props["title"].(string)
			if name == "" {
				name, _ = t.Props["name"].(string)
			}
			if name == "" {
				name = t.Text
			}
			b.WriteString("[[" + name + "]]")
		default:
			b.WriteString(auszeichnen(t.Text, t.Styles))
		}
	}
	return b.String()
}

// entschaerfen escapes characters Markdown reads as instructions even though
// they are only characters in the text.
//
// Without it the output is not what stood on the page: an asterisk becomes
// formatting, "2026. A good year" becomes a numbered list, "[[Note]]" becomes a
// link nobody made. That is exactly the kind of drift that shows when the file
// is read back in.
//
// The escaping is sparing. An underscore inside a word is left alone, because
// CommonMark does not read it as formatting there anyway; escaping it would
// litter every file name and identifier with backslashes without changing the
// result.
func entschaerfen(s string) string {
	if s == "" {
		return s
	}
	// [[Page title]] is a link in Nexora even when it sits in the document as
	// ordinary text; the editor recognises it by the pattern. If the brackets
	// were escaped, the export would hand back text that once was a link and is
	// one no longer. So those spots stay untouched and only what lies between
	// them is escaped.
	if stellen := wikiMuster.FindAllStringIndex(s, -1); len(stellen) > 0 {
		var b strings.Builder
		vorher := 0
		for _, st := range stellen {
			b.WriteString(entschaerfenRoh(s[vorher:st[0]]))
			b.WriteString(s[st[0]:st[1]])
			vorher = st[1]
		}
		b.WriteString(entschaerfenRoh(s[vorher:]))
		return b.String()
	}
	return entschaerfenRoh(s)
}

// wikiMuster is the same pattern the interface uses for [[links]].
var wikiMuster = regexp.MustCompile(`\[\[[^\[\]]+\]\]`)

func entschaerfenRoh(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	laeufer := []rune(s)
	for i, r := range laeufer {
		switch r {
		case '\\', '*', '`', '[', ']':
			b.WriteRune('\\')
		case '_':
			// Only at word boundaries. A letter or digit before or after means
			// mid-word, and there the underscore has no effect.
			vorher := i == 0 || !wortzeichen(laeufer[i-1])
			nachher := i == len(laeufer)-1 || !wortzeichen(laeufer[i+1])
			if vorher || nachher {
				b.WriteRune('\\')
			}
		}
		b.WriteRune(r)
	}
	return zeilenanfaengeEntschaerfen(b.String())
}

func wortzeichen(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r > 127
}

// zeilenanfaengeEntschaerfen deals with the characters that only mean something
// at the start of a line: hash, dash, greater-than, digit with a dot.
// Mid-sentence they are harmless and stay untouched.
func zeilenanfaengeEntschaerfen(s string) string {
	zeilen := strings.Split(s, "\n")
	for i, z := range zeilen {
		rest := strings.TrimLeft(z, " \t")
		vorne := z[:len(z)-len(rest)]
		switch {
		case strings.HasPrefix(rest, "#"),
			strings.HasPrefix(rest, ">"),
			strings.HasPrefix(rest, "|"):
			zeilen[i] = vorne + "\\" + rest
		case strings.HasPrefix(rest, "- "), strings.HasPrefix(rest, "+ "), rest == "-", rest == "+":
			zeilen[i] = vorne + "\\" + rest
		default:
			// "1. " or "12) " at the start makes a numbered list.
			ziffern := 0
			for ziffern < len(rest) && rest[ziffern] >= '0' && rest[ziffern] <= '9' {
				ziffern++
			}
			if ziffern > 0 && ziffern < len(rest) && (rest[ziffern] == '.' || rest[ziffern] == ')') {
				zeilen[i] = vorne + rest[:ziffern] + "\\" + rest[ziffern:]
			}
		}
	}
	return strings.Join(zeilen, "\n")
}

// klammersicher protects the visible part of a link. A closing bracket inside
// it would otherwise end the label in the wrong place.
func klammersicher(s string) string {
	s = strings.ReplaceAll(s, "[", "\\[")
	return strings.ReplaceAll(s, "]", "\\]")
}

// adressSicher makes an address usable inside a link. Round brackets and spaces
// end a Markdown address; angle brackets around it are the way out the standard
// provides.
func adressSicher(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsAny(s, " ()<>") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E", " ", "%20").Replace(s) + ">"
	}
	return s
}

// auszeichnen wraps the Markdown characters around a run of text.
//
// The order is not arbitrary: code innermost, because inside backticks an
// asterisk no longer has any effect. Anything else would be broken Markdown.
func auszeichnen(s string, styles map[string]any) string {
	if s == "" {
		return ""
	}
	an := func(name string) bool {
		v, ok := styles[name].(bool)
		return ok && v
	}
	// Leading and trailing spaces have to stay OUTSIDE the formatting: no reader
	// renders "** bold **" as bold.
	links := s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	rechts := s[len(strings.TrimRight(s, " \t")):]
	kern := strings.TrimSpace(s)
	if kern == "" {
		return s
	}

	if an("code") {
		// Inside code the rule above does NOT apply: spaces at the start are not
		// padding there but indentation, and indentation is content in code.
		// Outside the backticks everyone reading the file back in would lose it.
		kern = s
		links, rechts = "", ""

		// Inside code every character stays as it is, so nothing is escaped
		// here. Instead the fence is made longer than any run of backticks in the
		// text: there is no other way to get a backtick into a code span.
		zaun := "`"
		for strings.Contains(kern, zaun) {
			zaun += "`"
		}
		// A padding space at both ends is needed in two cases: when the content
		// itself begins or ends with a backtick, and when it has a space at both
		// ends, because then a reader strips one from each by the rule above, and
		// without padding that would be the content.
		fuellung := ""
		if strings.HasPrefix(kern, "`") || strings.HasSuffix(kern, "`") ||
			(strings.HasPrefix(kern, " ") && strings.HasSuffix(kern, " ")) {
			fuellung = " "
		}
		kern = zaun + fuellung + kern + fuellung + zaun
	} else {
		// Everything that is not code gets escaped: what was an asterisk in the
		// editor should stay an asterisk in the file and not become formatting.
		kern = entschaerfen(kern)
	}
	if an("bold") {
		kern = "**" + kern + "**"
	}
	if an("italic") {
		kern = "*" + kern + "*"
	}
	if an("strike") {
		kern = "~~" + kern + "~~"
	}
	if an("underline") {
		// Markdown has no underline. HTML is more honest here than dropping the
		// formatting silently.
		kern = "<u>" + kern + "</u>"
	}
	return links + kern + rechts
}

// rohText returns the text without formatting, for code blocks where asterisks
// have to stay asterisks.
func rohText(roh json.RawMessage) string {
	var teile []inline
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

// ExportMarkdown returns a page as a Markdown file.
func (s *Server) ExportMarkdown(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	canRead, _, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || !canRead {
		writeErr(w, http.StatusForbidden, "kein Zugriff auf diese Seite")
		return
	}

	var titel string
	var inhalt []byte
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT title, content FROM pages WHERE id=$1`, id).Scan(&titel, &inhalt); err != nil {
		writeErr(w, http.StatusNotFound, "Seite nicht gefunden")
		return
	}

	md := MarkdownAusInhalt(json.RawMessage(inhalt))
	// The title is not part of the document but lives in a column of its own.
	// Without this line the file would have no heading.
	//
	// Many pages repeat their title as the first heading in the text, though.
	// Then it would stand there twice, which looks like a mistake in the file, so
	// it is only prepended when it is not already there.
	if titel != "" && !beginntMitUeberschrift(md, titel) {
		md = "# " + titel + "\n\n" + md
	}

	name := dateiname(titel)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	// Two forms on purpose: filename for old clients that only understand ASCII,
	// filename* per RFC 5987 for everyone else. That way "Übersicht" stays an
	// Übersicht instead of becoming "Uebersicht"; transliterating umlauts was a
	// workaround from a time before filename* existed.
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+nurASCII(name)+`.md"; filename*=UTF-8''`+
			url.PathEscape(name+".md"))
	w.Write([]byte(md))
}

// beginntMitUeberschrift reports whether the document already starts with a
// heading carrying this title. The comparison ignores case: "Wake-on-LAN" and
// "Wake-On-LAN" mean the same thing.
func beginntMitUeberschrift(md, titel string) bool {
	erste := strings.TrimSpace(md)
	if i := strings.Index(erste, "\n"); i >= 0 {
		erste = erste[:i]
	}
	erste = strings.TrimSpace(strings.TrimLeft(erste, "#"))
	return strings.EqualFold(erste, strings.TrimSpace(titel))
}

// dateiname turns a title into a file name. Umlauts are kept; only what file
// systems or the HTTP header cannot bear is removed.
func dateiname(titel string) string {
	if strings.TrimSpace(titel) == "" {
		return "seite"
	}
	var b strings.Builder
	for _, r := range titel {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			b.WriteRune('-')
		default:
			if r < 32 {
				b.WriteRune('-')
			} else {
				b.WriteRune(r)
			}
		}
	}
	name := strings.Trim(strings.TrimSpace(b.String()), "-. ")
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	if name == "" {
		return "seite"
	}
	// File systems draw the line at 255 bytes; with the .md appended a little
	// room has to be left.
	if len(name) > 200 {
		name = strings.ToValidUTF8(name[:200], "")
	}
	return name
}

// nurASCII is the fallback spelling for the filename part of the header.
// Non-ASCII is replaced with _ rather than transliterated, since the correct
// spelling is in filename* anyway.
func nurASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
