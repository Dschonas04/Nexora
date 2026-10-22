package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFaelligAus(t *testing.T) {
	faelle := map[string]string{
		"Angebot schicken 📅 2026-10-30":  "2026-10-30",
		"Angebot schicken 📅2026-10-30":   "2026-10-30",
		"Angebot schicken 📅 30.10.2026":  "2026-10-30",
		"Angebot schicken 📅 3.1.2027":    "2027-01-03",
		"Angebot schicken 📅 2026-13-40":  "",
		"Angebot schicken am 2026-10-30": "",
		"ohne alles":                     "",
	}
	for text, soll := range faelle {
		if ist := faelligAus(text); ist != soll {
			t.Errorf("faelligAus(%q) = %q, soll %q", text, ist, soll)
		}
	}
}

func TestPersonenAus(t *testing.T) {
	ist := personenAus("@Anna und @bert.k schauen drauf, cc @anna. Mail an x@y.de")
	soll := []string{"anna", "bert.k"}
	if !reflect.DeepEqual(ist, soll) {
		t.Errorf("personenAus = %v, soll %v", ist, soll)
	}
}

const aufgabenDok = `[
 {"id":"a","type":"heading","content":[{"type":"text","text":"Plan","styles":{}}],"children":[]},
 {"id":"b","type":"checkListItem","props":{"checked":false},
  "content":[{"type":"text","text":"Angebot @anna ","styles":{}},
             {"type":"link","href":"x","content":[{"type":"text","text":"📅 2026-11-02","styles":{}}]}],
  "children":[
   {"id":"c","type":"checkListItem","props":{"checked":true},"content":[{"type":"text","text":"Entwurf","styles":{}}],"children":[]}
  ]},
 {"id":"d","type":"checkListItem","props":{"checked":false},"content":[],"children":[]}
]`

func TestAufgabenAus(t *testing.T) {
	liste := aufgabenAus(json.RawMessage(aufgabenDok))
	if len(liste) != 2 {
		t.Fatalf("2 Aufgaben erwartet (die leere zählt nicht), %d gefunden: %+v", len(liste), liste)
	}
	if liste[0].BlockID != "b" || liste[0].Faellig != "2026-11-02" || !reflect.DeepEqual(liste[0].Personen, []string{"anna"}) {
		t.Errorf("erste Aufgabe falsch gelesen: %+v", liste[0])
	}
	if liste[1].BlockID != "c" || !liste[1].Erledigt {
		t.Errorf("verschachtelte Aufgabe falsch gelesen: %+v", liste[1])
	}
}

func TestSetzeHaken(t *testing.T) {
	neu, ok := setzeHaken(json.RawMessage(aufgabenDok), "c", false)
	if !ok {
		t.Fatal("Block c nicht gefunden")
	}
	for _, a := range aufgabenAus(neu) {
		if a.BlockID == "c" && a.Erledigt {
			t.Error("c ist noch abgehakt")
		}
	}
	if _, ok := setzeHaken(json.RawMessage(aufgabenDok), "a", true); ok {
		t.Error("eine Überschrift ist keine Aufgabe")
	}
	if _, ok := setzeHaken(json.RawMessage(aufgabenDok), "zz", true); ok {
		t.Error("ein fehlender Block darf nicht als gefunden gelten")
	}
}

func TestZielGueltig(t *testing.T) {
	for roh, soll := range map[string]bool{
		"https://n8n.example/webhook/abc": true,
		"http://10.0.5.60:5678/webhook/x": true,
		"ftp://x":                         false,
		"javascript:alert(1)":             false,
		"https://":                        false,
	} {
		if _, ok := zielGueltig(roh); ok != soll {
			t.Errorf("zielGueltig(%q) = %v, soll %v", roh, ok, soll)
		}
	}
}

func TestEreignisseBereinigt(t *testing.T) {
	ist := ereignisseBereinigt([]string{"page.updated", "erfunden", "comment.created"})
	if !reflect.DeepEqual(ist, []string{"page.updated", "comment.created"}) {
		t.Errorf("ereignisseBereinigt = %v", ist)
	}
}
