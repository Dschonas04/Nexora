package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

// Two identically named files must not overwrite each other in the archive,
// and the number belongs before the extension: "Plan.png-2" would not be a
// valid image filename for many programs.
func TestEindeutigerDateinameZaehltVorDerEndung(t *testing.T) {
	vergeben := map[string]int{}
	if got := eindeutigerDateiname(vergeben, "Plan.png"); got != "Plan.png" {
		t.Fatalf("erster Name: %q", got)
	}
	if got := eindeutigerDateiname(vergeben, "Plan.png"); got != "Plan-2.png" {
		t.Fatalf("zweiter Name: %q", got)
	}
	if got := eindeutigerDateiname(vergeben, "Plan.png"); got != "Plan-3.png" {
		t.Fatalf("dritter Name: %q", got)
	}
}

// An attachment URL in the archive should point to the nearby file. Without
// this the produced Markdown would contain references that only make sense
// inside this instance.
func TestAdressenAufDateienZeigenInsArchiv(t *testing.T) {
	dateien := []exportDatei{{
		ID:    "b1b1b1b1-0000-0000-0000-000000000001",
		Seite: "aaaa1111-0000-0000-0000-000000000001",
		Name:  "Der Plan.png",
		Pfad:  "dateien/Der Plan.png",
	}}
	md := "![Plan](/api/pages/aaaa1111-0000-0000-0000-000000000001/attachments/b1b1b1b1-0000-0000-0000-000000000001)"
	got := adressenAufDateien(md, dateien)
	if got != "![Plan](<dateien/Der Plan.png>)" {
		t.Fatalf("umgeschrieben: %q", got)
	}
}

// An attachment not referenced in the text is listed below. Otherwise it
// would sit in the archive and never be found.
func TestAnhangListeNenntNurWasFehlt(t *testing.T) {
	dateien := []exportDatei{
		{Name: "Im Text.png", Pfad: "dateien/Im Text.png"},
		{Name: "Daneben.pdf", Pfad: "dateien/Daneben.pdf"},
	}
	md := "# Seite\n\n![x](<dateien/Im Text.png>)\n"
	got := anhangListe(md, dateien)
	if !strings.Contains(got, "## Anhänge") {
		t.Fatal("keine Anhangliste")
	}
	if !strings.Contains(got, "dateien/Daneben.pdf") {
		t.Fatal("die unverknüpfte Datei fehlt")
	}
	if strings.Count(got, "dateien/Im Text.png") != 1 {
		t.Fatal("die verknüpfte Datei steht doppelt")
	}
}

// With no attachments the text remains unchanged: an empty "Attachments"
// heading under every page would be noise.
func TestAnhangListeOhneDateien(t *testing.T) {
	md := "# Seite\n"
	if got := anhangListe(md, nil); got != md {
		t.Fatalf("verändert: %q", got)
	}
}

// The index reflects the page tree with indentation, derived from parent_id
// rather than the query order.
func TestVerzeichnisZeigtDenBaum(t *testing.T) {
	mutter := "aaaa1111-0000-0000-0000-000000000001"
	fremd := "cccc3333-0000-0000-0000-000000000003"
	seiten := []exportSeite{
		{ID: "bbbb2222-0000-0000-0000-000000000002", ParentID: &mutter, Titel: "Kind"},
		{ID: mutter, Titel: "Mutter"},
		{ID: "dddd4444-0000-0000-0000-000000000004", ParentID: &fremd, Titel: "Waise"},
	}
	namen := map[string]string{
		mutter:                                 "Mutter.md",
		"bbbb2222-0000-0000-0000-000000000002": "Kind.md",
		"dddd4444-0000-0000-0000-000000000004": "Waise.md",
	}
	var b strings.Builder
	schreibeVerzeichnis(&b, seiten, namen)
	got := b.String()

	if !strings.Contains(got, "- [Mutter](<Mutter.md>)\n  - [Kind](<Kind.md>)") {
		t.Fatalf("Baum nicht eingerückt:\n%s", got)
	}
	// Die Mutter der Waise liegt ausserhalb dieser Ausgabe. Sie steht trotzdem
	// da, nur eben oben: fehlen darf sie nicht.
	if !strings.Contains(got, "- [Waise](<Waise.md>)") {
		t.Fatalf("Seite ohne ausgegebene Mutter fehlt:\n%s", got)
	}
}

// Images of a shared page are rewritten to the public path. Without this a
// visitor without an account would see broken images.
func TestAdressenOeffnenSchreibtAufDenOeffentlichenWeg(t *testing.T) {
	seite := "aaaa1111-0000-0000-0000-000000000001"
	inhalt := json.RawMessage(`[{"type":"image","props":{"url":"/api/pages/` + seite + `/attachments/b1"}}]`)
	got := string(adressenOeffnen(inhalt, seite, "zeichen"))
	if !strings.Contains(got, "/api/public/zeichen/dateien/b1") {
		t.Fatalf("nicht umgeschrieben: %s", got)
	}
	if strings.Contains(got, seite) {
		t.Fatalf("die Kennung der Seite steht noch drin: %s", got)
	}
}

// An attachment of ANOTHER page remains locked. Exposing it would amount to
// implicitly sharing a second page that is not referenced.
func TestAdressenOeffnenLaesstFremdeSeitenZu(t *testing.T) {
	inhalt := json.RawMessage(`[{"type":"image","props":{"url":"/api/pages/ffff9999-0000-0000-0000-000000000009/attachments/b1"}}]`)
	got := string(adressenOeffnen(inhalt, "aaaa1111-0000-0000-0000-000000000001", "zeichen"))
	if strings.Contains(got, "/api/public/") {
		t.Fatalf("fremde Adresse geöffnet: %s", got)
	}
}
