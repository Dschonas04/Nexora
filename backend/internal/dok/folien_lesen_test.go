package dok

import (
	"os"
	"strings"
	"testing"
)

// testdata/folien-probe.pptx is made with python-pptx from the default
// template: a title slide, a slide with bullets on two levels and a red bold
// run, and a slide with a picture and a table.
func TestFolienProbe(t *testing.T) {
	roh, err := os.ReadFile("testdata/folien-probe.pptx")
	if err != nil {
		t.Fatal(err)
	}
	p, err := AusFolien(roh)
	if err != nil {
		t.Fatal(err)
	}
	if p.Breite != 9144000 || p.Hoehe != 6858000 || len(p.Folien) != 3 {
		t.Fatalf("Größe %dx%d, %d Folien", p.Breite, p.Hoehe, len(p.Folien))
	}

	// Title slide: both placeholders take their position from the layout,
	// the title its size from the master, the subtitle is centred and has
	// no bullet.
	f := p.Folien[0]
	if len(f.Elemente) != 2 || f.Hintergrund != "#FFFFFF" {
		t.Fatalf("Folie 1: %+v", f)
	}
	titel, unter := f.Elemente[0], f.Elemente[1]
	if titel.B == 0 || titel.Absaetze[0].Laeufe[0].Text != "Nexora Probe" || titel.Absaetze[0].Laeufe[0].Groesse != 4400 {
		t.Fatalf("Titel: %+v", titel)
	}
	if unter.Absaetze[0].Ausrichtung != "ctr" || unter.Absaetze[0].Aufzaehlung != "" ||
		!strings.Contains(unter.Absaetze[0].Laeufe[0].Text, "äöü") {
		t.Fatalf("Untertitel: %+v", unter.Absaetze[0])
	}

	// Bullets.
	text := p.Folien[1].Elemente[1].Absaetze
	if len(text) != 3 || text[0].Aufzaehlung != "•" || text[1].Ebene != 1 {
		t.Fatalf("Punkte: %+v", text)
	}
	if l := text[2].Laeufe[0]; !l.Fett || l.Farbe != "#C00000" {
		t.Fatalf("Lauf: %+v", l)
	}

	// Picture and table.
	var bild, tab *Element
	for i, e := range p.Folien[2].Elemente {
		switch e.Art {
		case "bild":
			bild = &p.Folien[2].Elemente[i]
		case "tabelle":
			tab = &p.Folien[2].Elemente[i]
		}
	}
	if bild == nil || !strings.HasPrefix(bild.Bild, "data:image/png;base64,") || bild.X != 914400 {
		t.Fatalf("Bild: %+v", bild)
	}
	if tab == nil || len(tab.Zeilen) != 2 || tab.Zeilen[0][0] != "A" || tab.Zeilen[1][1] != "D" {
		t.Fatalf("Tabelle: %+v", tab)
	}
}

func TestFolienKeinPaket(t *testing.T) {
	if _, err := AusFolien([]byte("kein zip")); err == nil {
		t.Fatal("Fehler erwartet")
	}
	if _, err := AusFolien(paket(t, map[string]string{"x.xml": "<x/>"})); err == nil {
		t.Fatal("Fehler erwartet")
	}
}
