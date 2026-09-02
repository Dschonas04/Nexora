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

// Ein Paket geht an alle im Raum, den Absender eingeschlossen. Das Echo ist
// Absicht: es ist der Herzschlag für den, der allein an einer Seite sitzt.
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

// Räume sind getrennt: was an einer Seite geschrieben wird, hat an einer
// anderen nichts zu suchen.
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

// Wer nicht abholt, fliegt, statt alle anderen zu bremsen.
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

// Der leere Raum wird abgeräumt. Sonst wüchse die Karte mit jeder je
// geöffneten Seite und schrumpfte nie wieder.
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

// Mehr als die Obergrenze kommt nicht hinein.
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

// Zweimal schließen darf keinen Absturz geben: der Verteiler schließt beim
// Rückstau, der Abgang schließt beim Auflegen.
func TestZweimalSchliessen(t *testing.T) {
	s := neuerSitzer(1)
	s.schliessen()
	s.schliessen()
}

// Betreten und Verlassen laufen nebeneinander, ohne dass die Buchführung
// auseinanderfällt.
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

// Eine Verbindung von einer fremden Seite wird abgewiesen. Der Anmeldekeks
// reist dort zwar ohnehin nicht mit, aber die Leitung soll sich nicht darauf
// verlassen.
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

// Und jetzt über eine echte Leitung: zwei Browser, ein Raum, ein Paket.
//
// Die Prüfungen darüber arbeiten am Speicher; hier hängt alles daran, dass das
// Aufschalten überhaupt gelingt. Das kann es nur, wenn der Messfilter die
// Verbindung durchreicht, und genau das ist eine Zeile, die man beim Umbauen
// verliert, ohne dass ein einziger anderer Test rot wird.
func TestLeitungTraegtPaketeZwischenBrowsern(t *testing.T) {
	const seite = "leitungsprobe"
	dienst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mich := &sitzer{post: make(chan []byte, stauGrenze), uid: r.URL.Query().Get("wer")}
		if !betreten(seite, mich) {
			http.Error(w, "voll", http.StatusConflict)
			return
		}
		// Genau wie im Handler: durch den Messfilter hindurch, damit die Probe
		// den Weg nimmt, den eine echte Anfrage nimmt.
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
