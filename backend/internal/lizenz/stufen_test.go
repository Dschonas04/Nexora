package lizenz

import "testing"

// Die Stufen bauen aufeinander auf. Wer Business kauft, bekommt alles aus Pro
// mit, eine Stufe, die etwas Kleineres nicht enthält, wäre eine Falle beim
// Wechsel.
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

// Jede Funktion muss in genau einer Stufe zum ersten Mal auftauchen. Eine, die
// in keiner steht, wäre unverkäuflich; eine, die in zweien neu ist, wäre ein
// Kopierfehler.
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

// Business ist die oberste Stufe und muss deshalb alles enthalten.
func TestBusinessEnthaeltAlles(t *testing.T) {
	if len(FunktionenDerStufe(StufeBusiness)) != len(Alle) {
		t.Errorf("Business enthält %d von %d Funktionen",
			len(FunktionenDerStufe(StufeBusiness)), len(Alle))
	}
}

// Ein Tippfehler im Schlüssel darf nicht zufällig etwas freischalten.
func TestUnbekannteStufeSchaltetNichtsFrei(t *testing.T) {
	if got := FunktionenDerStufe("gold"); len(got) != 0 {
		t.Errorf("unbekannte Stufe liefert %v", got)
	}
	if StufeGueltig("gold") {
		t.Error("gold gilt als bekannte Stufe")
	}
}
