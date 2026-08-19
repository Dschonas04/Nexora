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
	// Fett UND Code: die Backticks müssen innen liegen, sonst steht der Stern
	// im Code und wird nicht mehr als Auszeichnung gelesen.
	md := um(t, `[{"type":"paragraph","content":[
	 {"type":"text","text":"x","styles":{"bold":true,"code":true}}]}]`)
	enthaelt(t, md, "**`x`**")
}

func TestLeerzeichenBleibenAussen(t *testing.T) {
	// "** fett **" stellt kein Leser als fett dar. Die Leerzeichen gehören
	// vor und hinter die Sterne.
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
	// Nach dem Absatz beginnt eine neue Liste -- sie muss wieder bei 1 anfangen.
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
	// Ein Typ, den diese Fassung nicht kennt, darf seinen Text nicht
	// verschlucken -- unvollständig exportieren ist besser als verlieren.
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
	// Auch bei abweichender Schreibweise -- sonst stünde der Titel doppelt.
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
