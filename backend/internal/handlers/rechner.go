// Eigene Rechner: eine gepflegte Liste von Adressen, bei denen diese Instanz
// anklopft, damit an einer Stelle steht, wer im Haus noch antwortet.
//
// Der Nachbar dieser Datei ist verbund.go, und die Aufteilung ist die: dort
// stehen die Dienste, die Nexora zum Laufen braucht und deshalb von selbst
// kennt. Hier stehen die, die jemand einträgt, weil er sie im Blick behalten
// will. Nexora weiß von denen nichts, außer was in der Zeile steht.
//
// Gemessen wird ohne fremde Hilfe. Kein Prometheus, kein Grafana, kein Zugang
// zum fremden Rechner: was in der Tabelle steht, hat diese Instanz selbst
// gesehen, als sie angeklopft hat. Das ist weniger, als ein Überwacher mit
// Agenten auf jedem Gerät wüsste, und es ist dafür sofort da und hängt an
// nichts.
//
// Erstaunlich viel fällt dabei ohnehin ab. Wer eine Verbindung annimmt, sagt
// meist im ersten Atemzug, wer er ist: ein SSH-Dienst nennt seine Fassung,
// bevor überhaupt jemand nach einem Passwort gefragt hat, ein Webserver nennt
// sie in der Kopfzeile Server, und ein verschlüsselter Dienst zeigt sein
// Zertifikat samt Ablaufdatum. Genau das steht in den Spalten.
package handlers

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

const (
	// So lange gilt eine Messung als frisch. Die Oberfläche fragt im Takt von
	// wenigen Sekunden nach; ohne diesen Puffer klopfte jede Anfrage erneut an
	// jedem Rechner an, und aus einer Übersicht würde eine Belastung.
	rechnerFrisch = 15 * time.Second
	// Kurz, weil die Übersicht auch dann erscheinen muss, wenn ein Rechner
	// hängt -- gerade dann.
	rechnerGeduld = 3 * time.Second
	// Eine Obergrenze, damit die Liste eine Übersicht bleibt und nicht zum
	// Netzwerkscanner wird.
	hoechstensRechner = 50
)

// Rechner ist eine Zeile der Liste, wie die Oberfläche sie sieht: das
// Eingetragene und das gerade Gemessene in einem Stück.
type Rechner struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Ziel  string `json:"ziel"`
	Notiz string `json:"notiz,omitempty"`

	// Gemessen: "antwortet", "still" oder "unbekannt", solange nichts geprüft
	// wurde. Deutsch, weil es so angezeigt wird.
	Zustand string `json:"zustand"`
	Antwort string `json:"antwort,omitempty"`
	Hinweis string `json:"hinweis,omitempty"`

	// Was der Rechner beim Anklopfen von sich erzählt hat: die Kennung des
	// Dienstes, und bei einer verschlüsselten Verbindung, wie lange sein
	// Zertifikat noch gilt.
	Fassung    string `json:"fassung,omitempty"`
	Zertifikat string `json:"zertifikat,omitempty"`
	// TageBisAblauf ist dieselbe Angabe als Zahl, damit die Oberfläche eine
	// Zeile einfärben kann, ohne Text auszuwerten. Negativ heißt: abgelaufen.
	TageBisAblauf *int `json:"tageBisAblauf,omitempty"`
}

// RechnerListe ist die Antwort: die Zeilen und wann zuletzt gemessen wurde.
type RechnerListe struct {
	Rechner    []Rechner `json:"rechner"`
	GeprueftUm string    `json:"geprueftUm,omitempty"`
}

// rechnerSpeicher hält die letzte Messung. Ein Zustand im Arbeitsspeicher und
// nicht in der Datenbank: nach einem Neustart ist nichts gemessen, und das ist
// die Wahrheit, während eine gespeicherte Zeile von gestern behaupten würde,
// der Rechner sei erreichbar.
var rechnerSpeicher = struct {
	sync.Mutex
	stand   map[string]Rechner
	geprüft time.Time
}{stand: map[string]Rechner{}}

// zielPruefen nimmt die Eingabe an oder sagt, was ihr fehlt.
//
// Erlaubt ist "host:port" oder eine vollständige HTTP-Adresse. Ein Rechnername
// ohne Port wird abgelehnt statt geraten: welcher Port gemeint ist, weiß nur
// der, der die Zeile schreibt, und ein geratener Port meldet einen Rechner als
// still, der bloß auf einem anderen hört.
func zielPruefen(roh string) (string, error) {
	ziel := strings.TrimSpace(roh)
	if ziel == "" {
		return "", errors.New("ohne Adresse geht es nicht")
	}
	if strings.HasPrefix(ziel, "http://") || strings.HasPrefix(ziel, "https://") {
		u, err := url.Parse(ziel)
		if err != nil || u.Host == "" {
			return "", errors.New("diese Web-Adresse versteht niemand")
		}
		return ziel, nil
	}
	if strings.Contains(ziel, "://") {
		return "", errors.New("nur http:// und https://, sonst host:port")
	}
	rechner, port, err := net.SplitHostPort(ziel)
	if err != nil || rechner == "" {
		return "", errors.New("die Adresse braucht einen Port, etwa 10.0.0.5:22")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", errors.New("der Port ist keine Zahl zwischen 1 und 65535")
	}
	return ziel, nil
}

// messung ist, was ein einzelnes Anklopfen ergeben hat.
type messung struct {
	Da         bool
	Dauer      time.Duration
	Fassung    string
	Zertifikat string
	Tage       *int
	Hinweis    string
}

// anklopfen misst, ob unter der Adresse jemand antwortet, und nimmt mit, was
// er dabei von sich erzählt.
func anklopfen(ctx context.Context, ziel string) messung {
	ctx, abbruch := context.WithTimeout(ctx, rechnerGeduld)
	defer abbruch()
	beginn := time.Now()

	if strings.HasPrefix(ziel, "http://") || strings.HasPrefix(ziel, "https://") {
		anfrage, err := http.NewRequestWithContext(ctx, http.MethodGet, ziel, nil)
		if err != nil {
			return messung{Hinweis: kurz(err.Error())}
		}
		antwort, err := klopfer.Do(anfrage)
		if err != nil {
			return messung{Dauer: time.Since(beginn), Hinweis: kurz(err.Error())}
		}
		defer antwort.Body.Close()

		m := messung{Da: true, Dauer: time.Since(beginn)}
		// Die Kopfzeile Server ist die Fassung, die der Dienst selbst nennt.
		// Manche verschweigen sie, und das ist ihr gutes Recht -- dann bleibt
		// die Spalte leer, statt dass hier etwas geraten wird.
		m.Fassung = kurzeKennung(antwort.Header.Get("Server"))
		// Auch eine 404 ist eine Antwort: gefragt ist, ob dort ein Dienst
		// läuft, nicht ob er diesen einen Weg kennt. Der Status steht daneben,
		// falls jemand mehr erwartet hat.
		if antwort.StatusCode >= 400 {
			m.Hinweis = "antwortet mit " + strconv.Itoa(antwort.StatusCode)
		}
		if antwort.TLS != nil && len(antwort.TLS.PeerCertificates) > 0 {
			m.Zertifikat, m.Tage = zertifikatsAlter(antwort.TLS.PeerCertificates[0].NotAfter)
		}
		return m
	}

	waehler := net.Dialer{Timeout: rechnerGeduld}
	verbindung, err := waehler.DialContext(ctx, "tcp", ziel)
	if err != nil {
		return messung{Dauer: time.Since(beginn), Hinweis: kurz(err.Error())}
	}
	defer verbindung.Close()
	m := messung{Da: true, Dauer: time.Since(beginn)}
	m.Fassung = kurzeKennung(begruessung(verbindung))
	return m
}

// begruessung liest, was ein Dienst von sich aus sagt, sobald die Verbindung
// steht.
//
// Viele tun das: ein SSH-Dienst nennt seine Fassung, bevor irgendjemand nach
// einem Passwort gefragt hat, ein Postfachdienst grüßt mit 220 und seinem
// Namen. Genau das ist die Antwort auf "welche Fassung läuft da", ohne dass
// sich Nexora irgendwo anmelden müsste.
//
// Wer nichts sagt, sagt nichts: die kurze Frist läuft ab, die Spalte bleibt
// leer, und niemand hat gewartet. Geschrieben wird nie -- diese Prüfung klopft
// an und tritt nicht ein.
func begruessung(verbindung net.Conn) string {
	if err := verbindung.SetReadDeadline(time.Now().Add(600 * time.Millisecond)); err != nil {
		return ""
	}
	puffer := make([]byte, 256)
	n, err := verbindung.Read(puffer)
	if n <= 0 || (err != nil && n == 0) {
		return ""
	}
	return string(puffer[:n])
}

// kurzeKennung macht aus dem, was hereinkam, eine Zeile für die Tabelle.
//
// Nur die erste Zeile, nur Druckbares, und bei sechzig Zeichen ist Schluss:
// hier kommt an, was ein fremder Rechner sagen wollte, und das gehört
// beschnitten, bevor es in einer Oberfläche steht. Ein Dienst, der statt einer
// Kennung eine Seite Text schickt, soll die Tabelle nicht sprengen.
func kurzeKennung(roh string) string {
	roh = strings.SplitN(roh, "\n", 2)[0]
	roh = strings.TrimSpace(strings.SplitN(roh, "\r", 2)[0])
	var sauber strings.Builder
	for _, z := range roh {
		if z < 32 || z == 127 {
			continue
		}
		sauber.WriteRune(z)
		if sauber.Len() >= 60 {
			break
		}
	}
	return sauber.String()
}

// zertifikatsAlter sagt, wie lange das Zertifikat noch gilt.
//
// Die Zahl steht daneben, weil ein abgelaufenes Zertifikat der häufigste Grund
// dafür ist, dass ein Dienst im eigenen Haus plötzlich nicht mehr erreichbar
// ist -- und der einzige, den man Wochen vorher sehen könnte, wenn ihn nur
// jemand anzeigte.
func zertifikatsAlter(bis time.Time) (string, *int) {
	tage := int(time.Until(bis).Hours() / 24)
	switch {
	case tage < 0:
		return "abgelaufen", &tage
	case tage == 0:
		return "läuft heute ab", &tage
	case tage == 1:
		return "noch 1 Tag", &tage
	default:
		return "noch " + strconv.Itoa(tage) + " Tage", &tage
	}
}

// klopfer ist der Browser dieser Prüfung, und er unterscheidet sich in zwei
// Punkten von einem gewöhnlichen.
//
// Er folgt keiner Umleitung: eine 301 ist bereits eine Antwort, und wohin sie
// zeigt, ist eine andere Frage als die, ob unter dieser Adresse jemand da ist.
// Ohne die Bremse stünde am Ende die Erreichbarkeit eines fremden Rechners in
// der Zeile.
//
// Und er prüft das Zertifikat nicht. Das ist hier kein Loch, sondern die
// einzige brauchbare Einstellung: gefragt wird, ob etwas antwortet, gelesen
// wird nichts. Ein Proxmox, ein NAS, ein Backup-Server -- alles, was man im
// eigenen Haus überwachen will, trägt ein selbst unterschriebenes Zertifikat,
// und mit Prüfung stünden sie samt und sonders als "still" da, obwohl sie
// laufen. Eine Übersicht, die die halbe Wohnung für tot erklärt, ist wertlos.
var klopfer = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // siehe oben
		// Keine Verbindung wird aufgehoben: es sind wenige Adressen, selten
		// gefragt, und eine offene Verbindung zu einem Rechner, der gerade
		// neu startet, meldete ihn als erreichbar.
		DisableKeepAlives: true,
	},
}

// ListRechner gibt die Liste samt frischer Messung heraus.
func (s *Server) ListRechner(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	eintraege, err := s.rechnerLesen(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gelesen werden")
		return
	}

	antwort := RechnerListe{Rechner: s.rechnerMessen(r.Context(), eintraege)}
	rechnerSpeicher.Lock()
	if !rechnerSpeicher.geprüft.IsZero() {
		antwort.GeprueftUm = rechnerSpeicher.geprüft.Format(time.RFC3339)
	}
	rechnerSpeicher.Unlock()
	writeJSON(w, http.StatusOK, antwort)
}

func (s *Server) rechnerLesen(ctx context.Context) ([]Rechner, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, name, ziel, notiz FROM rechner
		  ORDER BY reihenfolge, angelegt_am`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	liste := []Rechner{}
	for rows.Next() {
		var e Rechner
		if err := rows.Scan(&e.ID, &e.Name, &e.Ziel, &e.Notiz); err == nil {
			liste = append(liste, e)
		}
	}
	return liste, rows.Err()
}

// rechnerMessen klopft an allen Adressen gleichzeitig an und reichert das
// Ergebnis mit dem an, was Prometheus über die Rechner weiß.
//
// Gleichzeitig, weil nacheinander die Wartezeiten aufeinanderlägen: bei zehn
// stillen Rechnern wären das dreißig Sekunden, und so lange sieht niemand einer
// Übersicht beim Laden zu.
func (s *Server) rechnerMessen(ctx context.Context, eintraege []Rechner) []Rechner {
	rechnerSpeicher.Lock()
	frisch := time.Since(rechnerSpeicher.geprüft) < rechnerFrisch
	if frisch {
		fertig := make([]Rechner, 0, len(eintraege))
		for _, e := range eintraege {
			if alt, ok := rechnerSpeicher.stand[e.ID]; ok {
				e.Zustand, e.Antwort, e.Hinweis = alt.Zustand, alt.Antwort, alt.Hinweis
				e.Fassung, e.Zertifikat, e.TageBisAblauf = alt.Fassung, alt.Zertifikat, alt.TageBisAblauf
			} else {
				e.Zustand = "unbekannt"
			}
			fertig = append(fertig, e)
		}
		rechnerSpeicher.Unlock()
		return fertig
	}
	rechnerSpeicher.Unlock()

	fertig := make([]Rechner, len(eintraege))
	var warten sync.WaitGroup
	for i, e := range eintraege {
		warten.Add(1)
		go func(i int, e Rechner) {
			defer warten.Done()
			m := anklopfen(ctx, e.Ziel)
			if m.Da {
				e.Zustand = "antwortet"
				e.Antwort = dauer(m.Dauer)
			} else {
				e.Zustand = "still"
			}
			e.Fassung, e.Zertifikat, e.TageBisAblauf = m.Fassung, m.Zertifikat, m.Tage
			e.Hinweis = m.Hinweis
			fertig[i] = e
		}(i, e)
	}
	warten.Wait()

	rechnerSpeicher.Lock()
	rechnerSpeicher.stand = map[string]Rechner{}
	for _, e := range fertig {
		rechnerSpeicher.stand[e.ID] = e
	}
	rechnerSpeicher.geprüft = time.Now()
	rechnerSpeicher.Unlock()
	return fertig
}

type rechnerReq struct {
	Name  string `json:"name"`
	Ziel  string `json:"ziel"`
	Notiz string `json:"notiz"`
}

// RechnerAnlegen trägt einen Rechner in die Liste ein.
func (s *Server) RechnerAnlegen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	var req rechnerReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	ziel, err := zielPruefen(req.Ziel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		// Ohne Namen tut es die Adresse. Eine Liste mit leeren Zellen sähe aus,
		// als fehlte etwas.
		name = ziel
	}

	var anzahl int
	_ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM rechner`).Scan(&anzahl)
	if anzahl >= hoechstensRechner {
		writeErr(w, http.StatusConflict,
			"mehr als "+strconv.Itoa(hoechstensRechner)+" Rechner sind keine Übersicht mehr")
		return
	}

	var e Rechner
	err = s.Pool.QueryRow(r.Context(),
		`INSERT INTO rechner (name, ziel, notiz, reihenfolge)
		 VALUES ($1, $2, $3, (SELECT coalesce(max(reihenfolge), 0) + 1 FROM rechner))
		 RETURNING id, name, ziel, notiz`,
		name, ziel, strings.TrimSpace(req.Notiz)).
		Scan(&e.ID, &e.Name, &e.Ziel, &e.Notiz)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht angelegt werden")
		return
	}
	e.Zustand = "unbekannt"
	rechnerVergessen()
	s.spurAusRequest(r, AktRechnerAngelegt, "rechner", e.ID, e.Name, map[string]any{"ziel": e.Ziel})
	writeJSON(w, http.StatusCreated, e)
}

// RechnerAendern ändert eine Zeile.
func (s *Server) RechnerAendern(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	id := chi.URLParam(r, "id")
	var req rechnerReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	ziel, err := zielPruefen(req.Ziel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = ziel
	}

	var e Rechner
	err = s.Pool.QueryRow(r.Context(),
		`UPDATE rechner SET name=$2, ziel=$3, notiz=$4 WHERE id=$1
		 RETURNING id, name, ziel, notiz`,
		id, name, ziel, strings.TrimSpace(req.Notiz)).
		Scan(&e.ID, &e.Name, &e.Ziel, &e.Notiz)
	if err != nil {
		writeErr(w, http.StatusNotFound, "nicht gefunden")
		return
	}
	e.Zustand = "unbekannt"
	rechnerVergessen()
	s.spurAusRequest(r, AktRechnerGeaendert, "rechner", e.ID, e.Name, map[string]any{"ziel": e.Ziel})
	writeJSON(w, http.StatusOK, e)
}

// RechnerLoeschen nimmt einen Rechner aus der Liste.
func (s *Server) RechnerLoeschen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	id := chi.URLParam(r, "id")
	var name string
	tag, err := s.Pool.Query(r.Context(), `DELETE FROM rechner WHERE id=$1 RETURNING name`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht entfernt werden")
		return
	}
	gefunden := false
	for tag.Next() {
		if tag.Scan(&name) == nil {
			gefunden = true
		}
	}
	tag.Close()
	if !gefunden {
		writeErr(w, http.StatusNotFound, "nicht gefunden")
		return
	}
	rechnerVergessen()
	s.spurAusRequest(r, AktRechnerEntfernt, "rechner", id, name, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// rechnerVergessen wirft die letzte Messung weg. Nach einer Änderung an der
// Liste wäre sie falsch: sie enthielte einen Zustand zu einer Adresse, die es
// so nicht mehr gibt.
func rechnerVergessen() {
	rechnerSpeicher.Lock()
	rechnerSpeicher.stand = map[string]Rechner{}
	rechnerSpeicher.geprüft = time.Time{}
	rechnerSpeicher.Unlock()
}
