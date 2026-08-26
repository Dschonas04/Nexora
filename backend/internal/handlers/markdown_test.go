package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func um(t *testing.T, j string) string {
	t.Helper()
	return MarkdownAusInhalt(json.RawMessage(j))
}

func enthaelt(t *testing.T, got, will string) {
	t.Helper()
	if !strings.Contains(got, will) {
		t.Fatalf("erwartet %q in:\n%s", will, got)
	}
}

func TestUeberschriften(t *testing.T) {
	md := um(t, `[
	 {"type":"heading","props":{"level":1},"content":[{"type":"text","text":"Eins"}]},
	 {"type":"heading","props":{"level":3},"content":[{"type":"text","text":"Drei"}]}]`)
	enthaelt(t, md, "# Eins")
	enthaelt(t, md, "### Drei")
}

func TestAuszeichnungen(t *testing.T) {
	md := um(t, `[{"type":"paragraph","content":[
	 {"type":"text","text":"fett","styles":{"bold":true}},
	 {"type":"text","text":" "},
	 {"type":"text","text":"kursiv","styles":{"italic":true}},
	 {"type":"text","text":" "},
	 {"type":"text","text":"weg","styles":{"strike":true}},
	 {"type":"text","text":" "},
	 {"type":"text","text":"code","styles":{"code":true}}]}]`)
	enthaelt(t, md, "**fett**")
	enthaelt(t, md, "*kursiv*")
	enthaelt(t, md, "~~weg~~")
	enthaelt(t, md, "`code`")
}

func TestCodeBleibtInnen(t *testing.T) {
	// Bold AND code: the backticks have to sit inside, otherwise the asterisk
	// stands in the code and is no longer read as a style.
	md := um(t, `[{"type":"paragraph","content":[
	 {"type":"text","text":"x","styles":{"bold":true,"code":true}}]}]`)
	enthaelt(t, md, "**`x`**")
}

func TestLeerzeichenBleibenAussen(t *testing.T) {
	// "** bold **" is rendered as bold by no reader. The spaces belong before and
	// after the asterisks.
	md := um(t, `[{"type":"paragraph","content":[
	 {"type":"text","text":" fett ","styles":{"bold":true}}]}]`)
	if strings.Contains(md, "** fett **") {
		t.Fatalf("Leerzeichen innerhalb der Auszeichnung:\n%s", md)
	}
	enthaelt(t, md, "**fett**")
}

func TestVerschachtelteListe(t *testing.T) {
	md := um(t, `[{"type":"bulletListItem","content":[{"type":"text","text":"oben"}],
	 "children":[{"type":"bulletListItem","content":[{"type":"text","text":"unten"}]}]}]`)
	enthaelt(t, md, "- oben")
	enthaelt(t, md, "  - unten")
}

func TestNummerierungZaehltUndSetztZurueck(t *testing.T) {
	md := um(t, `[
	 {"type":"numberedListItem","content":[{"type":"text","text":"a"}]},
	 {"type":"numberedListItem","content":[{"type":"text","text":"b"}]},
	 {"type":"paragraph","content":[{"type":"text","text":"dazwischen"}]},
	 {"type":"numberedListItem","content":[{"type":"text","text":"c"}]}]`)
	enthaelt(t, md, "1. a")
	enthaelt(t, md, "2. b")
	// After the paragraph a new list begins, and it has to start at 1 again.
	if !strings.Contains(md, "1. c") {
		t.Fatalf("Zähler wurde nicht zurückgesetzt:\n%s", md)
	}
}

func TestAufgabenliste(t *testing.T) {
	md := um(t, `[
	 {"type":"checkListItem","props":{"checked":true},"content":[{"type":"text","text":"fertig"}]},
	 {"type":"checkListItem","props":{"checked":false},"content":[{"type":"text","text":"offen"}]}]`)
	enthaelt(t, md, "- [x] fertig")
	enthaelt(t, md, "- [ ] offen")
}

func TestCodeblockOhneAuszeichnung(t *testing.T) {
	// Im Codeblock müssen Sternchen Sternchen bleiben.
	md := um(t, `[{"type":"codeBlock","props":{"language":"go"},
	 "content":[{"type":"text","text":"a := *b","styles":{"bold":true}}]}]`)
	enthaelt(t, md, "```go")
	enthaelt(t, md, "a := *b")
	if strings.Contains(md, "**a") {
		t.Fatalf("Auszeichnung im Codeblock angewandt:\n%s", md)
	}
}

func TestMehrzeiligesZitat(t *testing.T) {
	md := um(t, `[{"type":"quote","content":[{"type":"text","text":"erste\nzweite"}]}]`)
	enthaelt(t, md, "> erste")
	enthaelt(t, md, "> zweite")
}

func TestVerknuepfungUndErwaehnung(t *testing.T) {
	md := um(t, `[{"type":"paragraph","content":[
	 {"type":"link","href":"https://example.de","content":[{"type":"text","text":"hier"}]},
	 {"type":"text","text":" und "},
	 {"type":"mention","props":{"title":"Andere Seite"}}]}]`)
	enthaelt(t, md, "[hier](https://example.de)")
	enthaelt(t, md, "[[Andere Seite]]")
}

func TestTabelle(t *testing.T) {
	md := um(t, `[{"type":"table","content":{"rows":[
	 {"cells":[[{"type":"text","text":"A"}],[{"type":"text","text":"B"}]]},
	 {"cells":[[{"type":"text","text":"1"}],[{"type":"text","text":"2"}]]}]}}]`)
	enthaelt(t, md, "| A | B |")
	enthaelt(t, md, "| --- | --- |")
	enthaelt(t, md, "| 1 | 2 |")
}

func TestBild(t *testing.T) {
	md := um(t, `[{"type":"image","props":{"url":"/api/x.png","name":"Plan"}}]`)
	enthaelt(t, md, "![Plan](/api/x.png)")
}

func TestUnbekannterBlockVerliertKeinenText(t *testing.T) {
	// A type this version does not know must not swallow its text; exporting
	// incompletely is better than losing.
	md := um(t, `[{"type":"gibtEsNochNicht","content":[{"type":"text","text":"wichtig"}]}]`)
	enthaelt(t, md, "wichtig")
}

func TestKaputterInhaltStuerztNicht(t *testing.T) {
	for _, j := range []string{``, `null`, `{}`, `[{"type":"paragraph"}]`, `[1,2,3]`} {
		_ = MarkdownAusInhalt(json.RawMessage(j))
	}
}

func TestDateinameBehaeltUmlaute(t *testing.T) {
	if got := dateiname("Übersicht: Dienste/Ports"); !strings.Contains(got, "Ü") {
		t.Fatalf("Umlaut verloren: %q", got)
	}
	if got := dateiname("a/b:c"); strings.ContainsAny(got, `/:`) {
		t.Fatalf("verbotene Zeichen geblieben: %q", got)
	}
	if got := dateiname("   "); got != "seite" {
		t.Fatalf("leerer Titel ergab %q", got)
	}
}

func TestTitelWirdNichtVerdoppelt(t *testing.T) {
	md := "# Wake-on-LAN\n\nText"
	if !beginntMitUeberschrift(md, "Wake-on-LAN") {
		t.Fatal("gleiche Überschrift nicht erkannt")
	}
	// Even when spelled differently, otherwise the title would appear twice.
	if !beginntMitUeberschrift(md, "wake-on-lan") {
		t.Fatal("Groß- und Kleinschreibung wurde nicht ignoriert")
	}
	if beginntMitUeberschrift("## Etwas anderes\n", "Wake-on-LAN") {
		t.Fatal("fremde Überschrift fälschlich als Titel erkannt")
	}
	if beginntMitUeberschrift("", "Titel") {
		t.Fatal("leeres Dokument darf keine Überschrift melden")
	}
}

func nichtEnthaelt(t *testing.T, got, darfNicht string) {
	t.Helper()
	if strings.Contains(got, darfNicht) {
		t.Fatalf("unerwartet %q in:\n%s", darfNicht, got)
	}
}

// A paragraph directly after a list item, without a blank line in between, is
// the continuation of that item to every Markdown reader, and the paragraph then
// disappears into the list.
func TestAbsatzNachListeBekommtLeerzeile(t *testing.T) {
	md := um(t, `[
	 {"type":"bulletListItem","content":[{"type":"text","text":"Punkt"}]},
	 {"type":"paragraph","content":[{"type":"text","text":"Danach"}]}]`)
	enthaelt(t, md, "- Punkt\n\nDanach")
}

// Under "1. " the content begins in column 3. Two spaces of indent are not
// enough there: the sub item would be a second list beside it.
func TestUntereintragUnterNummerRuecktWeitGenugEin(t *testing.T) {
	md := um(t, `[{"type":"numberedListItem","content":[{"type":"text","text":"Eins"}],
	  "children":[{"type":"bulletListItem","content":[{"type":"text","text":"Unter"}]}]}]`)
	enthaelt(t, md, "1. Eins\n   - Unter")
}

// Newer editor versions deliver table cells as an object with a content field
// instead of as a bare list. If that is not recognised the table stands there
// empty.
func TestTabellenzelleAlsObjekt(t *testing.T) {
	md := um(t, `[{"type":"table","content":{"rows":[
	 {"cells":[{"type":"tableCell","content":[{"type":"text","text":"A"}]},
	           {"type":"tableCell","content":[{"type":"text","text":"B"}]}]}]}}]`)
	enthaelt(t, md, "| A | B |")
}

func TestSenkrechterStrichZerreisstTabelleNicht(t *testing.T) {
	md := um(t, `[{"type":"table","content":{"rows":[
	 {"cells":[[{"type":"text","text":"a|b"}]]}]}}]`)
	enthaelt(t, md, `| a\|b |`)
}

// Short rows get the same number of columns as the longest one. Otherwise a row
// with fewer cells tears the table apart on reading.
func TestTabelleWirdRechteckig(t *testing.T) {
	md := um(t, `[{"type":"table","content":{"rows":[
	 {"cells":[[{"type":"text","text":"A"}],[{"type":"text","text":"B"}]]},
	 {"cells":[[{"type":"text","text":"1"}]]}]}}]`)
	enthaelt(t, md, "| 1 |  |")
}

func TestSonderzeichenBleibenText(t *testing.T) {
	md := um(t, `[{"type":"paragraph","content":[{"type":"text","text":"2 * 3 und [Klammer]"}]}]`)
	enthaelt(t, md, `2 \* 3 und \[Klammer\]`)
}

func TestZeilenanfangWirdEntschaerft(t *testing.T) {
	md := um(t, `[{"type":"paragraph","content":[{"type":"text","text":"1998. Ein Jahr"}]}]`)
	enthaelt(t, md, `1998\. Ein Jahr`)
	nichtEnthaelt(t, md, "\n1998. ")
}

// [[Title]] is a link in Nexora, even as plain text. Escaping the brackets makes
// the export return a text that is no longer one.
func TestWikiVerweisBleibtStehen(t *testing.T) {
	md := um(t, `[{"type":"paragraph","content":[{"type":"text","text":"siehe [[Notiz]] dort"}]}]`)
	enthaelt(t, md, "siehe [[Notiz]] dort")
}

func TestUnterstrichImWortBleibt(t *testing.T) {
	md := um(t, `[{"type":"paragraph","content":[{"type":"text","text":"datei_name_lang"}]}]`)
	enthaelt(t, md, "datei_name_lang")
}

func TestBacktickPasstInCodeSpanne(t *testing.T) {
	// The backtick is inserted rather than written: a raw string in Go cannot
	// contain one.
	roh := strings.ReplaceAll(`[{"type":"paragraph","content":[
	 {"type":"text","text":"a @ b","styles":{"code":true}}]}]`, "@", "`")
	md := um(t, roh)
	enthaelt(t, md, "``a ` b``")
}

// Two blank lines in a code block are content. Turning them into one changes the
// exported code.
func TestLeerzeilenImCodeBleiben(t *testing.T) {
	md := um(t, `[{"type":"codeBlock","props":{"language":"go"},
	  "content":[{"type":"text","text":"a\n\n\nb"}]}]`)
	enthaelt(t, md, "a\n\n\nb")
}

func TestHarterUmbruchUeberlebt(t *testing.T) {
	md := um(t, `[{"type":"paragraph","content":[{"type":"text","text":"oben\nunten"}]}]`)
	enthaelt(t, md, "oben  \nunten")
}

func TestAdresseMitLeerzeichenWirdEingeklammert(t *testing.T) {
	md := um(t, `[{"type":"image","props":{"url":"/api/mein bild.png","name":"Bild"}}]`)
	enthaelt(t, md, "![Bild](</api/mein%20bild.png>)")
}

func TestKlapplisteWirdAufzaehlung(t *testing.T) {
	md := um(t, `[{"type":"toggleListItem","content":[{"type":"text","text":"Klappt"}]}]`)
	enthaelt(t, md, "- Klappt")
}
