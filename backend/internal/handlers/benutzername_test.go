package handlers

import "testing"

// Was durchgeht und was nicht. Der Name wird getippt und vorgelesen, deshalb
// sind Grossbuchstaben keine Ablehnung, sondern werden geglaettet.
func TestBenutzernamePruefen(t *testing.T) {
	faelle := []struct {
		eingabe string
		raus    string
		fehler  bool
	}{
		{"anna", "anna", false},
		{"  Anna  ", "anna", false},
		{"anna.mueller", "anna.mueller", false},
		{"a_b-c9", "a_b-c9", false},
		{"", "", false},
		{"an", "", true},
		{"anna@example.com", "", true},
		{"anna müller", "", true},
		{".anna", "", true},
		{"anna/pfad", "", true},
	}
	for _, f := range faelle {
		raus, err := benutzernamePruefen(f.eingabe)
		if f.fehler && err == nil {
			t.Errorf("%q hätte abgelehnt werden müssen", f.eingabe)
			continue
		}
		if !f.fehler && err != nil {
			t.Errorf("%q wurde abgelehnt: %v", f.eingabe, err)
			continue
		}
		if !f.fehler && raus != f.raus {
			t.Errorf("%q wurde zu %q, erwartet %q", f.eingabe, raus, f.raus)
		}
	}
}

// Der Vorschlag aus der Adresse nimmt den vorderen Teil und wirft heraus, was
// im Namen nichts zu suchen hat. Bleibt zu wenig übrig, gibt es keinen -- ein
// zurechtgestutzter Rest wäre schlechter als gar keiner.
func TestBenutzernameAusAdresse(t *testing.T) {
	faelle := map[string]string{
		"anna.mueller@example.com": "anna.mueller",
		"Anna+Werbung@example.com": "annawerbung",
		"a@example.com":            "",
		"ä@example.com":            "",
		".anna@example.com":        "anna",
	}
	for adresse, erwartet := range faelle {
		if raus := benutzernameAusAdresse(adresse); raus != erwartet {
			t.Errorf("%s wurde zu %q, erwartet %q", adresse, raus, erwartet)
		}
	}
}

// Der leere Name muss als NULL in die Datenbank: der eindeutige Index lässt
// beliebig viele NULL zu, aber nur einen leeren String.
func TestLeererNameWirdNull(t *testing.T) {
	if leerAlsNull("") != nil {
		t.Fatal("der leere Name muss NULL werden")
	}
	if leerAlsNull("anna") != any("anna") {
		t.Fatal("ein gesetzter Name muss so bleiben, wie er ist")
	}
}
