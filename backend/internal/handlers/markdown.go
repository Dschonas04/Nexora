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
	"regexp"
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
	schreibeBloecke(&b, bloecke, "")
	// Mehr als eine Leerzeile hintereinander bringt in Markdown nichts und
	// sieht in der Datei nach Versehen aus.
	return strings.TrimSpace(mehrfacheLeerzeilen(b.String())) + "\n"
}

// mehrfacheLeerzeilen dünnt Leerzeilen aus -- aber nur außerhalb von
// Codeblöcken.
//
// Der Unterschied ist kein Feinschliff: in einem Codeblock sind zwei
// Leerzeilen hintereinander Inhalt. Sie stillschweigend zu einer zu machen
// hieße, den exportierten Code zu verändern.
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

// istListe sagt, ob ein Block ein Listeneintrag ist. Steht als eigene Funktion
// da, weil an drei Stellen dieselbe Aufzählung gebraucht wird und eine davon
// eines Tages einen neuen Typ vergessen würde.
func istListe(typ string) bool {
	switch typ {
	case "bulletListItem", "numberedListItem", "checkListItem", "toggleListItem":
		return true
	}
	return false
}

// schreibeBloecke geht die Liste durch. einzug ist die Einrückung, die dieser
// Ebene vorausgeht -- eine Zeichenkette und keine Tiefenzahl, weil
// verschachtelte Einträge sich nicht an einer festen Schrittweite ausrichten,
// sondern an der Breite der Marke ihres Elternteils: unter "1. " beginnt der
// Inhalt in Spalte 3, unter "- " in Spalte 2. Mit einer festen Schrittweite von
// zwei Zeichen wäre ein Untereintrag einer nummerierten Liste keiner, sondern
// eine neue Liste daneben.
func schreibeBloecke(b *strings.Builder, bloecke []block, einzug string) {
	nummer := 0
	// Merkt, ob zuletzt ein Listeneintrag geschrieben wurde. Folgt darauf ein
	// Absatz ohne Leerzeile, liest ihn jeder Markdown-Leser als Fortsetzung
	// des letzten Eintrags und schluckt ihn in die Liste.
	warListe := false

	for _, bl := range bloecke {
		// Breite der Marke dieses Eintrags; nur Listen geben sie an ihre
		// Kinder weiter.
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
			// Eine Klappliste ist beim Lesen eine Aufzählung; Markdown kennt
			// das Zusammenklappen nicht. Der Inhalt ist wichtiger als die
			// Bedienung, also wird sie zur Aufzählung statt zu verschwinden.
			marke := "- "
			markenBreite = len(marke)
			fmt.Fprintf(b, "%s%s%s\n", einzug, marke, zeile(bl.Content, einzug+"  "))
			nummer = 0
			warListe = true

		case "numberedListItem":
			nummer++
			// Eine Liste, die bei 5 anfängt, soll auch bei 5 anfangen.
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
			// Die Fortsetzungsspalte liegt hinter "- ", nicht hinter der
			// Kästchenmarke: das Kästchen gehört bereits zum Inhalt.
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
			// Mehrzeilige Zitate brauchen das > vor jeder Zeile, sonst bricht
			// das Zitat nach der ersten ab.
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
				// Für Video, Ton und Dateien gibt es in Markdown kein
				// Einbetten -- ein Verweis ist das Ehrlichste.
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
				// Ein leerer Absatz ist im Editor eine Leerzeile. Genau das
				// soll er in Markdown auch sein, nicht drei.
				b.WriteString("\n")
			} else {
				fmt.Fprintf(b, "%s%s\n\n", einzug, t)
			}
			nummer = 0

		default:
			// Unbekannter Typ: den Text retten statt den Block zu verlieren.
			if t := zeile(bl.Content, einzug); strings.TrimSpace(t) != "" {
				fmt.Fprintf(b, "%s%s\n\n", einzug, t)
			}
			nummer = 0
		}

		if len(bl.Children) > 0 {
			// Verschachtelte Listen rücken um die Breite der eigenen Marke
			// ein, alles andere bleibt auf der Ebene -- eine eingerückte
			// Überschrift wäre in Markdown ein Codeblock.
			kindEinzug := einzug
			if markenBreite > 0 {
				kindEinzug = einzug + strings.Repeat(" ", markenBreite)
			}
			schreibeBloecke(b, bl.Children, kindEinzug)
			// Nach den Kindern gilt die Merkung des Elternteils nicht mehr;
			// was zuletzt geschrieben wurde, stand in der Unterebene.
			warListe = istListe(bl.Type)
		}
	}
}

func schreibeTabelle(b *strings.Builder, bl block, einzug string) {
	// BlockNote legt die Tabelle unter content.rows ab, nicht als Liste.
	var inhalt struct {
		Rows []struct {
			Cells []json.RawMessage `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(bl.Content, &inhalt); err != nil || len(inhalt.Rows) == 0 {
		return
	}
	// Alle Zeilen auf dieselbe Spaltenzahl bringen. Eine Tabelle mit
	// verbundenen oder fehlenden Zellen hat unterschiedlich lange Zeilen, und
	// eine Zeile mit mehr Trennstrichen als der Kopf zerlegt in vielen Lesern
	// die ganze Tabelle.
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
				// Ein Zeilenumbruch in einer Zelle würde die Tabelle
				// zerreißen; der senkrechte Strich ebenso, deshalb wird er in
				// zellSicher maskiert.
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

// zeile setzt den Inhalt eines Blocks in eine Zeile um und behandelt harte
// Umbrüche.
//
// Ein Umbruch innerhalb eines Absatzes ist im Editor ein Umbruch und kein neuer
// Absatz. Roh übernommen wäre er in Markdown gar nichts -- der Text liefe
// zusammen -- oder, in einem Listeneintrag, das Ende der Liste. Zwei
// Leerzeichen vor dem Umbruch machen daraus den harten Umbruch, und die
// Folgezeile bekommt die Einrückung der Fortsetzungsspalte.
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

// text wandelt den Inhalt eines Blocks in Markdown um, mit Auszeichnungen.
func text(roh json.RawMessage) string {
	if len(roh) == 0 {
		return ""
	}
	var teile []inline
	if err := json.Unmarshal(roh, &teile); err != nil {
		// Zwei andere Gestalten kommen vor. Erstens eine einzelne
		// Zeichenkette. Zweitens ein Objekt mit einem content-Feld -- so
		// liefert der Editor seit einiger Zeit die Zellen einer Tabelle. Ohne
		// diesen zweiten Fall blieben alle Zellen leer: die Tabelle stünde da,
		// aber ohne Inhalt.
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

// entschaerfen maskiert Zeichen, die Markdown als Anweisung liest, obwohl sie
// im Text nur Zeichen sind.
//
// Ohne das ist die Ausgabe nicht das, was auf der Seite stand: aus einem
// Sternchen wird eine Auszeichnung, aus "2026. Ein gutes Jahr" eine
// nummerierte Liste, aus "[[Notiz]]" ein Verweis, den niemand gesetzt hat. Das
// ist genau die Art Abweichung, die beim Wiedereinlesen sichtbar wird.
//
// Maskiert wird sparsam. Ein Unterstrich mitten im Wort bleibt stehen, weil
// CommonMark ihn dort ohnehin nicht als Auszeichnung liest -- ihn zu maskieren
// würde jeden Dateinamen und jeden Bezeichner mit Rückstrichen übersäen, ohne
// dass sich am Ergebnis etwas ändert.
func entschaerfen(s string) string {
	if s == "" {
		return s
	}
	// [[Seitentitel]] ist in Nexora ein Verweis, auch wenn er als gewöhnlicher
	// Text im Dokument steht -- der Editor erkennt ihn am Muster. Würden die
	// Klammern maskiert, käme aus dem Export ein Text zurück, der einmal ein
	// Verweis war und keiner mehr ist. Also bleiben diese Stellen unberührt
	// und nur das dazwischen wird entschärft.
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

// wikiMuster ist dasselbe Muster, das die Oberfläche für [[Verweise]] benutzt.
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
			// Nur am Wortrand. davor/danach ein Buchstabe oder eine Ziffer
			// heißt: mitten im Wort, dort wirkt der Unterstrich nicht.
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

// zeilenanfaengeEntschaerfen kümmert sich um die Zeichen, die nur am
// Zeilenanfang etwas bedeuten: Raute, Strich, Größerzeichen, Ziffer mit Punkt.
// Mitten im Satz sind sie harmlos und bleiben unangetastet.
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
			// "1. " oder "12) " am Anfang macht eine nummerierte Liste.
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

// klammersicher schützt den sichtbaren Teil eines Verweises. Eine eckige
// Klammer darin beendet ihn sonst an der falschen Stelle.
func klammersicher(s string) string {
	s = strings.ReplaceAll(s, "[", "\\[")
	return strings.ReplaceAll(s, "]", "\\]")
}

// adressSicher macht eine Adresse verweistauglich. Runde Klammern und
// Leerzeichen beenden eine Markdown-Adresse; spitze Klammern drumherum sind der
// vorgesehene Ausweg.
func adressSicher(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsAny(s, " ()<>") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E", " ", "%20").Replace(s) + ">"
	}
	return s
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
		// Im Code gilt die Regel von oben NICHT: Leerzeichen am Anfang sind
		// dort kein Rand, sondern Einrückung, und Einrückung ist in Code
		// Inhalt. Stünde sie außerhalb der Rückstriche, verlöre sie jeder,
		// der die Datei wieder einliest.
		kern = s
		links, rechts = "", ""

		// Im Code bleibt jedes Zeichen, wie es ist -- deshalb wird hier NICHT
		// maskiert. Stattdessen wird der Zaun so lang gemacht, dass er länger
		// ist als jede Backtick-Folge im Text: anders bekommt man einen
		// Backtick nicht in eine Code-Spanne.
		zaun := "`"
		for strings.Contains(kern, zaun) {
			zaun += "`"
		}
		// Ein Füllzeichen an beiden Enden braucht es in zwei Fällen: wenn der
		// Inhalt selbst mit einem Rückstrich anfängt oder aufhört, und wenn er
		// an beiden Enden ein Leerzeichen hat -- genau dann nimmt ein Leser
		// nach der Regel je eines wieder weg, und ohne Füllung wäre das der
		// Inhalt.
		fuellung := ""
		if strings.HasPrefix(kern, "`") || strings.HasSuffix(kern, "`") ||
			(strings.HasPrefix(kern, " ") && strings.HasSuffix(kern, " ")) {
			fuellung = " "
		}
		kern = zaun + fuellung + kern + fuellung + zaun
	} else {
		// Alles, was kein Code ist, wird entschärft: was im Editor ein
		// Sternchen war, soll in der Datei ein Sternchen bleiben und keine
		// Auszeichnung werden.
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
