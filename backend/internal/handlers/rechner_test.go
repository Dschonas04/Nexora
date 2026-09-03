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

// Eine Adresse ohne Port wird abgewiesen und nicht geraten: ein geratener Port
// meldet einen Rechner als still, der nur auf einem anderen hoert.
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

// Anklopfen misst an einem offenen Port und an einem geschlossenen, damit beide
// Zweige einmal wirklich gelaufen sind.
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

	// Ein Port, auf dem nichts horcht: derselbe Horcher, nachdem er zu ist.
	zu := horcher.Addr().String()
	horcher.Close()
	if m := anklopfen(context.Background(), zu); m.Da {
		t.Fatal("geschlossener Port gilt als erreichbar")
	}
}

// Eine HTTP-Adresse zaehlt als erreichbar, auch wenn der Weg 404 gibt: gefragt
// ist, ob dort ein Dienst laeuft, nicht ob er diesen einen Pfad kennt.
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

// Ein selbst unterschriebenes Zertifikat ist im eigenen Haus die Regel und darf
// den Rechner nicht als still melden: Proxmox, NAS und Backup-Server tragen
// alle eines.
func TestAnklopfenNimmtSelbstUnterschriebenesZertifikat(t *testing.T) {
	dienst := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer dienst.Close()

	if m := anklopfen(context.Background(), dienst.URL); !m.Da {
		t.Fatalf("selbst unterschriebenes HTTPS gilt als still: %s", m.Hinweis)
	}
}

// Einer Umleitung wird nicht gefolgt: sonst stuende am Ende die Erreichbarkeit
// eines ganz anderen Rechners in der Zeile.
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

// Ein Dienst, der sich beim Verbindungsaufbau vorstellt -- wie es jeder
// SSH-Dienst tut --, landet mit seiner Kennung in der Spalte. Das ist die
// Antwort auf "welche Fassung laeuft da", ohne Anmeldung und ohne fremde Hilfe.
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

// Wer nichts sagt, sagt nichts: die Spalte bleibt leer, und die Messung haengt
// nicht in der Frist fest.
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
			// Annehmen und schweigen, wie es etwa eine Datenbank tut.
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

// Die Kopfzeile Server ist die Fassung, die ein Webdienst selbst nennt, und das
// Zertifikat sagt, wie lange es noch gilt.
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

// Was ein fremder Rechner schickt, gehoert beschnitten, bevor es in einer
// Oberflaeche steht: eine Zeile, nur Druckbares, hoechstens sechzig Zeichen.
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

// Ein abgelaufenes Zertifikat ist der haeufigste Grund, warum ein Dienst im
// eigenen Haus ploetzlich nicht mehr erreichbar ist. Die Zahl steht daneben,
// damit die Oberflaeche einfaerben kann, ohne Text auszuwerten.
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
