// Local hosts: a curated list of addresses this instance probes so the
// dashboard shows which hosts in the network still respond.
//
// The neighbor of this file is `verbund.go`: that file lists services Nexora
// itself needs and therefore knows about automatically. This file lists hosts
// entered by a user to keep an eye on. Nexora knows nothing about them other
// than the data in the table.
//
// Measurements are self-contained. No Prometheus, no Grafana, no agent on the
// remote host: whatever appears in the table is what this instance observed
// when it probed the address. That is less than a dedicated monitor would
// know, but it is immediate and requires no extra infrastructure.
//
// Much information is revealed by default. A service usually announces its
// identity when a connection is made: an SSH server states its version before
// any password prompt, a web server sends a Server header, and a TLS service
// presents its certificate with expiry. Those are the fields shown.
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
	// How long a measurement is considered fresh. The UI polls every few
	// seconds; without this buffer every request would re-probe all hosts and
	// the dashboard would become a load generator.
	rechnerFrisch = 15 * time.Second
	// Short because the dashboard must return even when a host is unresponsive
	// — especially then.
	rechnerGeduld = 3 * time.Second
	// An upper bound so the list remains an overview and does not become a
	// network scanner.
	hoechstensRechner = 50
)

// `Rechner` is one row of the list as the UI sees it: the entered data and
// the latest measurement together.
type Rechner struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Ziel  string `json:"ziel"`
	Notiz string `json:"notiz,omitempty"`

	// Measured state: "antwortet" (responding), "still" (silent) or
	// "unbekannt" (unknown) while not yet probed. The strings are German as
	// they are shown in the UI.
	Zustand string `json:"zustand"`
	Antwort string `json:"antwort,omitempty"`
	Hinweis string `json:"hinweis,omitempty"`

	// What the host announced when probed: the service identifier and, for a
	// TLS connection, how long its certificate remains valid.
	Fassung    string `json:"fassung,omitempty"`
	Zertifikat string `json:"zertifikat,omitempty"`
	// `TageBisAblauf` provides the same information as a number so the UI can
	// colour a row without parsing text. Negative means expired.
	TageBisAblauf *int `json:"tageBisAblauf,omitempty"`
}

// `RechnerListe` is the response: the rows and when they were last measured.
type RechnerListe struct {
	Rechner    []Rechner `json:"rechner"`
	GeprueftUm string    `json:"geprueftUm,omitempty"`
}

// rechnerSpeicher keeps the last measurement. Stored in memory not in the
// database: after a restart nothing is measured, which is the truth, whereas
// a stored row from yesterday would falsely claim reachability.
var rechnerSpeicher = struct {
	sync.Mutex
	stand   map[string]Rechner
	geprüft time.Time
}{stand: map[string]Rechner{}}

// zielPruefen validates the input or reports what is missing.
//
// Allowed forms are "host:port" or a full HTTP URL. A hostname without port
// is rejected rather than guessed: only the person creating the entry knows
// which port is intended, and guessing would mark a host as silent when it
// simply listens on a different port.
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

// `anklopfen` measures whether someone responds at the address and records
// what it reveals about itself.
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
		// The Server header is the version the service claims. Some omit it and
		// that is fine — the field stays empty rather than guessing.
		m.Fassung = kurzeKennung(antwort.Header.Get("Server"))
		// A 404 is still a response: we ask whether a service is running there,
		// not whether it recognises this particular path. The status is shown in
		// case someone expected more.
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

// begruessung reads what a service says unprompted as soon as the connection
// is established.
//
// Many services do this: an SSH server announces its version before any
// password prompt, an SMTP server greets with 220 and its name. That answers
// "which release is running" without Nexora having to log in.
//
// If nothing is sent within the short deadline the field stays empty and
// nothing has waited. This probe never writes — it knocks and does not
// enter.
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

// kurzeKennung turns the raw input into a single table line.
//
// Only the first line and printable characters are kept, and truncated at 60
// characters: this is what a remote host wanted to say and should be clipped
// before appearing in the UI. A service that sends a whole page of text must
// not break the table.
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

// zertifikatsAlter reports how long the certificate is still valid.
//
// The numeric value is provided because an expired cert is the most common
// reason a local service suddenly becomes unreachable — and the only one you
// can see weeks in advance if someone bothered to display it.
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

// `klopfer` is the HTTP client used by the probe and differs from a normal
// client in two ways.
//
// It does not follow redirects: a 301 is already a response and where it
// points is a different question than whether anything responds at this
// address. Without this restriction the row could end up showing reachability
// of a remote host.
//
// It does not validate certificates. This is not a security hole but a
// practical necessity: the question is whether something answers, not whether
// its certificate chains to a CA. Many local services present self-signed
// certs and would appear "silent" if checked strictly. A dashboard that
// marks half the home as dead is worthless.
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

// `rechnerMessen` probes all addresses concurrently.

// Concurrent probing avoids cumulative wait times: probing ten silent hosts
// sequentially would take thirty seconds, during which the UI would appear
// unresponsive.
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

// `RechnerAnlegen` inserts a host into the list.
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
		// Use the address if no name is provided. A list with empty cells looks
		// like something is missing.
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

// `RechnerAendern` modifies a row.
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

// `RechnerLoeschen` removes a host from the list.
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

// `rechnerVergessen` discards the last measurement. After a change to the
// list it would be wrong: it could contain a state for an address that no
// longer exists.
func rechnerVergessen() {
	rechnerSpeicher.Lock()
	rechnerSpeicher.stand = map[string]Rechner{}
	rechnerSpeicher.geprüft = time.Time{}
	rechnerSpeicher.Unlock()
}
