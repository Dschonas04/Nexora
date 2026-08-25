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

// LiesHTML wandelt ein HTML-Dokument in Blöcke um und liefert seinen Titel.
//
// Der Titel kommt aus der ersten Überschrift im Text, ersatzweise aus dem
// <title>. Confluence schreibt dort "Ablage : Seitenname", der Teil hinter
// dem Doppelpunkt ist der Name, den man sucht.
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
	// Steht die Überschrift auch im Text, würde sie auf der Seite doppelt
	// erscheinen: einmal als Titel, einmal als erste Zeile.
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

// kopfTitel liest <title> und schneidet den Namen der Ablage ab, den
// Confluence davorsetzt.
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

// htmlLeser sammelt Blöcke, während er durch den Baum geht.
//
// Ein offener Absatz wird mitgeführt statt sofort abgelegt: HTML kennt Text
// zwischen den Elementen, der zu keinem Absatz gehört, und den in einen eigenen
// Block zu legen zerrisse jeden Satz an jedem <a> und jedem <b>.
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

// knoten geht einen Teilbaum ab. stile ist der Satz, der von den Vorfahren
// gilt; er wird beim Absteigen kopiert und nie verändert.
func (l *htmlLeser) knoten(n *html.Node, stile map[string]bool) {
	switch n.Type {
	case html.TextNode:
		// Umbrüche und Einrückung der Quelldatei sind Formatierung des
		// Dokuments, nicht des Textes: in HTML ist jede Folge von Leerraum ein
		// Leerzeichen.
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
			// Ein div ist meist nur eine Hülle. Es schließt den Absatz, damit
			// Text davor und dahinter nicht zusammenläuft, trägt aber selbst
			// keinen Block bei.
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

// sammle liest den Inhalt eines Elements als Inline-Stücke, ohne die Blöcke des
// Aufrufers zu stören.
func (l *htmlLeser) sammle(n *html.Node, stile map[string]bool) []Inline {
	gemerkt := l.offen
	l.offen = nil
	l.kinder(n, stile)
	teile := l.offen
	l.offen = gemerkt
	return teile
}

// liste liest ul und ol. Eine Liste innerhalb eines Eintrags wird zu dessen
// Kindern, dieselbe Verschachtelung wie im Markdown-Leser.
func (l *htmlLeser) liste(n *html.Node, nummeriert bool, stile map[string]bool) {
	nummer := 0
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		if k.Type != html.ElementNode || k.Data != "li" {
			continue
		}
		nummer++

		// Der Text des Eintrags ohne die Listen darin, die kommen als
		// Kinder darunter und nicht in die Zeile.
		eintrag := Block{Type: "bulletListItem"}
		if nummeriert {
			eintrag.Type = "numberedListItem"
		}
		// Ein angehaktes Kästchen erkennt man am input davor; Confluence und
		// GitHub schreiben Aufgabenlisten so.
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
			// Was ein Eintrag außer seiner ersten Zeile noch enthält, wird zu
			// seinen Kindern: ein zweiter Absatz unter einem Punkt gehört
			// unter den Punkt und nicht daneben.
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

// tabelle liest eine HTML-Tabelle. Verbundene Zellen werden nicht aufgelöst,
// der Editor kennt sie nicht, aber alle Zeilen bekommen dieselbe Breite,
// damit keine unvollständige Tabelle entsteht.
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

// rohText liefert den Text eines Teilbaums, wie er dasteht, ohne Leerraum
// zusammenzuziehen. Für <pre>, wo genau das der Inhalt ist.
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

// leerraum zieht Leerraum zusammen, wie es ein Browser täte, behält aber je ein
// Leerzeichen am Rand: "<b>fett</b> und" braucht das Leerzeichen zwischen den
// Stücken, sonst klebt der Text zusammen.
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

// suche findet das erste Element dieses Namens im Teilbaum.
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
