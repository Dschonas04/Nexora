package handlers

import "testing"

func gleich(t *testing.T, was string, bekam, erwartet []string) {
	t.Helper()
	if len(bekam) != len(erwartet) {
		t.Fatalf("%s: %v, erwartet %v", was, bekam, erwartet)
	}
	for i := range bekam {
		if bekam[i] != erwartet[i] {
			t.Fatalf("%s: %v, erwartet %v", was, bekam, erwartet)
		}
	}
}

func vor(s string) *string { return &s }

func TestEinsortierenSetztDavor(t *testing.T) {
	liste := []string{"a", "b", "c", "d"}
	gleich(t, "d vor b", einsortieren(liste, "d", vor("b")), []string{"a", "d", "b", "c"})
	gleich(t, "a vor c", einsortieren(liste, "a", vor("c")), []string{"b", "a", "c", "d"})
	gleich(t, "c ganz nach vorn", einsortieren(liste, "c", vor("a")), []string{"c", "a", "b", "d"})
}

func TestEinsortierenOhneZielAnsEnde(t *testing.T) {
	liste := []string{"a", "b", "c"}
	gleich(t, "ohne Ziel", einsortieren(liste, "a", nil), []string{"b", "c", "a"})
	// An unknown target means the end as well. A drop below the last entry must
	// not fall through as an error while the page hangs in its new place already.
	gleich(t, "unbekanntes Ziel", einsortieren(liste, "a", vor("weg")), []string{"b", "c", "a"})
	gleich(t, "leeres Ziel", einsortieren(liste, "b", vor("")), []string{"a", "c", "b"})
}

func TestEinsortierenNeuerEintrag(t *testing.T) {
	// The page comes from another level and is not in the list yet.
	gleich(t, "neu vor b", einsortieren([]string{"a", "b"}, "x", vor("b")), []string{"a", "x", "b"})
	gleich(t, "in eine leere Ebene", einsortieren(nil, "x", nil), []string{"x"})
}

func TestEinsortierenVorSichSelbst(t *testing.T) {
	// Dropping a page into the gap it already stands in changes nothing. Without
	// the guard the entry would be taken out, looked for in vain and land at the
	// end -- a gesture meaning "leave it" would move the page.
	gleich(t, "vor sich selbst", einsortieren([]string{"a", "b", "c"}, "b", vor("b")),
		[]string{"a", "b", "c"})
}
