package einlesen

import (
	"strconv"
	"strings"
)

// stapelEintrag hält einen offenen Listeneintrag samt der Spalte, in der seine
// Marke stand. Ein tiefer eingerückter Eintrag ist sein Kind.
type stapelEintrag struct {
	einzug int
	k      *knoten
}

// bloeckeAus liest die Zeilen eines Dokuments.
//
// Zeilenweise und ohne Rückschau über mehr als eine Zeile hinaus: Markdown ist
// zeilenorientiert, und ein Leser, der jederzeit sagen kann, in welchem Zustand
// er ist, bleibt änderbar. Ein vollständiger CommonMark-Leser wäre das Zehnfache
// an Code für Fälle, die in Notizen nicht vorkommen.
func bloeckeAus(zeilen []string) []Block {
	var wurzel []*knoten
	var stapel []stapelEintrag

	// anhaengen hängt einen Listeneintrag an der richtigen Stelle ein: unter
	// den letzten offenen Eintrag, der weniger weit eingerückt ist.
	anhaengen := func(k *knoten, einzug int) {
		for len(stapel) > 0 && stapel[len(stapel)-1].einzug >= einzug {
			stapel = stapel[:len(stapel)-1]
		}
		if len(stapel) == 0 {
			ersterDerListe(k, wurzel)
			wurzel = append(wurzel, k)
		} else {
			eltern := stapel[len(stapel)-1].k
			ersterDerListe(k, eltern.kinder)
			eltern.kinder = append(eltern.kinder, k)
		}
		stapel = append(stapel, stapelEintrag{einzug: einzug, k: k})
	}
	// gerade beendet die Liste: der nächste Block steht wieder ganz links.
	gerade := func(k *knoten) {
		stapel = stapel[:0]
		wurzel = append(wurzel, k)
	}

	for i := 0; i < len(zeilen); i++ {
		z := zeilen[i]
		leer := strings.TrimSpace(z) == ""

		if leer {
			// Eine Leerzeile beendet einen Absatz, aber keine Liste: zwischen
			// zwei Listeneinträgen darf eine stehen, und die Liste geht weiter.
			continue
		}

		// Codezaun. Zuerst geprüft, weil innerhalb des Zauns keine andere
		// Regel gilt -- ein "# " dort ist eine Raute und keine Überschrift.
		if m := zaunMuster.FindStringSubmatch(z); m != nil {
			zaun := m[1][:1]
			var code []string
			i++
			for ; i < len(zeilen); i++ {
				if t := strings.TrimSpace(zeilen[i]); strings.HasPrefix(t, zaun+zaun+zaun) && strings.Trim(t, zaun) == "" {
					break
				}
				code = append(code, zeilen[i])
			}
			for _, b := range codeBloecke(code, m[2]) {
				gerade(&knoten{blk: b})
			}
			continue
		}

		// Überschrift.
		if m := ueberschriftMuster.FindStringSubmatch(z); m != nil {
			stufe := len(m[1])
			// Der Editor kennt drei Stufen. Tiefere zusammenzulegen verliert
			// Gliederung, sie zu Absätzen zu machen verlöre die Überschrift
			// ganz -- und in einer dreistufigen Ansicht fällt eine vierte
			// Ebene ohnehin kaum auf.
			if stufe > 3 {
				stufe = 3
			}
			gerade(&knoten{blk: Block{
				Type:    "heading",
				Props:   map[string]any{"level": stufe},
				Content: inlineAus(m[2], nil),
			}})
			continue
		}

		// Waagerechte Linie. Der Editor hat keine; drei Geviertstriche sind
		// das, was am ehesten danach aussieht.
		if trennerMuster.MatchString(z) {
			gerade(&knoten{blk: Block{
				Type:    "paragraph",
				Content: []Inline{{Type: "text", Text: "———"}},
			}})
			continue
		}

		// Tabelle: eine Zeile mit senkrechten Strichen, deren Nachfolgerin die
		// Trennzeile ist. Ohne diese zweite Bedingung wäre jeder Satz mit einem
		// senkrechten Strich eine Tabelle.
		if strings.Contains(z, "|") && i+1 < len(zeilen) && trennzeileMuster.MatchString(zeilen[i+1]) {
			var roh []string
			roh = append(roh, z)
			i++ // Trennzeile überspringen
			for i+1 < len(zeilen) && strings.Contains(zeilen[i+1], "|") && strings.TrimSpace(zeilen[i+1]) != "" {
				i++
				roh = append(roh, zeilen[i])
			}
			gerade(&knoten{blk: tabelle(roh)})
			continue
		}

		// Zitat. Der Editor hat keinen Zitatblock, also wird der Inhalt kursiv
		// gesetzt und das Zeichen ">" fällt weg. Mehrere Zitatzeilen
		// hintereinander gehören zu einem Zitat.
		if strings.HasPrefix(strings.TrimLeft(z, " "), ">") {
			var zitat []string
			for i < len(zeilen) && strings.HasPrefix(strings.TrimLeft(zeilen[i], " "), ">") {
				t := strings.TrimLeft(zeilen[i], " ")
				t = strings.TrimPrefix(t, ">")
				zitat = append(zitat, strings.TrimPrefix(t, " "))
				i++
			}
			i--
			for _, b := range bloeckeAus(zitat) {
				gerade(&knoten{blk: kursivMachen(b)})
			}
			continue
		}

		// Listeneintrag.
		if k, einzug, ok := listenEintrag(z); ok {
			anhaengen(k, einzug)
			continue
		}

		// Absatz. Alle folgenden Zeilen gehören dazu, bis eine Leerzeile oder
		// ein Block anderer Art kommt.
		// Roh gesammelt, ungetrimmt: zwei Leerzeichen am Zeilenende sind in
		// Markdown ein harter Umbruch, und wer vorher trimmt, hat ihn verloren.
		var absatz []string
		absatz = append(absatz, z)
		for i+1 < len(zeilen) {
			n := zeilen[i+1]
			if strings.TrimSpace(n) == "" || beginntBlock(n) {
				break
			}
			i++
			absatz = append(absatz, n)
		}
		text := absatzText(absatz)

		// Eine eingerückte Zeile, die ganz aus einem Codestück besteht, kommt
		// aus einem Codeblock. Der Editor hat keinen; der Export schreibt
		// deshalb jede Zeile als `Text`, und die Einrückung steht davor statt
		// darin. Wandert sie nicht in das Codestück hinein, verliert jeder
		// eingerückte Codeblock beim Wiedereinlesen seine Stufen -- und
		// Einrückung ist in Code kein Schmuck.
		if len(absatz) == 1 && len(stapel) == 0 {
			if einzug := absatz[0][:einzugVon(absatz[0])]; einzug != "" {
				if inneres, marke, ok := ganzCode(text); ok {
					text = marke + strings.ReplaceAll(einzug, "\t", "    ") + inneres + marke
				}
			}
		}

		// Eine Zeile, die nur aus einem Bild besteht, wird ein Bildblock. In
		// einen Absatz gepackt wäre sie ein Verweis, den man anklicken muss --
		// gemeint war ein Bild, das man sieht.
		if m := bildAlleinMuster.FindStringSubmatch(text); m != nil {
			gerade(&knoten{blk: bildBlock(m[1], m[2])})
			continue
		}

		// Fortsetzung eines Listeneintrags: eingerückter Fließtext unter einem
		// Eintrag gehört zu ihm, nicht neben die Liste.
		if len(stapel) > 0 && einzugVon(z) > stapel[len(stapel)-1].einzug {
			letzter := stapel[len(stapel)-1].k
			letzter.blk.Content = append(letzter.blk.Content.([]Inline),
				append([]Inline{{Type: "text", Text: "\n"}}, inlineAus(text, nil)...)...)
			continue
		}

		gerade(&knoten{blk: Block{Type: "paragraph", Content: inlineAus(text, nil)}})
	}

	// Ein Dokument ohne einen einzigen Block wäre für den Editor kein gültiger
	// Anfangsinhalt; ein leerer Absatz ist die leere Seite.
	if len(wurzel) == 0 {
		return []Block{{Type: "paragraph"}}
	}
	out := make([]Block, 0, len(wurzel))
	for _, k := range wurzel {
		out = append(out, k.bauen())
	}
	return out
}

// ersterDerListe nimmt die Anfangszahl weg, wenn schon ein nummerierter
// Eintrag davor steht.
//
// BlockNote zählt eine Liste selbst durch und liest die Angabe nur am ersten
// Eintrag. Stünde sie an jedem, finge nach dem Speichern jeder Eintrag eine
// neue Liste an -- aus "5. 6. 7." würde "5. 5. 5.".
func ersterDerListe(k *knoten, geschwister []*knoten) {
	if k.blk.Type != "numberedListItem" || len(geschwister) == 0 {
		return
	}
	if geschwister[len(geschwister)-1].blk.Type != "numberedListItem" {
		return
	}
	delete(k.blk.Props, "start")
	if len(k.blk.Props) == 0 {
		k.blk.Props = nil
	}
}

// ganzCode sagt, ob eine Zeile aus nichts als einem Codestück besteht, und
// liefert dessen Inhalt samt der Zahl der Rückstriche, die ihn einfassen.
func ganzCode(z string) (string, string, bool) {
	n := 0
	for n < len(z) && z[n] == '`' {
		n++
	}
	if n == 0 || len(z) < 2*n+1 {
		return "", "", false
	}
	marke := z[:n]
	if !strings.HasSuffix(z, marke) {
		return "", "", false
	}
	inneres := z[n : len(z)-n]
	// Ein Rückstrichlauf derselben Länge im Inneren hieße, dass die Zeile aus
	// mehreren Stücken besteht und nicht aus einem.
	if strings.Contains(inneres, marke) {
		return "", "", false
	}
	return inneres, marke, true
}

// beginntBlock sagt, ob eine Zeile einen neuen Block anfängt. Ohne das würde
// eine Liste, die ohne Leerzeile auf einen Absatz folgt, im Absatz verschwinden.
func beginntBlock(z string) bool {
	if ueberschriftMuster.MatchString(z) || trennerMuster.MatchString(z) ||
		zaunMuster.MatchString(z) || aufzaehlungMuster.MatchString(z) ||
		nummerMuster.MatchString(z) {
		return true
	}
	return strings.HasPrefix(strings.TrimLeft(z, " "), ">")
}

func einzugVon(z string) int {
	return len(z) - len(strings.TrimLeft(z, " \t"))
}

// listenEintrag erkennt Aufzählung, Nummerierung und Kästchen.
func listenEintrag(z string) (*knoten, int, bool) {
	if m := aufzaehlungMuster.FindStringSubmatch(z); m != nil {
		einzug := len(strings.ReplaceAll(m[1], "\t", "    "))
		rest := m[3]
		// "- [x] Text" ist kein Aufzählungspunkt, sondern ein Kästchen.
		if h := hakenMuster.FindStringSubmatch(rest); h != nil {
			return &knoten{blk: Block{
				Type:    "checkListItem",
				Props:   map[string]any{"checked": h[1] == "x" || h[1] == "X"},
				Content: inlineAus(h[2], nil),
			}}, einzug, true
		}
		return &knoten{blk: Block{Type: "bulletListItem", Content: inlineAus(rest, nil)}}, einzug, true
	}
	if m := nummerMuster.FindStringSubmatch(z); m != nil {
		einzug := len(strings.ReplaceAll(m[1], "\t", "    "))
		props := map[string]any{}
		// Eine Liste, die bei 5 anfängt, soll bei 5 anfangen. BlockNote merkt
		// sich das nur am ersten Eintrag; bei allen anderen wäre die Angabe
		// falsch, weil der Editor selbst durchzählt.
		if n, err := strconv.Atoi(m[2]); err == nil && n != 1 {
			props["start"] = n
		}
		if len(props) == 0 {
			props = nil
		}
		return &knoten{blk: Block{Type: "numberedListItem", Props: props, Content: inlineAus(m[3], nil)}}, einzug, true
	}
	return nil, 0, false
}

// absatzText fügt die Zeilen eines Absatzes zusammen.
//
// Ein weicher Umbruch -- eine Zeile, die einfach zu Ende ist -- wird zum
// Leerzeichen, so will es Markdown. Zwei Leerzeichen oder ein Rückstrich am
// Ende sind ein harter Umbruch und bleiben einer: im Editor steht dann ein
// Zeilenumbruch mitten im Absatz, und der Export schreibt ihn wieder als
// "  \n". Ohne diese Unterscheidung liefe eine Anschrift oder ein Gedicht beim
// Import zu einer einzigen Zeile zusammen.
func absatzText(zeilen []string) string {
	var b strings.Builder
	hartVorher := false
	for i, z := range zeilen {
		hart := strings.HasSuffix(z, "  ") || strings.HasSuffix(z, "\\")
		z = strings.TrimSpace(strings.TrimSuffix(strings.TrimRight(z, " "), "\\"))
		if i > 0 {
			if hartVorher {
				b.WriteString("\n")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(z)
		hartVorher = hart
	}
	return b.String()
}

// codeBloecke macht aus den Zeilen eines Codezauns Absätze in fester Schrift.
//
// Der Editor dieser Fassung hat keinen Codeblock. Ein Absatz je Zeile hält die
// Umbrüche und die Reihenfolge; alles in einen Absatz zu legen sähe im Editor
// zwar geschlossener aus, ließe sich aber nicht mehr zeilenweise bearbeiten.
// Die Sprachangabe geht dabei verloren -- sie steht in keinem Feld, das der
// Editor kennt.
func codeBloecke(zeilen []string, sprache string) []Block {
	var out []Block
	if sprache != "" {
		out = append(out, Block{
			Type:    "paragraph",
			Content: []Inline{{Type: "text", Text: sprache, Styles: map[string]bool{"code": true, "italic": true}}},
		})
	}
	for _, z := range zeilen {
		if z == "" {
			out = append(out, Block{Type: "paragraph"})
			continue
		}
		out = append(out, Block{
			Type:    "paragraph",
			Content: []Inline{{Type: "text", Text: z, Styles: map[string]bool{"code": true}}},
		})
	}
	return out
}

// kursivMachen zeichnet einen ganzen Block kursiv aus -- der Ersatz für den
// Zitatblock, den der Editor nicht hat.
func kursivMachen(b Block) Block {
	if teile, ok := b.Content.([]Inline); ok {
		for i := range teile {
			if teile[i].Styles == nil {
				teile[i].Styles = map[string]bool{}
			}
			teile[i].Styles["italic"] = true
		}
		b.Content = teile
	}
	for i := range b.Children {
		b.Children[i] = kursivMachen(b.Children[i])
	}
	return b
}

// tabelle liest die Zeilen einer Tabelle. Alle Zeilen bekommen die Breite der
// breitesten: eine Zeile mit weniger Zellen als der Kopf würde den Editor sonst
// mit einer unvollständigen Tabelle zurücklassen.
func tabelle(zeilen []string) Block {
	var roh [][]string
	breite := 0
	for _, z := range zeilen {
		z := zellenTeilen(z)
		if len(z) > breite {
			breite = len(z)
		}
		roh = append(roh, z)
	}
	if breite == 0 {
		return Block{Type: "paragraph"}
	}
	inhalt := TabellenInhalt{Type: "tableContent"}
	for _, z := range roh {
		zeile := TabellenZeile{Cells: make([][]Inline, breite)}
		for i := range zeile.Cells {
			if i < len(z) {
				zeile.Cells[i] = inlineAus(z[i], nil)
			} else {
				zeile.Cells[i] = []Inline{}
			}
		}
		inhalt.Rows = append(inhalt.Rows, zeile)
	}
	return Block{Type: "table", Content: inhalt}
}

// zellenTeilen trennt an senkrechten Strichen, aber nicht an maskierten: "\|"
// ist ein Strich im Text und keine Zellengrenze.
func zellenTeilen(z string) []string {
	z = strings.TrimSpace(z)
	z = strings.TrimPrefix(z, "|")
	z = strings.TrimSuffix(z, "|")
	var zellen []string
	var b strings.Builder
	for i := 0; i < len(z); i++ {
		if z[i] == '\\' && i+1 < len(z) && z[i+1] == '|' {
			b.WriteByte('|')
			i++
			continue
		}
		if z[i] == '|' {
			zellen = append(zellen, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteByte(z[i])
	}
	zellen = append(zellen, strings.TrimSpace(b.String()))
	return zellen
}

// bildBlock baut den Bildblock. Die Adresse behält ihre Gestalt; ob sie auf
// eine Datei im Archiv zeigt und umgeschrieben werden muss, entscheidet der
// Aufrufer, nicht der Leser.
func bildBlock(alt, adresse string) Block {
	adresse = strings.TrimSuffix(strings.TrimPrefix(adresse, "<"), ">")
	return Block{
		Type: "image",
		Props: map[string]any{
			"url":     adresse,
			"name":    alt,
			"caption": alt,
		},
	}
}
