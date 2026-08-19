// Markdown export.
//
// The editor ships a converter of its own, but it is called
// blocksToMarkdownLossy for a reason: it drops what it cannot express, and it
// only works while the editor is loaded in a browser. This one reads the stored
// document directly, so it also serves the cases where no editor is involved --
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
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// block ist ein Knoten des Editor-Dokuments. Nur die Felder, die für Markdown
// zählen -- Farben und Ausrichtung haben in Markdown kein Gegenstück.
type block struct {
	Type     string          `json:"type"`
	Props    map[string]any  `json:"props"`
	Content  json.RawMessage `json:"content"`
	Children []block         `json:"children"`
}

// inline ist ein Textstück innerhalb eines Blocks.
type inline struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Styles  map[string]any  `json:"styles"`
	Href    string          `json:"href"`
	Content json.RawMessage `json:"content"` // bei type=link liegt der Text hier
	Props   map[string]any  `json:"props"`   // bei Erwähnungen
}

// MarkdownAusInhalt wandelt ein gespeichertes Dokument um. Exportiert, weil der
// Space-Export dieselbe Umwandlung braucht.
func MarkdownAusInhalt(roh json.RawMessage) string {
	var bloecke []block
	if err := json.Unmarshal(roh, &bloecke); err != nil {
		return ""
	}
	var b strings.Builder
	schreibeBloecke(&b, bloecke, 0, nil)
	// Mehr als eine Leerzeile hintereinander bringt in Markdown nichts und
	// sieht in der Datei nach Versehen aus.
	return strings.TrimSpace(mehrfacheLeerzeilen(b.String())) + "\n"
}

func mehrfacheLeerzeilen(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// schreibeBloecke geht die Liste durch. tiefe steuert die Einrückung
// verschachtelter Listen, nummer zählt bei nummerierten Listen mit.
func schreibeBloecke(b *strings.Builder, bloecke []block, tiefe int, _ *int) {
	nummer := 0
	for _, bl := range bloecke {
		einzug := strings.Repeat("  ", tiefe)

		switch bl.Type {
		case "heading":
			stufe := 1
			if v, ok := bl.Props["level"].(float64); ok && v >= 1 && v <= 6 {
				stufe = int(v)
			}
			fmt.Fprintf(b, "\n%s %s\n\n", strings.Repeat("#", stufe), text(bl.Content))
			nummer = 0

		case "bulletListItem":
			fmt.Fprintf(b, "%s- %s\n", einzug, text(bl.Content))
			nummer = 0

		case "numberedListItem":
			nummer++
			fmt.Fprintf(b, "%s%d. %s\n", einzug, nummer, text(bl.Content))

		case "checkListItem":
			haken := " "
			if v, ok := bl.Props["checked"].(bool); ok && v {
				haken = "x"
			}
			fmt.Fprintf(b, "%s- [%s] %s\n", einzug, haken, text(bl.Content))
			nummer = 0

		case "codeBlock":
			sprache, _ := bl.Props["language"].(string)
			fmt.Fprintf(b, "\n```%s\n%s\n```\n\n", sprache, rohText(bl.Content))
			nummer = 0

		case "quote":
			// Mehrzeilige Zitate brauchen das > vor jeder Zeile, sonst bricht
			// das Zitat nach der ersten ab.
			for _, z := range strings.Split(text(bl.Content), "\n") {
				fmt.Fprintf(b, "%s> %s\n", einzug, z)
			}
			b.WriteString("\n")
			nummer = 0

		case "image", "video", "audio", "file":
			url, _ := bl.Props["url"].(string)
			name, _ := bl.Props["name"].(string)
			bildunterschrift, _ := bl.Props["caption"].(string)
			if name == "" {
				name = bildunterschrift
			}
			if name == "" {
				name = bl.Type
			}
			if bl.Type == "image" {
				fmt.Fprintf(b, "\n![%s](%s)\n", name, url)
			} else {
				// Für Video, Ton und Dateien gibt es in Markdown kein
				// Einbetten -- ein Verweis ist das Ehrlichste.
				fmt.Fprintf(b, "\n[%s](%s)\n", name, url)
			}
			if bildunterschrift != "" && bildunterschrift != name {
				fmt.Fprintf(b, "\n*%s*\n", bildunterschrift)
			}
			b.WriteString("\n")
			nummer = 0

		case "table":
			schreibeTabelle(b, bl)
			nummer = 0

		case "paragraph":
			t := text(bl.Content)
			if strings.TrimSpace(t) == "" {
				// Ein leerer Absatz ist im Editor eine Leerzeile. Genau das
				// soll er in Markdown auch sein, nicht drei.
				b.WriteString("\n")
			} else {
				fmt.Fprintf(b, "%s%s\n\n", einzug, t)
			}
			nummer = 0

		default:
			// Unbekannter Typ: den Text retten statt den Block zu verlieren.
			if t := text(bl.Content); strings.TrimSpace(t) != "" {
				fmt.Fprintf(b, "%s%s\n\n", einzug, t)
			}
			nummer = 0
		}

		if len(bl.Children) > 0 {
			// Verschachtelte Listen rücken ein, alles andere bleibt auf der
			// Ebene -- eine eingerückte Überschrift wäre in Markdown ein
			// Codeblock.
			kindTiefe := tiefe
			switch bl.Type {
			case "bulletListItem", "numberedListItem", "checkListItem":
				kindTiefe = tiefe + 1
			}
			schreibeBloecke(b, bl.Children, kindTiefe, nil)
		}
	}
}

func schreibeTabelle(b *strings.Builder, bl block) {
	// BlockNote legt die Tabelle unter content.rows ab, nicht als Liste.
	var inhalt struct {
		Rows []struct {
			Cells []json.RawMessage `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(bl.Content, &inhalt); err != nil || len(inhalt.Rows) == 0 {
		return
	}
	b.WriteString("\n")
	for i, zeile := range inhalt.Rows {
		zellen := make([]string, 0, len(zeile.Cells))
		for _, c := range zeile.Cells {
			// Ein Zeilenumbruch in einer Zelle würde die Tabelle zerreißen.
			zellen = append(zellen, strings.ReplaceAll(text(c), "\n", " "))
		}
		fmt.Fprintf(b, "| %s |\n", strings.Join(zellen, " | "))
		if i == 0 {
			trenner := make([]string, len(zellen))
			for j := range trenner {
				trenner[j] = "---"
			}
			fmt.Fprintf(b, "| %s |\n", strings.Join(trenner, " | "))
		}
	}
	b.WriteString("\n")
}

// text wandelt den Inhalt eines Blocks in Markdown um, mit Auszeichnungen.
func text(roh json.RawMessage) string {
	if len(roh) == 0 {
		return ""
	}
	var teile []inline
	if err := json.Unmarshal(roh, &teile); err != nil {
		// Manche Blöcke tragen statt einer Liste eine einzelne Zeichenkette.
		var s string
		if json.Unmarshal(roh, &s) == nil {
			return s
		}
		return ""
	}

	var b strings.Builder
	for _, t := range teile {
		switch t.Type {
		case "link":
			b.WriteString("[" + text(t.Content) + "](" + t.Href + ")")
		case "mention":
			// Erwähnungen zeigen im Editor einen Namen, dahinter steckt eine
			// Seite. Als reiner Name wäre der Bezug weg.
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

// auszeichnen legt die Markdown-Zeichen um ein Textstück.
//
// Die Reihenfolge ist nicht beliebig: Code ganz innen, weil innerhalb von
// Backticks kein Stern mehr wirkt. Alles andere wäre kaputtes Markdown.
func auszeichnen(s string, styles map[string]any) string {
	if s == "" {
		return ""
	}
	an := func(name string) bool {
		v, ok := styles[name].(bool)
		return ok && v
	}
	// Führende und folgende Leerzeichen müssen AUSSERHALB der Auszeichnung
	// bleiben -- "** fett **" wird von keinem Leser als fett dargestellt.
	links := s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	rechts := s[len(strings.TrimRight(s, " \t")):]
	kern := strings.TrimSpace(s)
	if kern == "" {
		return s
	}

	if an("code") {
		kern = "`" + kern + "`"
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
		// Markdown kennt keine Unterstreichung. HTML ist hier ehrlicher als
		// sie stillschweigend fallen zu lassen.
		kern = "<u>" + kern + "</u>"
	}
	return links + kern + rechts
}

// rohText liefert den Text ohne Auszeichnung -- für Codeblöcke, in denen
// Sternchen Sternchen bleiben müssen.
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

// ExportMarkdown liefert eine Seite als Markdown-Datei.
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
	// Der Titel steht nicht im Dokument, sondern in einer eigenen Spalte. Ohne
	// diese Zeile hätte die Datei keine Überschrift.
	//
	// Viele Seiten wiederholen ihren Titel aber als erste Überschrift im Text.
	// Dann bliebe er zweimal stehen, was in der Datei nach einem Fehler
	// aussieht -- also nur voranstellen, wenn er nicht ohnehin schon dasteht.
	if titel != "" && !beginntMitUeberschrift(md, titel) {
		md = "# " + titel + "\n\n" + md
	}

	name := dateiname(titel)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	// Zwei Angaben mit Absicht: filename für alte Klienten, die nur ASCII
	// verstehen, filename* nach RFC 5987 für alle anderen. So bleibt "Übersicht"
	// eine Übersicht, statt zu "Uebersicht" zu werden -- Umlaute umzuschreiben
	// wäre eine Notlösung aus einer Zeit, in der es filename* noch nicht gab.
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+nurASCII(name)+`.md"; filename*=UTF-8''`+
			url.PathEscape(name+".md"))
	w.Write([]byte(md))
}

// beginntMitUeberschrift prüft, ob das Dokument bereits mit einer Überschrift
// dieses Titels anfängt. Verglichen wird ohne Rücksicht auf Groß- und
// Kleinschreibung: "Wake-on-LAN" und "Wake-On-LAN" meinen dasselbe.
func beginntMitUeberschrift(md, titel string) bool {
	erste := strings.TrimSpace(md)
	if i := strings.Index(erste, "\n"); i >= 0 {
		erste = erste[:i]
	}
	erste = strings.TrimSpace(strings.TrimLeft(erste, "#"))
	return strings.EqualFold(erste, strings.TrimSpace(titel))
}

// dateiname macht aus einem Titel einen Dateinamen. Umlaute bleiben erhalten;
// entfernt wird nur, was Dateisysteme oder der HTTP-Kopf nicht vertragen.
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
	// Dateisysteme setzen bei 255 Bytes die Grenze; mit dem angehängten .md
	// bleibt etwas Luft.
	if len(name) > 200 {
		name = strings.ToValidUTF8(name[:200], "")
	}
	return name
}

// nurASCII ist die Rückfallschreibweise für den filename-Teil des Kopfes.
// Nicht-ASCII wird durch _ ersetzt, nicht umgeschrieben -- die richtige
// Schreibweise steht ohnehin in filename*.
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
