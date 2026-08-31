package puls

import (
	"sync"
	"testing"
	"time"
)

// Das Fach wird über die Uhr gewechselt, nicht über einen Zeitgeber. Der Fall,
// der dabei schiefgehen kann, ist ein Fach aus der vorigen Minute, das
// weitergezählt statt geleert wird: dann stünde eine alte Zahl als frische da.
func TestFachWirdGeleertStattWeitergezaehlt(t *testing.T) {
	m := Neu()
	f := &m.faecher[0]
	// Ein Fach, das zu einer Sekunde vor über einer Minute gehört.
	f.sekunde.Store(time.Now().Unix() - 3600)
	f.anfragen.Store(999)

	s := m.Lies()
	for _, sek := range s.Minute {
		if sek.Anfragen == 999 {
			t.Fatal("ein Fach aus einer früheren Minute wurde mitgezählt")
		}
	}
}

// naechsteSekunde wartet, bis die Uhr weitergesprungen ist.
//
// Nötig, weil Lies die laufende Sekunde auslässt: sie ist erst zum Teil
// vergangen, und eine halbe Sekunde sähe in der Anzeige wie ein Einbruch aus.
// Frisch Gezähltes taucht deshalb erst auf, wenn seine Sekunde vorbei ist.
func naechsteSekunde() {
	for start := time.Now().Unix(); time.Now().Unix() == start; {
		time.Sleep(5 * time.Millisecond)
	}
}

func TestZaehltUndUnterscheidetDenStatus(t *testing.T) {
	m := Neu()
	for _, status := range []int{200, 200, 404, 500} {
		m.Beginn()(status)
	}
	naechsteSekunde()
	s := m.Lies()
	if s.Gesamt != 4 {
		t.Fatalf("Gesamt = %d, erwartet 4", s.Gesamt)
	}
	// 4xx ist abgewiesen, 5xx ist kaputt. Die Unterscheidung ist der Grund,
	// warum überhaupt nach Status getrennt wird: eine Instanz, die fleißig 401
	// verteilt, ist nicht dieselbe wie eine, die abstürzt.
	if s.Abgelehnt != 1 {
		t.Fatalf("Abgelehnt = %d, erwartet 1", s.Abgelehnt)
	}
	if s.Fehler != 1 {
		t.Fatalf("Fehler = %d, erwartet 1", s.Fehler)
	}
}

func TestLaufendGehtWiederAufNull(t *testing.T) {
	m := Neu()
	ende := m.Beginn()
	if got := m.Lies().Laufend; got != 1 {
		t.Fatalf("Laufend = %d, erwartet 1", got)
	}
	ende(200)
	if got := m.Lies().Laufend; got != 0 {
		t.Fatalf("Laufend = %d nach dem Ende, erwartet 0", got)
	}
}

// Gemessen wird auf dem heißen Weg, also von allen Anfragen gleichzeitig. Unter
// -race fällt hier jede Sperre auf, die vergessen wurde.
func TestVieleGleichzeitig(t *testing.T) {
	m := Neu()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Beginn()(200)
			}
		}()
	}
	wg.Wait()
	if got := m.Lies().Gesamt; got != 5000 {
		t.Fatalf("Gesamt = %d, erwartet 5000", got)
	}
	if got := m.Lies().Laufend; got != 0 {
		t.Fatalf("Laufend = %d, erwartet 0", got)
	}
}

// Die laufende Sekunde bleibt draußen, weil sie erst zum Teil vergangen ist.
// Die Minute muss deshalb 59 Fächer haben und nicht 60.
func TestMinuteLaesstDieLaufendeSekundeAus(t *testing.T) {
	if got := len(Neu().Lies().Minute); got != Faecher-1 {
		t.Fatalf("Minute hat %d Fächer, erwartet %d", got, Faecher-1)
	}
}
