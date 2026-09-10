package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// An address without a port is rejected and not guessed: a guessed port
// reports a machine as silent that merely listens on a different one.
func TestZielPruefen(t *testing.T) {
	faelle := []struct {
		wert string
		gut  bool
	}{
		{"10.0.0.5:22", true},
		{"nas.fritz.box:445", true},
		{"http://10.0.0.5:9090", true},
		{"https://beispiel.de", true},
		{"10.0.0.5", false},
		{"", false},
		{"   ", false},
		{"10.0.0.5:0", false},
		{"10.0.0.5:70000", false},
		{"10.0.0.5:ssh", false},
		{"ssh://10.0.0.5:22", false},
	}
	for _, f := range faelle {
		_, err := zielPruefen(f.wert)
		if f.gut && err != nil {
			t.Errorf("%q sollte durchgehen, kam: %v", f.wert, err)
		}
		if !f.gut && err == nil {
			t.Errorf("%q sollte abgewiesen werden", f.wert)
		}
	}
}

// Knocking is measured against an open port and a closed one, so that both
// branches have really run once.
func TestAnklopfen(t *testing.T) {
	horcher, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer horcher.Close()
	go func() {
		for {
			c, err := horcher.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	if m := anklopfen(context.Background(), horcher.Addr().String()); !m.Da {
		t.Fatalf("offener Port gilt als still: %s", m.Hinweis)
	}

	// A port nothing listens on: the same listener, after it has closed.
	zu := horcher.Addr().String()
	horcher.Close()
	if m := anklopfen(context.Background(), zu); m.Da {
		t.Fatal("geschlossener Port gilt als erreichbar")
	}
}

// An HTTP address counts as reachable even when the path answers 404: the
// question is whether a service runs there, not whether it knows this one path.
func TestAnklopfenHTTPMitFehlerstatus(t *testing.T) {
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer dienst.Close()

	m := anklopfen(context.Background(), dienst.URL)
	if !m.Da {
		t.Fatal("404 gilt als still")
	}
	if m.Hinweis == "" {
		t.Fatal("der Status fehlt im Hinweis")
	}
}

// A self-signed certificate is the rule inside one's own house and must not
// report the machine as silent: Proxmox, NAS and backup server all carry one.
func TestAnklopfenNimmtSelbstUnterschriebenesZertifikat(t *testing.T) {
	dienst := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer dienst.Close()

	if m := anklopfen(context.Background(), dienst.URL); !m.Da {
		t.Fatalf("selbst unterschriebenes HTTPS gilt als still: %s", m.Hinweis)
	}
}

// A redirect is not followed: otherwise the row would end up stating the
// reachability of a completely different machine.
func TestAnklopfenFolgtKeinerUmleitung(t *testing.T) {
	besucht := 0
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		besucht++
		http.Redirect(w, r, "/woanders", http.StatusMovedPermanently)
	}))
	defer dienst.Close()

	m := anklopfen(context.Background(), dienst.URL)
	if !m.Da {
		t.Fatal("eine Umleitung ist auch eine Antwort")
	}
	if besucht != 1 {
		t.Fatalf("der Umleitung wurde gefolgt, %d Aufrufe", besucht)
	}
}

// A service that introduces itself as the connection opens -- as every SSH
// daemon does -- lands in the column with its banner. That is the answer to
// "which version runs there", without signing in and without outside help.
func TestAnklopfenLiestDieBegruessung(t *testing.T) {
	horcher, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer horcher.Close()
	go func() {
		c, err := horcher.Accept()
		if err != nil {
			return
		}
		c.Write([]byte("SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u6\r\n"))
		c.Close()
	}()

	m := anklopfen(context.Background(), horcher.Addr().String())
	if !m.Da {
		t.Fatal("Dienst gilt als still")
	}
	if m.Fassung != "SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u6" {
		t.Fatalf("Kennung falsch gelesen: %q", m.Fassung)
	}
}

// Whoever says nothing says nothing: the column stays empty, and the
// measurement does not hang until the deadline.
func TestStillerPortHatKeineKennung(t *testing.T) {
	horcher, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer horcher.Close()
	go func() {
		for {
			c, err := horcher.Accept()
			if err != nil {
				return
			}
			// Accept and stay silent, the way a database does.
			defer c.Close()
		}
	}()

	beginn := time.Now()
	m := anklopfen(context.Background(), horcher.Addr().String())
	if !m.Da || m.Fassung != "" {
		t.Fatalf("erwartet erreichbar ohne Kennung, bekam %+v", m)
	}
	if time.Since(beginn) > 2*time.Second {
		t.Fatalf("zu lange gewartet: %s", time.Since(beginn))
	}
}

// The Server header is the version a web service names itself, and the
// certificate says how much longer it is valid.
func TestAnklopfenLiestServerUndZertifikat(t *testing.T) {
	dienst := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "nginx/1.27.4")
		w.WriteHeader(http.StatusOK)
	}))
	defer dienst.Close()

	m := anklopfen(context.Background(), dienst.URL)
	if m.Fassung != "nginx/1.27.4" {
		t.Fatalf("Kennung falsch: %q", m.Fassung)
	}
	if m.Zertifikat == "" || m.Tage == nil {
		t.Fatalf("kein Zertifikat gelesen: %+v", m)
	}
}

// Whatever a foreign machine sends belongs trimmed before it appears in an
// interface: one line, printable characters only, sixty at the most.
func TestKurzeKennungBeschneidet(t *testing.T) {
	if raus := kurzeKennung("SSH-2.0-OpenSSH_9.2\r\nzweite Zeile"); raus != "SSH-2.0-OpenSSH_9.2" {
		t.Fatalf("zweite Zeile nicht abgeschnitten: %q", raus)
	}
	if raus := kurzeKennung("mit\x00Steuer\x07zeichen"); raus != "mitSteuerzeichen" {
		t.Fatalf("Steuerzeichen blieben stehen: %q", raus)
	}
	lang := strings.Repeat("x", 200)
	if raus := kurzeKennung(lang); len(raus) != 60 {
		t.Fatalf("nicht auf 60 gekuerzt, sondern %d", len(raus))
	}
	if raus := kurzeKennung("   "); raus != "" {
		t.Fatalf("Leerraum ergibt keine Kennung, bekam %q", raus)
	}
}

// An expired certificate is the most frequent reason a service in one's own
// house suddenly stops being reachable. The number sits beside it so the
// interface can colour the row without parsing text.
func TestZertifikatsAlter(t *testing.T) {
	text, tage := zertifikatsAlter(time.Now().Add(48 * time.Hour))
	if tage == nil || *tage != 1 || text == "" {
		t.Fatalf("zwei Tage ergaben %q / %v", text, tage)
	}
	text, tage = zertifikatsAlter(time.Now().Add(-24 * time.Hour))
	if tage == nil || *tage >= 0 || text != "abgelaufen" {
		t.Fatalf("abgelaufen ergab %q / %v", text, tage)
	}
}
