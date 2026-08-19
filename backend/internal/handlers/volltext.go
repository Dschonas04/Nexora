// Plain text extraction for the search index.
//
// A page's content is BlockNote JSON: nested blocks, each with an array of
// inline pieces that carry the actual words. Searching that JSON as raw text
// was what the old implementation did, and it matched key names and block ids
// as readily as prose. Here the words are pulled out once on save, so the index
// contains what a reader would call the text of the page.
package handlers

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// maxTextLaenge caps what goes into the index. A tsvector is limited to about a
// megabyte, and a page that long is pathological anyway; cutting is better than
// letting the insert fail.
const maxTextLaenge = 400_000

// textAusInhalt walks the BlockNote document and returns its words separated by
// spaces. It is deliberately tolerant: unknown block types, missing fields and
// malformed content yield less text, never an error. A page must remain
// saveable even when its content cannot be indexed.
func textAusInhalt(inhalt json.RawMessage) string {
	if len(inhalt) == 0 {
		return ""
	}
	var wurzel interface{}
	if err := json.Unmarshal(inhalt, &wurzel); err != nil {
		return ""
	}
	var b strings.Builder
	sammle(wurzel, &b)

	s := strings.Join(strings.Fields(b.String()), " ")
	if len(s) > maxTextLaenge {
		s = s[:maxTextLaenge]
		// Cutting by bytes can split a multi-byte character; trim the remainder
		// so the column never holds invalid UTF-8.
		for len(s) > 0 && !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// sammle recurses through the document. Only two shapes carry words in
// BlockNote: a "text" field on an inline piece, and the "caption"/"name" a
// media block uses for its label. Everything else is structure.
func sammle(v interface{}, b *strings.Builder) {
	switch x := v.(type) {
	case []interface{}:
		for _, e := range x {
			sammle(e, b)
		}
	case map[string]interface{}:
		for _, feld := range []string{"text", "caption", "name", "title", "url"} {
			if s, ok := x[feld].(string); ok && s != "" {
				// url is included because a link's target is often the only
				// clue to what a bare "hier" link points at.
				b.WriteString(s)
				b.WriteByte(' ')
			}
		}
		// Children and inline content live under their own keys; recursing over
		// every value covers them without naming each one.
		for schluessel, wert := range x {
			switch schluessel {
			case "text", "caption", "name", "title", "url", "type", "id":
				// Already taken, or pure structure.
				continue
			}
			sammle(wert, b)
		}
	}
}
