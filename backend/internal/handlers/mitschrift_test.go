package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"nexora/internal/middleware"
	"nexora/internal/puls"
)

func neuerSitzer(platz int) *sitzer {
	return &sitzer{post: make(chan []byte, platz)}
}

// A packet is sent to everyone in the room, including the sender. The echo
// is intentional: it is the heartbeat for someone who is alone on a page.
func TestVerteilenErreichtAlleSamtAbsender(t *testing.T) {
	a, b := neuerSitzer(4), neuerSitzer(4)
	if !betreten("s1", a) || !betreten("s1", b) {
		t.Fatal("betreten fehlgeschlagen")
	}
	defer func() { verlassen("s1", a); verlassen("s1", b) }()

	verteilen("s1", []byte("hallo"))
	for name, s := range map[string]*sitzer{"a": a, "b": b} {
		select {
		case p := <-s.post:
			if string(p) != "hallo" {
				t.Fatalf("%s bekam %q", name, p)
			}
		default:
			t.Fatalf("%s bekam nichts", name)
		}
	}
}

// Rooms are isolated: what is written on one page must not appear on another.
func TestVerteilenBleibtImRaum(t *testing.T) {
	a, b := neuerSitzer(4), neuerSitzer(4)
	betreten("s1", a)
	betreten("s2", b)
	defer func() { verlassen("s1", a); verlassen("s2", b) }()

	verteilen("s1", []byte("x"))
	if len(b.post) != 0 {
		t.Fatal("Paket ist in den falschen Raum gelaufen")
	}
}

// A client that does not consume messages is kicked out rather than slowing
// everyone else down.
func TestStauWirftHinaus(t *testing.T) {
	langsam := neuerSitzer(1)
	flott := neuerSitzer(8)
	betreten("s3", langsam)
	betreten("s3", flott)
	defer func() { verlassen("s3", langsam); verlassen("s3", flott) }()

	verteilen("s3", []byte("1")) // füllt den Kanal des Langsamen
	verteilen("s3", []byte("2")) // dafür ist kein Platz mehr

	if ImRaum("s3") != 1 {
		t.Fatalf("erwartet: nur noch einer im Raum, ist: %d", ImRaum("s3"))
	}
	// Sein Kanal ist zu, sein Schreiber beendet sich von selbst.
	<-langsam.post
	if _, offen := <-langsam.post; offen {
		t.Fatal("Kanal des Hinausgeworfenen ist noch offen")
	}
	// Der Flotte hat beide bekommen.
	if len(flott.post) != 2 {
		t.Fatalf("der Flotte hat %d von 2 Paketen", len(flott.post))
	}
}

// Empty rooms are pruned. Otherwise the map would grow with every opened
// page and never shrink.
func TestLeererRaumVerschwindet(t *testing.T) {
	a := neuerSitzer(2)
	betreten("s4", a)
	verlassen("s4", a)

	raeume.Lock()
	_, da := raeume.nach["s4"]
	raeume.Unlock()
	if da {
		t.Fatal("leerer Raum steht noch")
	}
}

// No more than the capacity may enter.
func TestRaumIstBegrenzt(t *testing.T) {
	var drin []*sitzer
	for i := 0; i < hoechstensImRaum; i++ {
		s := neuerSitzer(1)
		if !betreten("s5", s) {
			t.Fatalf("Platz %d wurde abgewiesen", i)
		}
		drin = append(drin, s)
	}
	defer func() {
		for _, s := range drin {
			verlassen("s5", s)
		}
	}()
	if betreten("s5", neuerSitzer(1)) {
		t.Fatal("einer zu viel kam hinein")
	}
}

// Closing twice must not panic: the dispatcher closes on backpressure and
// the departure routine closes on hangup.
func TestZweimalSchliessen(t *testing.T) {
	s := neuerSitzer(1)
	s.schliessen()
	s.schliessen()
}

// Entering and leaving run concurrently without corrupting the bookkeeping.
func TestRaumHaeltNebenlaeufigStand(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := neuerSitzer(64)
			betreten("s6", s)
			verteilen("s6", []byte("x"))
			verlassen("s6", s)
		}()
	}
	wg.Wait()
	if n := ImRaum("s6"); n != 0 {
		t.Fatalf("nach allen Abgängen sitzen noch %d im Raum", n)
	}
}

// A connection from a foreign origin is rejected. The session cookie would
// not be sent there anyway, but the realtime layer must not rely on that.
func TestFremdeHerkunftWirdAbgewiesen(t *testing.T) {
	faelle := []struct {
		herkunft string
		wirt     string
		zulassen bool
	}{
		{"https://wiki.example.org", "wiki.example.org", true},
		{"https://boeser.example.net", "wiki.example.org", false},
		{"", "wiki.example.org", false},
		{"https://wiki.example.org:8443", "wiki.example.org", false},
	}
	for _, f := range faelle {
		r := httptest.NewRequest(http.MethodGet, "/api/echtzeit/abc", nil)
		r.Host = f.wirt
		if f.herkunft != "" {
			r.Header.Set("Origin", f.herkunft)
		}
		err := gleicheHerkunft(nil, r)
		if (err == nil) != f.zulassen {
			t.Fatalf("Herkunft %q bei Wirt %q: Fehler=%v, erwartet zugelassen=%v",
				f.herkunft, f.wirt, err, f.zulassen)
		}
	}
}

// Now over a real connection: two browsers, one room, one packet.
//
// The checks above operate on in-memory structures; here everything depends
// on the connection setup working. That only succeeds if the measurement
// middleware passes the connection through — a single line easily lost when
// refactoring, and which would not cause any other test to fail.
func TestLeitungTraegtPaketeZwischenBrowsern(t *testing.T) {
	const seite = "leitungsprobe"
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mich := &sitzer{post: make(chan []byte, stauGrenze), uid: r.URL.Query().Get("wer")}
		if !betreten(seite, mich) {
			http.Error(w, "voll", http.StatusConflict)
			return
		}
		// As in the handler: run through the measurement middleware so the probe
		// takes the same path as a real request.
		middleware.Messen(puls.Neu())(leitung(seite, mich)).ServeHTTP(w, r)
		verlassen(seite, mich)
		mich.schliessen()
	}))
	defer dienst.Close()

	waehle := func(wer string) *websocket.Conn {
		t.Helper()
		adresse := "ws" + strings.TrimPrefix(dienst.URL, "http") + "/?wer=" + wer
		aufbau, err := websocket.NewConfig(adresse, dienst.URL)
		if err != nil {
			t.Fatalf("Adresse: %v", err)
		}
		c, err := websocket.DialConfig(aufbau)
		if err != nil {
			t.Fatalf("Verbinden als %s: %v", wer, err)
		}
		return c
	}

	a := waehle("a")
	defer a.Close()
	b := waehle("b")
	defer b.Close()

	// Warten, bis beide eingetragen sind: das Verbinden ist fertig, sobald der
	// Aufschlag durch ist, das Eintragen geschieht davor im Handler.
	for i := 0; i < 100 && ImRaum(seite) < 2; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if ImRaum(seite) != 2 {
		t.Fatalf("es sitzen %d im Raum, erwartet 2", ImRaum(seite))
	}

	// Ein Paket von a geht an b UND an a zurück: das Echo ist der Herzschlag.
	if err := websocket.Message.Send(a, []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("senden: %v", err)
	}
	for name, c := range map[string]*websocket.Conn{"b": b, "a": a} {
		var got []byte
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := websocket.Message.Receive(c, &got); err != nil {
			t.Fatalf("%s empfängt nicht: %v", name, err)
		}
		if string(got) != "\x01\x02\x03" {
			t.Fatalf("%s bekam %x", name, got)
		}
	}

	// Legt einer auf, ist der Raum um einen leerer.
	a.Close()
	for i := 0; i < 200 && ImRaum(seite) > 1; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if ImRaum(seite) != 1 {
		t.Fatalf("nach dem Auflegen sitzen %d im Raum, erwartet 1", ImRaum(seite))
	}
}
