// Eigene Rechner: eine gepflegte Liste von Adressen, bei denen diese Instanz
// anklopft, damit an einer Stelle steht, wer im Haus noch antwortet.
//
// Der Nachbar dieser Datei ist verbund.go, und die Aufteilung ist die: dort
// stehen die Dienste, die Nexora zum Laufen braucht und deshalb von selbst
// kennt. Hier stehen die, die jemand einträgt, weil er sie im Blick behalten
// will. Nexora weiß von denen nichts, außer was in der Zeile steht.
//
// Angeklopft wird auf der Ebene, auf der eine Aussage ohne Zugangsdaten möglich
// ist: eine TCP-Verbindung, die zustande kommt, oder eine HTTP-Antwort, die
// eintrifft. Mehr geht ohne Anmeldung am fremden Rechner nicht, und einen
// Generalschlüssel zum eigenen Netz in einem Wiki abzulegen, das nach außen
// erreichbar sein kann, wäre ein schlechter Tausch.
//
// Die Fassung kommt deshalb aus einer zweiten Quelle: aus dem Prometheus, den
// die meisten ohnehin betreiben. Der node_exporter kennt Betriebssystem, Kern
// und Startzeit jedes Rechners, und Nexora fragt ihn danach, statt sich selbst
// irgendwo anzumelden. Ohne eingetragene Adresse bleibt die Spalte leer, und
// die Erreichbarkeit steht trotzdem da.
package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Ziel    string `json:"ziel"`
	Notiz   string `json:"notiz,omitempty"`
	Instanz string `json:"instanz,omitempty"`

	// Gemessen: "antwortet", "still" oder "unbekannt", solange nichts geprüft
	// wurde. Deutsch, weil es so angezeigt wird.
	Zustand string `json:"zustand"`
	Antwort string `json:"antwort,omitempty"`
	Hinweis string `json:"hinweis,omitempty"`

	// Aus Prometheus, wenn dort etwas über den Rechner steht.
	System string `json:"system,omitempty"`
	Kern   string `json:"kern,omitempty"`
	Laeuft string `json:"laeuft,omitempty"`
}

// RechnerListe ist die Antwort samt dem, was zur Herkunft der Fassungen zu
// sagen ist. Die Oberfläche soll erklären können, warum eine Spalte leer ist.
type RechnerListe struct {
	Rechner    []Rechner `json:"rechner"`
	Prometheus string    `json:"prometheus,omitempty"`
	Quelle     string    `json:"quelle,omitempty"`
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

// anklopfen misst, ob unter der Adresse jemand antwortet.
func anklopfen(ctx context.Context, ziel string) (bool, time.Duration, string) {
	ctx, abbruch := context.WithTimeout(ctx, rechnerGeduld)
	defer abbruch()
	beginn := time.Now()

	if strings.HasPrefix(ziel, "http://") || strings.HasPrefix(ziel, "https://") {
		anfrage, err := http.NewRequestWithContext(ctx, http.MethodGet, ziel, nil)
		if err != nil {
			return false, 0, kurz(err.Error())
		}
		antwort, err := klopfer.Do(anfrage)
		if err != nil {
			return false, time.Since(beginn), kurz(err.Error())
		}
		antwort.Body.Close()
		// Auch eine 404 ist eine Antwort: gefragt ist, ob dort ein Dienst
		// läuft, nicht ob er diesen einen Weg kennt. Der Status steht daneben,
		// falls jemand mehr erwartet hat.
		hinweis := ""
		if antwort.StatusCode >= 400 {
			hinweis = "antwortet mit " + strconv.Itoa(antwort.StatusCode)
		}
		return true, time.Since(beginn), hinweis
	}

	waehler := net.Dialer{Timeout: rechnerGeduld}
	verbindung, err := waehler.DialContext(ctx, "tcp", ziel)
	if err != nil {
		return false, time.Since(beginn), kurz(err.Error())
	}
	verbindung.Close()
	return true, time.Since(beginn), ""
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
	if adresse := prometheusAdresse(); adresse != "" {
		antwort.Prometheus = adresse
		antwort.Quelle = "Prometheus"
	}
	rechnerSpeicher.Lock()
	if !rechnerSpeicher.geprüft.IsZero() {
		antwort.GeprueftUm = rechnerSpeicher.geprüft.Format(time.RFC3339)
	}
	rechnerSpeicher.Unlock()
	writeJSON(w, http.StatusOK, antwort)
}

func (s *Server) rechnerLesen(ctx context.Context) ([]Rechner, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, name, ziel, notiz, instanz FROM rechner
		  ORDER BY reihenfolge, angelegt_am`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	liste := []Rechner{}
	for rows.Next() {
		var e Rechner
		if err := rows.Scan(&e.ID, &e.Name, &e.Ziel, &e.Notiz, &e.Instanz); err == nil {
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
				e.System, e.Kern, e.Laeuft = alt.System, alt.Kern, alt.Laeuft
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
			da, gedauert, hinweis := anklopfen(ctx, e.Ziel)
			if da {
				e.Zustand = "antwortet"
				e.Antwort = dauer(gedauert)
			} else {
				e.Zustand = "still"
			}
			e.Hinweis = hinweis
			fertig[i] = e
		}(i, e)
	}
	warten.Wait()

	ausPrometheus(ctx, fertig)

	rechnerSpeicher.Lock()
	rechnerSpeicher.stand = map[string]Rechner{}
	for _, e := range fertig {
		rechnerSpeicher.stand[e.ID] = e
	}
	rechnerSpeicher.geprüft = time.Now()
	rechnerSpeicher.Unlock()
	return fertig
}

// prometheusAdresse ist der Prometheus, den diese Instanz fragen darf. Leer
// heißt: keine Fassungen, nur Erreichbarkeit.
func prometheusAdresse() string {
	return strings.TrimRight(strings.TrimSpace(wert("prometheus_adresse")), "/")
}

// ausPrometheus trägt Betriebssystem, Kern und Laufzeit nach.
//
// Drei Abfragen und nicht eine je Rechner: Prometheus antwortet auf eine Frage
// mit allen Zeitreihen, die dazu passen, und die Zuordnung geschieht danach
// hier. Bei fünfzig Rechnern ist das der Unterschied zwischen drei Anfragen und
// hundertfünfzig.
func ausPrometheus(ctx context.Context, liste []Rechner) {
	adresse := prometheusAdresse()
	if adresse == "" || len(liste) == 0 {
		return
	}
	ctx, abbruch := context.WithTimeout(ctx, rechnerGeduld)
	defer abbruch()

	uname := prometheusFragen(ctx, adresse, "node_uname_info")
	os := prometheusFragen(ctx, adresse, "node_os_info")
	start := prometheusFragen(ctx, adresse, "node_boot_time_seconds")
	if len(uname) == 0 && len(os) == 0 && len(start) == 0 {
		return
	}

	for i := range liste {
		schluessel := instanzSchluessel(liste[i].Instanz, liste[i].Ziel)
		if schluessel == "" {
			continue
		}
		if m, ok := uname[schluessel]; ok {
			liste[i].Kern = m.Labels["release"]
			if liste[i].System == "" && m.Labels["sysname"] != "" {
				liste[i].System = m.Labels["sysname"]
			}
		}
		if m, ok := os[schluessel]; ok {
			if huebsch := m.Labels["pretty_name"]; huebsch != "" {
				liste[i].System = huebsch
			}
		}
		if m, ok := start[schluessel]; ok {
			if sek, err := strconv.ParseFloat(m.Wert, 64); err == nil && sek > 0 {
				liste[i].Laeuft = dauer(time.Since(time.Unix(int64(sek), 0)))
			}
		}
	}
}

// instanzSchluessel bestimmt, unter welchem Namen ein Rechner in Prometheus zu
// suchen ist: die eingetragene Kennung, sonst der Rechnername aus dem Ziel.
// Der Port bleibt außen vor, denn in Prometheus steht der des Exporters (9100)
// und hier der, an dem angeklopft wird (etwa 22).
func instanzSchluessel(instanz, ziel string) string {
	roh := instanz
	if roh == "" {
		roh = ziel
	}
	roh = strings.TrimSpace(roh)
	if roh == "" {
		return ""
	}
	if u, err := url.Parse(roh); err == nil && u.Host != "" {
		roh = u.Host
	}
	if rechner, _, err := net.SplitHostPort(roh); err == nil {
		roh = rechner
	}
	return strings.ToLower(roh)
}

// promReihe ist eine Zeitreihe, so weit sie hier gebraucht wird.
type promReihe struct {
	Labels map[string]string
	Wert   string
}

// prometheusFragen stellt eine Sofortabfrage und ordnet die Antwort nach dem
// Rechnernamen aus dem instance-Etikett.
//
// Fehler sind hier kein Fehler: antwortet der Prometheus nicht, bleiben die
// Spalten leer, und die Erreichbarkeit steht trotzdem da. Eine Übersicht, die
// ganz ausfällt, weil eine Nebenquelle klemmt, wäre der schlechtere Tausch.
func prometheusFragen(ctx context.Context, adresse, abfrage string) map[string]promReihe {
	ziel := adresse + "/api/v1/query?query=" + url.QueryEscape(abfrage)
	anfrage, err := http.NewRequestWithContext(ctx, http.MethodGet, ziel, nil)
	if err != nil {
		return nil
	}
	antwort, err := http.DefaultClient.Do(anfrage)
	if err != nil {
		return nil
	}
	defer antwort.Body.Close()
	if antwort.StatusCode != http.StatusOK {
		return nil
	}

	var gelesen struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(antwort.Body).Decode(&gelesen); err != nil {
		return nil
	}

	nach := map[string]promReihe{}
	for _, reihe := range gelesen.Data.Result {
		schluessel := instanzSchluessel(reihe.Metric["instance"], "")
		if schluessel == "" {
			continue
		}
		eintrag := promReihe{Labels: reihe.Metric}
		// value ist [zeit, "wert"], der Wert steht als Zeichenkette hinten.
		if len(reihe.Value) == 2 {
			if s, ok := reihe.Value[1].(string); ok {
				eintrag.Wert = s
			}
		}
		nach[schluessel] = eintrag
	}
	return nach
}

type rechnerReq struct {
	Name    string `json:"name"`
	Ziel    string `json:"ziel"`
	Notiz   string `json:"notiz"`
	Instanz string `json:"instanz"`
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
		`INSERT INTO rechner (name, ziel, notiz, instanz, reihenfolge)
		 VALUES ($1, $2, $3, $4, (SELECT coalesce(max(reihenfolge), 0) + 1 FROM rechner))
		 RETURNING id, name, ziel, notiz, instanz`,
		name, ziel, strings.TrimSpace(req.Notiz), strings.TrimSpace(req.Instanz)).
		Scan(&e.ID, &e.Name, &e.Ziel, &e.Notiz, &e.Instanz)
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
		`UPDATE rechner SET name=$2, ziel=$3, notiz=$4, instanz=$5 WHERE id=$1
		 RETURNING id, name, ziel, notiz, instanz`,
		id, name, ziel, strings.TrimSpace(req.Notiz), strings.TrimSpace(req.Instanz)).
		Scan(&e.ID, &e.Name, &e.Ziel, &e.Notiz, &e.Instanz)
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
