// Reading HTML back in.
//
// The reason this exists is Confluence. Its export is a folder of HTML files,
// and it is the single most common thing somebody is trying to leave when they
// look at a wiki like this one. Notion and most other tools export Markdown;
// Confluence does not, and telling people to convert a thousand pages by hand
// first is the same as telling them no.
//
// The same subset as the Markdown reader, for the same reason: what the editor
// can hold. Everything else, attributes, classes, styles, the div soup a page
// builder leaves behind, is walked through and dropped. That is deliberate:
// an import that carried the source formatting along would produce pages nobody
// can edit afterwards.
package einlesen

import (
	"strings"

	"golang.org/x/net/html"
)

// LiesHTML turns an HTML document into blocks and returns its title.
//
// The title comes from the first heading in the text, failing that from
// <title>. Confluence writes "Space : Page name" there; the part behind the
// colon is the name one is looking for.
func LiesHTML(quelle string) (string, []Block) {
	wurzel, err := html.Parse(strings.NewReader(quelle))
	if err != nil {
		return "", []Block{{Type: "paragraph"}}
	}

	l := &htmlLeser{}
	l.knoten(wurzel, nil)
	l.absatzSchliessen()

	titel := l.titel
	if titel == "" {
		titel = kopfTitel(wurzel)
	}
	// If the heading also stands in the text it would appear twice on the page:
	// once as the title, once as the first line.
	if len(l.bloecke) > 0 && l.bloecke[0].Type == "heading" && NurText(inhaltVon(l.bloecke[0])) == titel {
		l.bloecke = l.bloecke[1:]
	}
	if len(l.bloecke) == 0 {
		l.bloecke = []Block{{Type: "paragraph"}}
	}
	return titel, l.bloecke
}

func inhaltVon(b Block) []Inline {
	if teile, ok := b.Content.([]Inline); ok {
		return teile
	}
	return nil
}

// kopfTitel reads <title> and cuts off the name of the space that Confluence
// puts in front of it.
func kopfTitel(wurzel *html.Node) string {
	var gefunden string
	var gehen func(*html.Node)
	gehen = func(n *html.Node) {
		if gefunden != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			gefunden = strings.TrimSpace(n.FirstChild.Data)
			return
		}
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			gehen(k)
		}
	}
	gehen(wurzel)
	if i := strings.LastIndex(gefunden, " : "); i >= 0 {
		gefunden = strings.TrimSpace(gefunden[i+3:])
	}
	return gefunden
}

// htmlLeser collects blocks while walking the tree.
//
// An open paragraph is carried along instead of being filed right away: HTML
// has text between elements that belongs to no paragraph, and putting that into
// a block of its own would tear every sentence apart at every <a> and every <b>.
type htmlLeser struct {
	bloecke []Block
	offen   []Inline
	titel   string
}

func (l *htmlLeser) absatzSchliessen() {
	if len(l.offen) == 0 {
		return
	}
	if NurText(l.offen) == "" {
		l.offen = nil
		return
	}
	l.bloecke = append(l.bloecke, Block{Type: "paragraph", Content: l.offen})
	l.offen = nil
}

func (l *htmlLeser) block(b Block) {
	l.absatzSchliessen()
	l.bloecke = append(l.bloecke, b)
}

// knoten walks a subtree. stile is the set of styles inherited from the
// ancestors; it is copied while descending and never modified.
func (l *htmlLeser) knoten(n *html.Node, stile map[string]bool) {
	switch n.Type {
	case html.TextNode:
		// Line breaks and indentation in the source file are formatting of the
		// document, not of the text: in HTML any run of whitespace is one space.
		text := leerraum(n.Data)
		if text == "" {
			return
		}
		l.offen = append(l.offen, Inline{Type: "text", Text: text, Styles: kopie(stile)})
		return

	case html.ElementNode:
		switch n.Data {
		case "script", "style", "head", "noscript", "svg", "nav", "footer":
			return

		case "h1", "h2", "h3", "h4", "h5", "h6":
			l.absatzSchliessen()
			teile := l.sammle(n, stile)
			stufe := int(n.Data[1] - '0')
			if stufe > 3 {
				stufe = 3
			}
			if l.titel == "" && stufe == 1 {
				l.titel = NurText(teile)
			}
			l.block(Block{Type: "heading", Props: map[string]any{"level": stufe}, Content: teile})
			return

		case "p", "div", "section", "article", "header", "dd", "dt":
			// A div is usually just a wrapper. It closes the paragraph so that text
			// before and after it does not run together, but contributes no block of
			// its own.
			l.absatzSchliessen()
			l.kinder(n, stile)
			l.absatzSchliessen()
			return

		case "br":
			l.offen = append(l.offen, Inline{Type: "text", Text: "\n", Styles: kopie(stile)})
			return

		case "hr":
			l.block(Block{Type: "paragraph", Content: []Inline{{Type: "text", Text: "———"}}})
			return

		case "ul", "ol":
			l.absatzSchliessen()
			l.liste(n, n.Data == "ol", stile)
			return

		case "blockquote":
			l.absatzSchliessen()
			innen := &htmlLeser{}
			innen.kinder(n, stile)
			innen.absatzSchliessen()
			for _, b := range innen.bloecke {
				l.block(kursivMachen(b))
			}
			return

		case "pre":
			l.absatzSchliessen()
			for _, z := range strings.Split(strings.Trim(rohText(n), "\n"), "\n") {
				if strings.TrimSpace(z) == "" {
					l.bloecke = append(l.bloecke, Block{Type: "paragraph"})
					continue
				}
				l.bloecke = append(l.bloecke, Block{
					Type:    "paragraph",
					Content: []Inline{{Type: "text", Text: z, Styles: map[string]bool{"code": true}}},
				})
			}
			return

		case "table":
			l.absatzSchliessen()
			if b, ok := l.tabelle(n, stile); ok {
				l.bloecke = append(l.bloecke, b)
			}
			return

		case "img":
			adresse := attribut(n, "src")
			if adresse == "" {
				return
			}
			alt := attribut(n, "alt")
			if alt == "" {
				alt = attribut(n, "title")
			}
			l.block(bildBlock(alt, adresse))
			return

		case "a":
			ziel := attribut(n, "href")
			teile := l.sammle(n, stile)
			if ziel == "" || len(teile) == 0 {
				l.offen = append(l.offen, teile...)
				return
			}
			l.offen = append(l.offen, Inline{Type: "link", Href: ziel, Content: teile})
			return

		case "b", "strong":
			l.kinder(n, mitStil(stile, "bold"))
			return
		case "i", "em", "cite":
			l.kinder(n, mitStil(stile, "italic"))
			return
		case "u", "ins":
			l.kinder(n, mitStil(stile, "underline"))
			return
		case "s", "del", "strike":
			l.kinder(n, mitStil(stile, "strike"))
			return
		case "code", "tt", "kbd", "samp":
			l.kinder(n, mitStil(stile, "code"))
			return
		}
	}

	l.kinder(n, stile)
}

func (l *htmlLeser) kinder(n *html.Node, stile map[string]bool) {
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		l.knoten(k, stile)
	}
}

// sammle reads the content of an element as inline pieces without disturbing
// the caller's blocks.
func (l *htmlLeser) sammle(n *html.Node, stile map[string]bool) []Inline {
	gemerkt := l.offen
	l.offen = nil
	l.kinder(n, stile)
	teile := l.offen
	l.offen = gemerkt
	return teile
}

// liste reads ul and ol. A list inside an item becomes that item's children,
// the same nesting as in the Markdown reader.
func (l *htmlLeser) liste(n *html.Node, nummeriert bool, stile map[string]bool) {
	nummer := 0
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		if k.Type != html.ElementNode || k.Data != "li" {
			continue
		}
		nummer++

		// The text of the item without the lists inside it; those come below as
		// children and not into the line.
		eintrag := Block{Type: "bulletListItem"}
		if nummeriert {
			eintrag.Type = "numberedListItem"
		}
		// A ticked box is recognised by the input in front of it; Confluence and
		// GitHub write task lists that way.
		if kasten := suche(k, "input"); kasten != nil && attribut(kasten, "type") == "checkbox" {
			eintrag.Type = "checkListItem"
			eintrag.Props = map[string]any{"checked": hatAttribut(kasten, "checked")}
		}

		innen := &htmlLeser{}
		var unterlisten []*html.Node
		for kk := k.FirstChild; kk != nil; kk = kk.NextSibling {
			if kk.Type == html.ElementNode && (kk.Data == "ul" || kk.Data == "ol") {
				unterlisten = append(unterlisten, kk)
				continue
			}
			innen.knoten(kk, stile)
		}
		innen.absatzSchliessen()

		if len(innen.bloecke) > 0 {
			eintrag.Content = inhaltVon(innen.bloecke[0])
			// Whatever an item contains besides its first line becomes its
			// children: a second paragraph under a bullet belongs below the bullet
			// and not beside it.
			for _, b := range innen.bloecke[1:] {
				eintrag.Children = append(eintrag.Children, b)
			}
		}
		for _, ul := range unterlisten {
			unten := &htmlLeser{}
			unten.liste(ul, ul.Data == "ol", stile)
			eintrag.Children = append(eintrag.Children, unten.bloecke...)
		}
		l.bloecke = append(l.bloecke, eintrag)
	}
}

// tabelle reads an HTML table. Merged cells are not resolved, the editor does
// not know them, but every row gets the same width so that no incomplete table
// results.
func (l *htmlLeser) tabelle(n *html.Node, stile map[string]bool) (Block, bool) {
	var zeilen [][][]Inline
	var lesen func(*html.Node)
	lesen = func(k *html.Node) {
		for c := k.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "thead", "tbody", "tfoot":
				lesen(c)
			case "tr":
				var zelle [][]Inline
				for z := c.FirstChild; z != nil; z = z.NextSibling {
					if z.Type == html.ElementNode && (z.Data == "td" || z.Data == "th") {
						innen := stile
						if z.Data == "th" {
							innen = mitStil(stile, "bold")
						}
						zelle = append(zelle, l.sammle(z, innen))
					}
				}
				if len(zelle) > 0 {
					zeilen = append(zeilen, zelle)
				}
			}
		}
	}
	lesen(n)
	if len(zeilen) == 0 {
		return Block{}, false
	}
	breite := 0
	for _, z := range zeilen {
		if len(z) > breite {
			breite = len(z)
		}
	}
	inhalt := TabellenInhalt{Type: "tableContent"}
	for _, z := range zeilen {
		zeile := TabellenZeile{Cells: make([][]Inline, breite)}
		for i := range zeile.Cells {
			if i < len(z) {
				zeile.Cells[i] = z[i]
			} else {
				zeile.Cells[i] = []Inline{}
			}
		}
		inhalt.Rows = append(inhalt.Rows, zeile)
	}
	return Block{Type: "table", Content: inhalt}, true
}

// rohText returns the text of a subtree exactly as it stands, without
// collapsing whitespace. For <pre>, where that is precisely the content.
func rohText(n *html.Node) string {
	var b strings.Builder
	var gehen func(*html.Node)
	gehen = func(k *html.Node) {
		if k.Type == html.TextNode {
			b.WriteString(k.Data)
			return
		}
		if k.Type == html.ElementNode && k.Data == "br" {
			b.WriteString("\n")
		}
		for c := k.FirstChild; c != nil; c = c.NextSibling {
			gehen(c)
		}
	}
	gehen(n)
	return b.String()
}

// leerraum collapses whitespace the way a browser would, but keeps one space at
// each edge: "<b>bold</b> and" needs the space between the pieces, otherwise the
// text sticks together.
func leerraum(s string) string {
	if strings.TrimSpace(s) == "" {
		if s == "" {
			return ""
		}
		return " "
	}
	links := s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r'
	rechts := s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r'
	kern := strings.Join(strings.Fields(s), " ")
	if links {
		kern = " " + kern
	}
	if rechts {
		kern += " "
	}
	return kern
}

func attribut(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func hatAttribut(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}

// suche finds the first element of that name in the subtree.
func suche(n *html.Node, name string) *html.Node {
	if n.Type == html.ElementNode && n.Data == name {
		return n
	}
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		if t := suche(k, name); t != nil {
			return t
		}
	}
	return nil
}
