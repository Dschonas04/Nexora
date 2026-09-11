package puls

import (
	"sync"
	"testing"
	"time"
)

// The slot is switched by the clock, not by a timer. The case that can go
// wrong there is a slot from the previous minute that is counted on instead of
// cleared: an old number would then stand there as a fresh one.
func TestFachWirdGeleertStattWeitergezaehlt(t *testing.T) {
	m := Neu()
	f := &m.faecher[0]
	// A slot belonging to a second more than a minute ago.
	f.sekunde.Store(time.Now().Unix() - 3600)
	f.anfragen.Store(999)

	s := m.Lies()
	for _, sek := range s.Minute {
		if sek.Anfragen == 999 {
			t.Fatal("ein Fach aus einer früheren Minute wurde mitgezählt")
		}
	}
}

// naechsteSekunde waits until the clock has moved on.
//
// Necessary because Lies leaves out the current second: it has only partly
// passed, and half a second would look like a slump in the display. Freshly
// counted things therefore only show up once their second is over.
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
	// 4xx is rejected, 5xx is broken. That distinction is the reason for
	// separating by status at all: an instance busily handing out 401s is not
	// the same as one that crashes.
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

// Measuring happens on the hot path, that is from all requests at once. Under
// -race every lock that was forgotten shows up here.
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

// The current second stays out because it has only partly passed. The minute
// must therefore have 59 slots and not 60.
func TestMinuteLaesstDieLaufendeSekundeAus(t *testing.T) {
	if got := len(Neu().Lies().Minute); got != Faecher-1 {
		t.Fatalf("Minute hat %d Fächer, erwartet %d", got, Faecher-1)
	}
}
