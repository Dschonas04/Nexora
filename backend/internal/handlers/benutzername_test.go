package handlers

import "testing"

// What passes and what doesn't. The name is typed and read aloud, so
// uppercase letters are not rejected but normalized.
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

// The suggestion derived from an address takes the local part and removes
// characters not allowed in a username. If too little remains no suggestion
// is returned — a trimmed remainder would be worse than none.
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

// The empty name must become NULL in the database: the unique index allows
// many NULLs but only a single empty string.
func TestLeererNameWirdNull(t *testing.T) {
	if leerAlsNull("") != nil {
		t.Fatal("der leere Name muss NULL werden")
	}
	if leerAlsNull("anna") != any("anna") {
		t.Fatal("ein gesetzter Name muss so bleiben, wie er ist")
	}
}
