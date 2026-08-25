package handlers

import (
	"testing"
	"time"
)

// Zeitspannen sollen so dastehen, wie man sie vorliest. Der Fall, der den Test
// ausgelöst hat: eine Laufzeit von 48 Sekunden stand als "48023 ms" da.
func TestDauerLiestSichVor(t *testing.T) {
	faelle := []struct {
		d    time.Duration
		will string
	}{
		{400 * time.Microsecond, "0.4 ms"},
		{12 * time.Millisecond, "12 ms"},
		{48 * time.Second, "48 s"},
		{5 * time.Minute, "5 min"},
		{3 * time.Hour, "3 h"},
		{100 * time.Hour, "4 Tagen"},
	}
	for _, f := range faelle {
		if got := dauer(f.d); got != f.will {
			t.Errorf("dauer(%v) = %q, erwartet %q", f.d, got, f.will)
		}
	}
}
