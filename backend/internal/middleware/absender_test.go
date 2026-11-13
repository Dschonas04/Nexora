package middleware

import "testing"

func TestEchteIP(t *testing.T) {
	faelle := []struct {
		name   string
		remote string
		xff    []string
		soll   string
	}{
		{"Kette über drei Proxys", "172.18.0.1:5000", []string{"203.0.113.7, 10.0.2.6, 172.18.0.1"}, "203.0.113.7"},
		{"gefälschter Eintrag links", "172.18.0.1:5000", []string{"1.2.3.4, 203.0.113.7, 10.0.2.6"}, "203.0.113.7"},
		{"Besucher im LAN", "172.18.0.1:5000", []string{"10.0.0.5, 10.0.2.6"}, "10.0.0.5"},
		{"direkt von außen", "198.51.100.9:443", []string{"1.2.3.4"}, ""},
		{"ohne Kopfzeile", "172.18.0.1:5000", nil, "172.18.0.1"},
		{"Müll im Kopf", "172.18.0.1:5000", []string{"1.2.3.4, quatsch, 10.0.2.6"}, "10.0.2.6"},
		{"mehrere Kopfzeilen", "127.0.0.1:1", []string{"1.2.3.4", "203.0.113.7, 10.0.2.6"}, "203.0.113.7"},
		{"IPv6", "[::1]:1", []string{"2001:db8::1, 10.0.2.6"}, "2001:db8::1"},
	}
	for _, f := range faelle {
		if ist := echteIP(f.remote, f.xff); ist != f.soll {
			t.Errorf("%s: %q, erwartet %q", f.name, ist, f.soll)
		}
	}
}
