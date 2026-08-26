package handlers

import (
	"testing"
	"time"
)

// Durations shall stand there the way one reads them out. The case that
// prompted the test: a runtime of 48 seconds stood there as "48023 ms".
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
