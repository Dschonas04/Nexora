package einlesen

import (
	"strings"
	"unicode"
)

// inlineAus liest den Text innerhalb eines Blocks: Auszeichnungen, Verweise,
// Code, Bilder.
//
// stile ist der Satz, der von außen schon gilt, innerhalb von "**fett**"
// erbt ein "*kursiv*" die Fettung. Die Karte wird nie verändert, sondern beim
// Absteigen kopiert; sonst färbte eine Auszeichnung auf ihre Geschwister ab.
func inlineAus(s string, stile map[string]bool) []Inline {
	var out []Inline
	var b strings.Builder

	fertig := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, Inline{Type: "text", Text: b.String(), Styles: kopie(stile)})
		b.Reset()
	}

	for i := 0; i < len(s); {
		c := s[i]

		// Maskierung: "\*" ist ein Sternchen und keine Auszeichnung. Genau so
		// schreibt der Export es heraus, und genau so kommt es zurück.
		if c == '\\' && i+1 < len(s) && istSatzzeichen(s[i+1]) {
			b.WriteByte(s[i+1])
			i += 2
			continue
		}

		// Code in Rückstrichen. Zuerst geprüft: innerhalb davon gilt keine
		// andere Auszeichnung, sonst würde "`a*b*c`" ausgezeichnet.
		if c == '`' {
			n := laufLaenge(s, i, '`')
			if schluss := findeLauf(s, i+n, '`', n); schluss >= 0 {
				text := s[i+n : schluss]
				// Ein Leerzeichen an beiden Enden gehört zum Zaun, nicht zum
				// Code: so schreibt man einen Rückstrich als Inhalt.
				if len(text) > 2 && strings.HasPrefix(text, " ") && strings.HasSuffix(text, " ") {
					text = text[1 : len(text)-1]
				}
				fertig()
				out = append(out, Inline{Type: "text", Text: text, Styles: mitStil(stile, "code")})
				i = schluss + n
				continue
			}
		}

		// [[Seitentitel]] ist in Nexora ein Verweis und bleibt als Text stehen
		// der Editor erkennt ihn am Muster, und die Rückverweise ebenso.
		if c == '[' && i+1 < len(s) && s[i+1] == '[' {
			if schluss := strings.Index(s[i:], "]]"); schluss > 0 {
				b.WriteString(s[i : i+schluss+2])
				i += schluss + 2
				continue
			}
		}

		// Bild mitten im Text. Der Editor kennt kein Bild innerhalb einer
		// Zeile, nur als eigenen Block. Ein Verweis auf die Datei ist das,
		// was am wenigsten verliert.
		if c == '!' && i+1 < len(s) && s[i+1] == '[' {
			if text, adresse, ende, ok := verweisLesen(s, i+1); ok {
				fertig()
				if text == "" {
					text = adresse
				}
				out = append(out, Inline{
					Type:    "link",
					Href:    adresse,
					Content: []Inline{{Type: "text", Text: text, Styles: kopie(stile)}},
				})
				i = ende
				continue
			}
		}

		if c == '[' {
			if text, adresse, ende, ok := verweisLesen(s, i); ok {
				fertig()
				inhalt := inlineAus(text, stile)
				if len(inhalt) == 0 {
					inhalt = []Inline{{Type: "text", Text: adresse, Styles: kopie(stile)}}
				}
				out = append(out, Inline{Type: "link", Href: adresse, Content: inhalt})
				i = ende
				continue
			}
		}

		// <https://…>, der kurze Verweis, der Adresse und Beschriftung ist.
		if c == '<' {
			if ende := strings.IndexByte(s[i:], '>'); ende > 1 {
				innen := s[i+1 : i+ende]
				if istAdresse(innen) {
					fertig()
					out = append(out, Inline{
						Type:    "link",
						Href:    innen,
						Content: []Inline{{Type: "text", Text: innen, Styles: kopie(stile)}},
					})
					i += ende + 1
					continue
				}
			}
		}

		// Durchstreichung.
		if c == '~' && laufLaenge(s, i, '~') >= 2 {
			if schluss := findeLauf(s, i+2, '~', 2); schluss >= 0 {
				fertig()
				out = append(out, inlineAus(s[i+2:schluss], mitStil(stile, "strike"))...)
				i = schluss + 2
				continue
			}
		}

		// Fett und kursiv. Der längere Lauf zuerst, sonst wäre "**fett**" ein
		// leeres Kursiv gefolgt von Text.
		if c == '*' || c == '_' {
			n := laufLaenge(s, i, c)
			if n > 3 {
				n = 3
			}
			// Ein Unterstrich mitten im Wort zeichnet nichts aus, so liest
			// CommonMark ihn, und Dateinamen wie mein_datei_name bleiben heil.
			if c == '_' && i > 0 && istWortzeichen(s[i-1]) {
				b.WriteByte(c)
				i++
				continue
			}
			if schluss := findeAuszeichnungsEnde(s, i+n, c, n); schluss >= 0 {
				innen := s[i+n : schluss]
				neu := stile
				switch n {
				case 1:
					neu = mitStil(stile, "italic")
				case 2:
					neu = mitStil(stile, "bold")
				case 3:
					neu = mitStil(mitStil(stile, "bold"), "italic")
				}
				fertig()
				out = append(out, inlineAus(innen, neu)...)
				i = schluss + n
				continue
			}
		}

		b.WriteByte(c)
		i++
	}
	fertig()
	return out
}

// verweisLesen liest "[Text](Adresse)" ab Position i und liefert die Position
// hinter der schließenden Klammer.
//
// Die eckigen Klammern werden gezählt statt gesucht: "[siehe [dort]](/a)" hat
// eine innere Klammer, und wer die erste schließende nimmt, zerlegt den Verweis.
func verweisLesen(s string, i int) (text, adresse string, ende int, ok bool) {
	if i >= len(s) || s[i] != '[' {
		return "", "", 0, false
	}
	tiefe := 0
	j := i
	for ; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == '[' {
			tiefe++
		} else if s[j] == ']' {
			tiefe--
			if tiefe == 0 {
				break
			}
		}
	}
	if j >= len(s) || j+1 >= len(s) || s[j+1] != '(' {
		return "", "", 0, false
	}
	text = s[i+1 : j]

	// Die Adresse endet an der passenden runden Klammer. In spitzen Klammern
	// darf sie Leerzeichen enthalten, so schreibt der Export Dateinamen mit
	// Leerzeichen heraus.
	k := j + 2
	if k < len(s) && s[k] == '<' {
		zu := strings.IndexByte(s[k:], '>')
		if zu < 0 {
			return "", "", 0, false
		}
		adresse = s[k+1 : k+zu]
		k += zu + 1
		for k < len(s) && s[k] != ')' {
			k++
		}
		if k >= len(s) {
			return "", "", 0, false
		}
		return text, adresse, k + 1, true
	}

	tiefe = 1
	anfang := k
	for ; k < len(s); k++ {
		if s[k] == '\\' {
			k++
			continue
		}
		if s[k] == '(' {
			tiefe++
		} else if s[k] == ')' {
			tiefe--
			if tiefe == 0 {
				break
			}
		}
	}
	if k >= len(s) {
		return "", "", 0, false
	}
	adresse = strings.TrimSpace(s[anfang:k])
	// Ein Titel hinter der Adresse gehört nicht zur Adresse.
	if leer := strings.IndexAny(adresse, " \t"); leer > 0 {
		adresse = adresse[:leer]
	}
	return text, adresse, k + 1, true
}

// findeAuszeichnungsEnde sucht den schließenden Lauf gleicher Länge.
//
// Ein Lauf schließt nur, wenn davor kein Leerzeichen steht: in "a * b * c" sind
// die Sternchen Zeichen und keine Auszeichnung.
func findeAuszeichnungsEnde(s string, ab int, zeichen byte, n int) int {
	for i := ab; i+n <= len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] != zeichen {
			continue
		}
		if laufLaenge(s, i, zeichen) < n {
			continue
		}
		if i == ab {
			// Leerer Inhalt: "**" direkt hinter "**" zeichnet nichts aus.
			continue
		}
		if s[i-1] == ' ' || s[i-1] == '\t' {
			continue
		}
		if zeichen == '_' && i+n < len(s) && istWortzeichen(s[i+n]) {
			continue
		}
		return i
	}
	return -1
}

// findeLauf sucht einen Lauf von genau n gleichen Zeichen, für Code, wo die
// Zahl der Rückstriche den Zaun bestimmt.
func findeLauf(s string, ab int, zeichen byte, n int) int {
	for i := ab; i+n <= len(s); i++ {
		if s[i] != zeichen {
			continue
		}
		if laufLaenge(s, i, zeichen) == n {
			return i
		}
	}
	return -1
}

func laufLaenge(s string, i int, zeichen byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == zeichen {
		n++
	}
	return n
}

func istWortzeichen(c byte) bool {
	return c == '_' || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c))
}

func istSatzzeichen(c byte) bool {
	return strings.IndexByte("\\`*_{}[]()#+-.!|<>~\"'$", c) >= 0
}

func istAdresse(s string) bool {
	for _, p := range []string{"http://", "https://", "mailto:", "ftp://", "/"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// kopie gibt den Stilsatz weiter, ohne ihn zu teilen.
func kopie(stile map[string]bool) map[string]bool {
	if len(stile) == 0 {
		return nil
	}
	out := make(map[string]bool, len(stile))
	for k, v := range stile {
		out[k] = v
	}
	return out
}

func mitStil(stile map[string]bool, neu string) map[string]bool {
	out := make(map[string]bool, len(stile)+1)
	for k, v := range stile {
		out[k] = v
	}
	out[neu] = true
	return out
}
