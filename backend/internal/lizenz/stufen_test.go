package lizenz

import "testing"

// The tiers build on one another. Whoever buys Business gets everything from Pro
// along with it; a tier not containing something smaller would be a trap when
// switching.
func TestStufenBauenAufeinanderAuf(t *testing.T) {
	enthalten := func(st Stufe) map[Funktion]bool {
		m := map[Funktion]bool{}
		for _, f := range FunktionenDerStufe(st) {
			m[f] = true
		}
		return m
	}
	for i := 1; i < len(StufenReihe); i++ {
		kleiner := enthalten(StufenReihe[i-1])
		groesser := enthalten(StufenReihe[i])
		for f := range kleiner {
			if !groesser[f] {
				t.Errorf("%s enthält %q nicht, %s aber schon", StufenReihe[i], f, StufenReihe[i-1])
			}
		}
	}
}

// Every feature has to appear for the first time in exactly one tier. One that
// stands in none would be unsellable; one that is new in two would be a copying
// mistake.
func TestJedeFunktionGehoertZuGenauEinerStufe(t *testing.T) {
	woher := map[Funktion][]Stufe{}
	for _, st := range StufenReihe {
		for _, f := range stufenZusatz[st] {
			woher[f] = append(woher[f], st)
		}
	}
	for _, f := range Alle {
		switch len(woher[f]) {
		case 1:
		case 0:
			t.Errorf("%q gehört zu keiner Stufe", f)
		default:
			t.Errorf("%q steht in mehreren Stufen: %v", f, woher[f])
		}
	}
	for f := range woher {
		bekannt := false
		for _, a := range Alle {
			if a == f {
				bekannt = true
			}
		}
		if !bekannt {
			t.Errorf("Stufe nennt die unbekannte Funktion %q", f)
		}
	}
}

// Business is the top tier and therefore has to contain everything.
func TestBusinessEnthaeltAlles(t *testing.T) {
	if len(FunktionenDerStufe(StufeBusiness)) != len(Alle) {
		t.Errorf("Business enthält %d von %d Funktionen",
			len(FunktionenDerStufe(StufeBusiness)), len(Alle))
	}
}

// A typo in a key must not unlock anything by accident.
func TestUnbekannteStufeSchaltetNichtsFrei(t *testing.T) {
	if got := FunktionenDerStufe("gold"); len(got) != 0 {
		t.Errorf("unbekannte Stufe liefert %v", got)
	}
	if StufeGueltig("gold") {
		t.Error("gold gilt als bekannte Stufe")
	}
}
