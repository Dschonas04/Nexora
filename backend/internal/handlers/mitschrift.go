// Gemeinsames Schreiben an derselben Seite.
//
// Der Dienst führt hier keinen eigenen Text: er reicht die Pakete der Beteiligten
// nur weiter. Was am Ende dasteht, rechnen die Browser selbst aus, Yjs ist dafür
// gebaut und kommt ohne eine Schiedsstelle in der Mitte aus. Damit bleibt die
// Sitzung auch dann heil, wenn der Dienst mitten hinein neu startet: die Browser
// verbinden sich neu und gleichen ab, verloren ist nichts.
//
// Weitergereicht wird an alle im Raum, den Absender eingeschlossen. Das Echo
// klingt nach Verschwendung, ist aber der Herzschlag: der Browser legt auf,
// wenn dreißig Sekunden lang nichts kommt, und wer allein an einer Seite sitzt,
// bekäme sonst nie etwas zu hören. Yjs verträgt die eigene Änderung ein zweites
// Mal, sie ändert nichts mehr.
//
// Wer den Raum betreten darf, entscheidet dieselbe Prüfung wie beim Speichern.
// Nur Schreibberechtigte: ein Mitleser könnte über die Leitung sonst Text
// schicken, den ein anderer Browser dann arglos in die Datenbank schreibt.
package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"golang.org/x/net/websocket"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

const (
	// So viele dürfen gleichzeitig an einer Seite sitzen. Keine technische
	// Grenze, sondern eine gegen den Unfall: an einer Seite arbeitet ein Team,
	// keine Belegschaft.
	hoechstensImRaum = 25
	// So viele Pakete darf ein Langsamer hinter sich herziehen. Danach fliegt
	// er, statt dass sein Rückstau alle anderen bremst; sein Browser verbindet
	// sich neu und gleicht ab.
	stauGrenze = 512
	// Größtes einzelnes Paket. Ein Yjs-Paket ist im Betrieb ein paar hundert
	// Bytes; ein Megabyte deckt auch das erste vollständige Abgleichen einer
	// langen Seite ab.
	groesstesPaket = 1 << 20
)

// sitzer ist ein offener Browser in einem Raum. Geschrieben wird über den
// Kanal und nicht direkt auf die Verbindung: sonst schrieben mehrere Absender
// gleichzeitig in dieselbe Leitung.
type sitzer struct {
	post chan []byte
	uid  string
	// einmal, weil zwei Wege denselben Kanal schließen wollen: der Verteiler,
	// wenn der Rückstau zu groß wird, und der Abgang, wenn der Browser geht.
	// Ein Kanal zweimal geschlossen ist ein Absturz.
	einmal sync.Once
}

func (s *sitzer) schliessen() {
	s.einmal.Do(func() { close(s.post) })
}

type raum struct {
	sync.Mutex
	leute map[*sitzer]bool
}

// raeume hält die offenen Sitzungen. Im Speicher und nicht in der Datenbank:
// der Inhalt eines Raums ist nur so lange etwas wert, wie jemand darin sitzt.
var raeume = struct {
	sync.Mutex
	nach map[string]*raum
}{nach: map[string]*raum{}}

func betreten(seite string, s *sitzer) bool {
	raeume.Lock()
	r := raeume.nach[seite]
	if r == nil {
		r = &raum{leute: map[*sitzer]bool{}}
		raeume.nach[seite] = r
	}
	raeume.Unlock()

	r.Lock()
	defer r.Unlock()
	if len(r.leute) >= hoechstensImRaum {
		return false
	}
	r.leute[s] = true
	return true
}

func verlassen(seite string, s *sitzer) {
	raeume.Lock()
	r := raeume.nach[seite]
	raeume.Unlock()
	if r == nil {
		return
	}
	r.Lock()
	delete(r.leute, s)
	leer := len(r.leute) == 0
	r.Unlock()

	// Der leere Raum wird abgeräumt, sonst wüchse die Karte mit jeder je
	// geöffneten Seite und schrumpfte nie wieder.
	if leer {
		raeume.Lock()
		if r2 := raeume.nach[seite]; r2 == r {
			r2.Lock()
			if len(r2.leute) == 0 {
				delete(raeume.nach, seite)
			}
			r2.Unlock()
		}
		raeume.Unlock()
	}
}

func verteilen(seite string, paket []byte) {
	raeume.Lock()
	r := raeume.nach[seite]
	raeume.Unlock()
	if r == nil {
		return
	}
	r.Lock()
	defer r.Unlock()
	for s := range r.leute {
		select {
		case s.post <- paket:
		default:
			// Voll: der Kanal wird geschlossen, der Schreiber beendet sich und
			// legt die Verbindung auf.
			s.schliessen()
			delete(r.leute, s)
		}
	}
}

// ImRaum sagt, wie viele gerade an einer Seite sitzen. Für die Anzeige im
// Teilen-Fenster.
func ImRaum(seite string) int {
	raeume.Lock()
	r := raeume.nach[seite]
	raeume.Unlock()
	if r == nil {
		return 0
	}
	r.Lock()
	defer r.Unlock()
	return len(r.leute)
}

var errFremdeHerkunft = errors.New("fremde Herkunft")

// Mitschreibende sagt, wie viele gerade an dieser Seite sitzen. Das
// Teilen-Fenster zeigt es an; wer eine Seite freigibt, will sehen, ob jemand
// darin ist, bevor er etwas ändert.
func (s *Server) Mitschreibende(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"anzahl":   ImRaum(id),
		"moeglich": s.echtzeitAn(),
	})
}

// offeneRaeume gibt für jede Seite, an der gerade jemand sitzt, die Kennungen
// der Beteiligten zurück. Eine Kopie, keine Sicht auf das Original: der Aufrufer
// soll daran nichts verändern können, und die Sperre wird nur kurz gehalten.
func offeneRaeume() map[string][]string {
	raeume.Lock()
	kopie := make(map[string]*raum, len(raeume.nach))
	for seite, r := range raeume.nach {
		kopie[seite] = r
	}
	raeume.Unlock()

	raus := map[string][]string{}
	for seite, r := range kopie {
		r.Lock()
		for s := range r.leute {
			raus[seite] = append(raus[seite], s.uid)
		}
		r.Unlock()
		if len(raus[seite]) == 0 {
			delete(raus, seite)
		}
	}
	return raus
}

// MitschriftZustand zeigt der Verwaltung, woran gerade gemeinsam geschrieben
// wird: welche Seiten offen sind und wer daran sitzt.
//
// Nur die laufenden Sitzungen, nichts Gespeichertes. Wer wann was geschrieben
// hat, steht in der Prüfspur und in den Versionen; hier geht es um den
// Augenblick, und der ist vorbei, sobald der Letzte den Reiter schließt.
func (s *Server) MitschriftZustand(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	offen := offeneRaeume()
	// Namen und Titel in einem Rutsch, nicht in einer Schleife: die Zahl der
	// offenen Räume ist klein, die Zahl der Abfragen soll es auch bleiben.
	namen := map[string]string{}
	titel := map[string]string{}
	alleKonten := map[string]bool{}
	seiten := make([]string, 0, len(offen))
	for seite, konten := range offen {
		seiten = append(seiten, seite)
		for _, k := range konten {
			alleKonten[k] = true
		}
	}
	if len(seiten) > 0 {
		if rows, err := s.Pool.Query(r.Context(),
			`SELECT id::text, title FROM pages WHERE id = ANY($1)`, seiten); err == nil {
			for rows.Next() {
				var id, t string
				if err := rows.Scan(&id, &t); err == nil {
					titel[id] = t
				}
			}
			rows.Close()
		}
		liste := make([]string, 0, len(alleKonten))
		for k := range alleKonten {
			liste = append(liste, k)
		}
		if rows, err := s.Pool.Query(r.Context(),
			`SELECT id::text, name FROM users WHERE id = ANY($1)`, liste); err == nil {
			for rows.Next() {
				var id, n string
				if err := rows.Scan(&id, &n); err == nil {
					namen[id] = n
				}
			}
			rows.Close()
		}
	}

	art := make([]map[string]interface{}, 0, len(offen))
	for seite, konten := range offen {
		wer := make([]string, 0, len(konten))
		for _, k := range konten {
			if n := namen[k]; n != "" {
				wer = append(wer, n)
			}
		}
		sort.Strings(wer)
		t := titel[seite]
		if t == "" {
			t = "Ohne Titel"
		}
		art = append(art, map[string]interface{}{
			"seite":  seite,
			"titel":  t,
			"anzahl": len(konten),
			"wer":    wer,
		})
	}
	sort.Slice(art, func(i, j int) bool {
		return art[i]["titel"].(string) < art[j]["titel"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"an":         s.echtzeitAn(),
		"lizenziert": lizenz.Frei(lizenz.Echtzeit),
		"hoechstens": hoechstensImRaum,
		"raeume":     art,
	})
}

// gleicheHerkunft lässt nur Verbindungen von der eigenen Oberfläche zu.
//
// Der Anmeldekeks ist SameSite=Lax und reist bei einer fremden Seite gar nicht
// erst mit; das hier ist der Gürtel dazu. Die Prüfung, die x/net mitbringt,
// verlangt nur eine lesbare Herkunft und vergleicht sie mit nichts.
func gleicheHerkunft(_ *websocket.Config, req *http.Request) error {
	herkunft := req.Header.Get("Origin")
	if herkunft == "" {
		return errFremdeHerkunft
	}
	u, err := url.Parse(herkunft)
	if err != nil {
		return err
	}
	if !strings.EqualFold(u.Host, req.Host) {
		return errFremdeHerkunft
	}
	return nil
}

// Mitschrift ist die Leitung für das gemeinsame Schreiben an einer Seite.
func (s *Server) Mitschrift(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "read-only access")
		return
	}
	if !s.echtzeitAn() {
		writeErr(w, http.StatusForbidden, "gemeinsames Bearbeiten ist abgeschaltet")
		return
	}

	mich := &sitzer{post: make(chan []byte, stauGrenze), uid: uid}
	if !betreten(id, mich) {
		writeErr(w, http.StatusConflict, "an dieser Seite sitzen bereits genug Leute")
		return
	}

	leitung(id, mich).ServeHTTP(w, r)

	verlassen(id, mich)
	// Erst austragen, dann schließen: der Verteiler hat den Kanal jetzt nicht
	// mehr, ein Senden kann sich mit dem Schließen nicht mehr überschneiden.
	mich.schliessen()
}

// leitung ist die offene Verbindung selbst, ohne Rechteprüfung und ohne
// Datenbank. Eigens herausgezogen, damit sie sich prüfen lässt: das Verteilen
// über eine echte Leitung ist der Teil, der schweigend falsch sein kann.
func leitung(id string, mich *sitzer) http.Handler {
	return &websocket.Server{
		Handshake: gleicheHerkunft,
		Handler: func(c *websocket.Conn) {
			defer c.Close()
			c.MaxPayloadBytes = groesstesPaket

			// Der Schreiber läuft nebenher, damit das Lesen nicht wartet.
			fertig := make(chan struct{})
			go func() {
				defer close(fertig)
				for paket := range mich.post {
					if err := websocket.Message.Send(c, paket); err != nil {
						return
					}
				}
			}()

			for {
				var paket []byte
				// Kein r.Context() hier: der Router setzt darauf dreißig
				// Sekunden, und eine Sitzung dauert Stunden. Zu Ende ist sie,
				// wenn die Leitung zu ist.
				if err := websocket.Message.Receive(c, &paket); err != nil {
					return
				}
				if len(paket) == 0 {
					continue
				}
				verteilen(id, paket)
			}
		},
	}
}
