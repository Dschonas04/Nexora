package einlesen

import (
	"strings"
	"unicode"
)

// inlineAus reads the text inside a block: styles, links, code, images.
//
// stile is the set already in force from the outside; inside "**bold**" an
// "*italic*" inherits the bold. The map is never modified but copied while
// descending; otherwise a style would bleed onto its siblings.
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

		// Escaping: "\*" is an asterisk and not a style. That is exactly how the
		// export writes it out, and exactly how it comes back.
		if c == '\\' && i+1 < len(s) && istSatzzeichen(s[i+1]) {
			b.WriteByte(s[i+1])
			i += 2
			continue
		}

		// Code in backticks. Checked first: no other style applies inside it,
		// otherwise "`a*b*c`" would come out styled.
		if c == '`' {
			n := laufLaenge(s, i, '`')
			if schluss := findeLauf(s, i+n, '`', n); schluss >= 0 {
				text := s[i+n : schluss]
				// A space at both ends belongs to the fence, not to the code: that is
				// how one writes a backtick as content.
				if len(text) > 2 && strings.HasPrefix(text, " ") && strings.HasSuffix(text, " ") {
					text = text[1 : len(text)-1]
				}
				fertig()
				out = append(out, Inline{Type: "text", Text: text, Styles: mitStil(stile, "code")})
				i = schluss + n
				continue
			}
		}

		// [[Page title]] is a link in Nexora and stays as text; the editor
		// recognises it by the pattern, and so do the backlinks.
		if c == '[' && i+1 < len(s) && s[i+1] == '[' {
			if schluss := strings.Index(s[i:], "]]"); schluss > 0 {
				b.WriteString(s[i : i+schluss+2])
				i += schluss + 2
				continue
			}
		}

		// An image in the middle of the text. The editor has no image inside a
		// line, only as a block of its own. A link to the file is what loses the
		// least.
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

		// <https://…>, the short link that is address and label in one.
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

		// Bold and italic. The longer run first, otherwise "**bold**" would be an
		// empty italic followed by text.
		if c == '*' || c == '_' {
			n := laufLaenge(s, i, c)
			if n > 3 {
				n = 3
			}
			// An underscore in the middle of a word styles nothing, that is how
			// CommonMark reads it, and file names like my_file_name stay intact.
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

// verweisLesen reads "[text](address)" from position i and returns the position
// behind the closing bracket.
//
// The square brackets are counted rather than searched for: "[see [there]](/a)"
// has an inner bracket, and whoever takes the first closing one tears the link
// apart.
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

	// The address ends at the matching round bracket. Inside angle brackets it
	// may contain spaces; that is how the export writes out file names with
	// spaces in them.
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
	// A title behind the address is not part of the address.
	if leer := strings.IndexAny(adresse, " \t"); leer > 0 {
		adresse = adresse[:leer]
	}
	return text, adresse, k + 1, true
}

// findeAuszeichnungsEnde looks for the closing run of equal length.
//
// A run only closes when no space precedes it: in "a * b * c" the asterisks are
// characters and not a style.
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
			// Empty content: "**" right after "**" formats nothing.
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

// findeLauf looks for a run of exactly n identical characters, for code, where
// the number of backticks determines the fence.
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

// kopie passes the style set on without sharing it.
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
