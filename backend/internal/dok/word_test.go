package dok

import "testing"

// The round trip: write a document, read it back in and compare. Whatever does
// not arrive here is lost when editing a Word file; the test is the honest list
// of what the path carries.
func TestWordRundlauf(t *testing.T) {
	d := Dokument{
		Titel: "Bericht",
		Absatz: []Absatz{
			{Art: ArtUeberschrift, Stufe: 2, Text: []Stueck{{Text: "Abschnitt"}}},
			{Art: ArtAbsatz, Text: []Stueck{
				{Text: "Ein "},
				{Text: "fetter", Fett: true},
				{Text: " und ein "},
				{Text: "kursiver", Kursiv: true},
				{Text: " Teil."},
			}},
			{Art: ArtAufzaehlung, Text: []Stueck{{Text: "Erster Punkt"}}},
			{Art: ArtAufzaehlung, Text: []Stueck{{Text: "Zweiter Punkt"}}},
			{Art: ArtTabelle, Tabelle: [][]string{{"Kopf A", "Kopf B"}, {"eins", "zwei"}}},
		},
	}

	roh, err := Word(d)
	if err != nil {
		t.Fatalf("schreiben: %v", err)
	}
	zurueck, err := AusWord(roh)
	if err != nil {
		t.Fatalf("lesen: %v", err)
	}

	if zurueck.Titel != "Bericht" {
		t.Errorf("Titel: %q", zurueck.Titel)
	}

	var text string
	arten := map[Art]int{}
	for _, a := range zurueck.Absatz {
		arten[a.Art]++
		for _, s := range a.Text {
			text += s.Text
		}
		for _, z := range a.Tabelle {
			for _, c := range z {
				text += c + " "
			}
		}
	}
	for _, muss := range []string{"Abschnitt", "fetter", "kursiver", "Erster Punkt", "Kopf A", "zwei"} {
		if !contains(text, muss) {
			t.Errorf("%q fehlt nach dem Rundlauf", muss)
		}
	}
	if arten[ArtUeberschrift] < 1 {
		t.Error("keine Überschrift überlebt")
	}
	if arten[ArtTabelle] < 1 {
		t.Error("keine Tabelle überlebt")
	}

	// The styles have to hang on exactly the pieces they hung on, otherwise the
	// text is there but stressed in the wrong places.
	fettGefunden, kursivGefunden := false, false
	for _, a := range zurueck.Absatz {
		for _, s := range a.Text {
			if s.Fett && contains(s.Text, "fetter") {
				fettGefunden = true
			}
			if s.Kursiv && contains(s.Text, "kursiver") {
				kursivGefunden = true
			}
		}
	}
	if !fettGefunden {
		t.Error("fett ging verloren")
	}
	if !kursivGefunden {
		t.Error("kursiv ging verloren")
	}
}

// A broken file must not end in a panic but has to return an error; it comes
// from the network.
func TestWordKaputt(t *testing.T) {
	for _, roh := range [][]byte{nil, []byte("kein zip"), []byte("PK\x03\x04 aber sonst nichts")} {
		if _, err := AusWord(roh); err == nil {
			t.Errorf("kaputte Eingabe %q wurde angenommen", roh)
		}
	}
}

func contains(h, n string) bool { return len(n) > 0 && len(h) >= len(n) && indexOf(h, n) >= 0 }

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
