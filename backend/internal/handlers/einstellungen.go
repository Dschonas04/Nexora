// Runtime settings: the ones an administrator changes from the browser.
//
// The split against config.conf is deliberate. What has to be known before the
// database is open — its address, the listening port, the signing secret —
// lives in the file. What may change while the server runs lives here, because
// nobody can edit a file inside a container from a web page.
//
// Values are cached in memory and re-read only when something writes, so the
// hot paths (every registration, every upload) cost a map lookup rather than a
// query.
package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexora/internal/config"
	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// Einstellung describes one setting for the interface: what it is called, what
// it means, and what happens at the extremes. The descriptions travel to the
// browser so the page does not carry its own copy that drifts.
type Einstellung struct {
	Schluessel   string `json:"schluessel"`
	Wert         string `json:"wert"`
	Art          string `json:"art"` // "janein", "zahl", "text", "liste"
	Titel        string `json:"titel"`
	Erklaerung   string `json:"erklaerung"`
	Warnung      string `json:"warnung,omitempty"`
	Vorgabe      string `json:"vorgabe"`
	AusDatei     bool   `json:"ausDatei"` // true: value still comes from config.conf
	GeaendertVon string `json:"geaendertVon,omitempty"`
	GeaendertAm  string `json:"geaendertAm,omitempty"`
}

// bekannt lists every setting that may be changed at runtime. A key that is not
// in here is rejected — otherwise a crafted request could write arbitrary rows
// into the table and a later reader would trust them.
var bekannt = map[string]struct {
	Art        string
	Titel      string
	Erklaerung string
	Warnung    string
}{
	"registrierung_offen": {
		Art:        "janein",
		Titel:      "Selbstregistrierung",
		Erklaerung: "Darf sich jeder mit der Adresse dieser Instanz ein Konto anlegen? Steht sie aus, legen nur Administratoren Konten an.",
		Warnung:    "Für eine Firmeninstanz gehört das aus. Sonst kann sich jeder eintragen, der die Adresse kennt.",
	},
	"erlaubte_domaenen": {
		Art:        "liste",
		Titel:      "Erlaubte E-Mail-Domänen",
		Erklaerung: "Kommagetrennt. Leer heißt: keine Einschränkung. Wirkt auf die Selbstregistrierung, nicht auf Konten, die ein Administrator anlegt.",
	},
	"max_anhang_mb": {
		Art:        "zahl",
		Titel:      "Größte Datei je Anhang (MB)",
		Erklaerung: "Die eigentliche Grenze ist der Platz auf der Platte hinter dem Datenverzeichnis.",
	},
	"sitzung_tage": {
		Art:        "zahl",
		Titel:      "Gültigkeit einer Anmeldung (Tage)",
		Erklaerung: "Es gibt keine Verlängerung und keine Liste offener Sitzungen: ein Token bleibt bis zum Ablauf brauchbar, auch nach einer Passwortänderung.",
		Warnung:    "Wirkt erst auf NEUE Anmeldungen. Bereits ausgegebene Token behalten ihre alte Laufzeit.",
	},
	"such_woerterbuch": {
		Art:        "text",
		Titel:      "Wörterbuch der Volltextsuche",
		Erklaerung: "german, english oder simple. simple stemmt gar nicht und trifft dafür in keiner Sprache daneben.",
		Warnung:    "Eine Änderung wirkt erst nach einem Neuaufbau des Suchindex -- die Spalte wurde mit dem alten Wörterbuch erzeugt.",
	},
}

// speicher hält die Werte im Arbeitsspeicher.
var speicher struct {
	sync.RWMutex
	werte map[string]string
	basis config.Konfig // was aus der Datei kam, als Rückfall
}

// EinstellungenLaden fills the cache at startup. Rows in the database win over
// the file: they were set deliberately and later.
func (s *Server) EinstellungenLaden(ctx context.Context, k config.Konfig) {
	speicher.Lock()
	defer speicher.Unlock()

	speicher.basis = k
	speicher.werte = map[string]string{}

	rows, err := s.Pool.Query(ctx, `SELECT schluessel, wert FROM einstellungen`)
	if err != nil {
		log.Printf("Einstellungen laden: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			if _, ok := bekannt[k]; ok {
				speicher.werte[k] = v
			}
		}
	}
	if len(speicher.werte) > 0 {
		log.Printf("Einstellungen: %d Werte aus der Datenbank überschreiben die Datei", len(speicher.werte))
	}
}

// wert returns the effective value: database first, then the file.
func wert(schluessel string) string {
	speicher.RLock()
	defer speicher.RUnlock()
	if v, ok := speicher.werte[schluessel]; ok {
		return v
	}
	return ausDatei(schluessel, speicher.basis)
}

func ausDatei(schluessel string, k config.Konfig) string {
	switch schluessel {
	case "registrierung_offen":
		return janein(k.RegistrierungOffen)
	case "erlaubte_domaenen":
		return strings.Join(k.ErlaubteDomaenen, ", ")
	case "max_anhang_mb":
		return strconv.Itoa(k.MaxAnhangMB)
	case "sitzung_tage":
		return strconv.Itoa(k.SitzungTage)
	case "such_woerterbuch":
		return k.SuchWoerterbuch
	}
	return ""
}

func janein(b bool) string {
	if b {
		return "ja"
	}
	return "nein"
}

// RegistrierungOffen, ErlaubteDomaenen und MaxAnhangBytes sind die Fragen, die
// die Handler tatsächlich stellen. Sie lesen aus dem Zwischenspeicher, damit
// eine Änderung sofort greift, ohne Neustart.
func RegistrierungOffen() bool { return wert("registrierung_offen") == "ja" }

func ErlaubteDomaenen() []string {
	var out []string
	for _, t := range strings.Split(wert("erlaubte_domaenen"), ",") {
		if t = strings.TrimSpace(strings.ToLower(t)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func MaxAnhangBytes() int64 {
	n, err := strconv.Atoi(wert("max_anhang_mb"))
	if err != nil || n <= 0 {
		return maxUploadBytes
	}
	return int64(n) << 20
}

// SitzungDauer is how long a newly issued token stays valid. Only new sign-ins
// are affected: a token already handed out carries its own expiry and cannot be
// shortened afterwards.
func SitzungDauer() time.Duration {
	n, err := strconv.Atoi(wert("sitzung_tage"))
	if err != nil || n <= 0 {
		n = 7
	}
	return time.Duration(n) * 24 * time.Hour
}

// ListEinstellungen returns every changeable setting with its description.
func (s *Server) ListEinstellungen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	// Herkunft und letzte Änderung mitliefern: ein Administrator soll sehen,
	// ob ein Wert noch aus der Datei stammt oder jemand ihn hier gesetzt hat.
	art := map[string]struct {
		von string
		am  string
	}{}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT schluessel, geaendert_von, to_char(geaendert_am, 'YYYY-MM-DD HH24:MI') FROM einstellungen`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, von, am string
			if rows.Scan(&k, &von, &am) == nil {
				art[k] = struct {
					von string
					am  string
				}{von, am}
			}
		}
	}

	speicher.RLock()
	basis := speicher.basis
	gesetzt := map[string]bool{}
	for k := range speicher.werte {
		gesetzt[k] = true
	}
	speicher.RUnlock()

	// Feste Reihenfolge: eine Map iteriert zufällig, und eine Seite, deren
	// Felder bei jedem Laden die Plätze tauschen, ist unbenutzbar.
	reihenfolge := []string{
		"registrierung_offen", "erlaubte_domaenen",
		"max_anhang_mb", "sitzung_tage", "such_woerterbuch",
	}

	liste := make([]Einstellung, 0, len(reihenfolge))
	for _, k := range reihenfolge {
		b := bekannt[k]
		e := Einstellung{
			Schluessel: k,
			Wert:       wert(k),
			Art:        b.Art,
			Titel:      b.Titel,
			Erklaerung: b.Erklaerung,
			Warnung:    b.Warnung,
			Vorgabe:    ausDatei(k, basis),
			AusDatei:   !gesetzt[k],
		}
		if a, ok := art[k]; ok {
			e.GeaendertVon, e.GeaendertAm = a.von, a.am
		}
		liste = append(liste, e)
	}
	writeJSON(w, http.StatusOK, liste)
}

type setzenReq struct {
	Schluessel string `json:"schluessel"`
	Wert       string `json:"wert"`
}

// SetzeEinstellung writes one setting.
func (s *Server) SetzeEinstellung(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	var req setzenReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	b, ok := bekannt[req.Schluessel]
	if !ok {
		// Unbekannte Schlüssel werden abgewiesen statt gespeichert. Sonst
		// könnte man beliebige Zeilen ablegen, denen ein späterer Leser traut.
		writeErr(w, http.StatusBadRequest, "unbekannte Einstellung")
		return
	}

	wertNeu := strings.TrimSpace(req.Wert)
	switch b.Art {
	case "janein":
		if wertNeu != "ja" && wertNeu != "nein" {
			writeErr(w, http.StatusBadRequest, "erwartet ja oder nein")
			return
		}
	case "zahl":
		n, err := strconv.Atoi(wertNeu)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "erwartet eine Zahl größer null")
			return
		}
		if req.Schluessel == "max_anhang_mb" && n > 2048 {
			writeErr(w, http.StatusBadRequest, "höchstens 2048 MB")
			return
		}
		if req.Schluessel == "sitzung_tage" && n > 365 {
			writeErr(w, http.StatusBadRequest, "höchstens 365 Tage")
			return
		}
	case "text":
		if req.Schluessel == "such_woerterbuch" {
			switch wertNeu {
			case "german", "english", "simple":
			default:
				writeErr(w, http.StatusBadRequest, "erwartet german, english oder simple")
				return
			}
		}
	}

	// Die Selbstregistrierung darf nicht abgeschaltet werden, solange es noch
	// gar kein Konto gibt -- sonst käme niemand mehr hinein, denn das erste
	// Konto wird zum Administrator.
	if req.Schluessel == "registrierung_offen" && wertNeu == "nein" {
		var anzahl int
		_ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&anzahl)
		if anzahl == 0 {
			writeErr(w, http.StatusBadRequest, "es existiert noch kein Konto -- das erste muss sich anlegen können")
			return
		}
	}

	var name string
	_ = s.Pool.QueryRow(r.Context(), `SELECT name FROM users WHERE id=$1`, uid).Scan(&name)

	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO einstellungen (schluessel, wert, geaendert_von) VALUES ($1, $2, $3)
		 ON CONFLICT (schluessel) DO UPDATE
		 SET wert = EXCLUDED.wert, geaendert_am = now(), geaendert_von = EXCLUDED.geaendert_von`,
		req.Schluessel, wertNeu, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}

	speicher.Lock()
	speicher.werte[req.Schluessel] = wertNeu
	speicher.Unlock()

	s.spurAusRequest(r, AktEinstellung, "einstellung", req.Schluessel, b.Titel,
		map[string]interface{}{"wert": wertNeu})
	writeJSON(w, http.StatusOK, map[string]string{"wert": wertNeu})
}

// LoescheEinstellung removes the override and lets the file value take over
// again. Useful when someone is unsure what they changed.
func (s *Server) LoescheEinstellung(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	schluessel := r.URL.Query().Get("schluessel")
	if _, ok := bekannt[schluessel]; !ok {
		writeErr(w, http.StatusBadRequest, "unbekannte Einstellung")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`DELETE FROM einstellungen WHERE schluessel=$1`, schluessel); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht zurückgesetzt werden")
		return
	}
	speicher.Lock()
	delete(speicher.werte, schluessel)
	speicher.Unlock()

	s.spurAusRequest(r, AktEinstellungZurueck, "einstellung", schluessel, "", nil)
	writeJSON(w, http.StatusOK, map[string]string{"wert": wert(schluessel)})
}

// SystemZustand is the read-only half of the settings page: what cannot be
// changed here, plus the numbers an administrator wants at a glance.
func (s *Server) SystemZustand(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	ctx := r.Context()

	zahl := func(sql string) int {
		var n int
		_ = s.Pool.QueryRow(ctx, sql).Scan(&n)
		return n
	}

	speicher.RLock()
	k := speicher.basis
	speicher.RUnlock()

	var dbGroesse string
	_ = s.Pool.QueryRow(ctx, `SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbGroesse)

	z := lizenz.Aktuell()
	frei := []string{}
	for _, f := range lizenz.Alle {
		if lizenz.Frei(f) {
			frei = append(frei, string(f))
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lizenz": map[string]interface{}{
			"gueltig":        z.Gueltig,
			"inhaber":        z.Inhaber,
			"laeuftAb":       z.LaeuftAb,
			"grund":          z.Grund,
			"freigeschaltet": frei,
			"alle":           len(lizenz.Alle),
		},
		"zahlen": map[string]int{
			"konten":        zahl(`SELECT count(*) FROM users`),
			"admins":        zahl(`SELECT count(*) FROM users WHERE role='admin'`),
			"seiten":        zahl(`SELECT count(*) FROM pages WHERE deleted_at IS NULL`),
			"papierkorb":    zahl(`SELECT count(*) FROM pages WHERE deleted_at IS NOT NULL`),
			"versionen":     zahl(`SELECT count(*) FROM page_versions`),
			"anhaenge":      zahl(`SELECT count(*) FROM attachments`),
			"kommentare":    zahl(`SELECT count(*) FROM kommentare WHERE geloescht_am IS NULL`),
			"spureintraege": zahl(`SELECT count(*) FROM pruefspur`),
			"ohneSuchtext":  zahl(`SELECT count(*) FROM pages WHERE length(trim(content_text))=0 AND deleted_at IS NULL`),
		},
		"nurInDerDatei": map[string]interface{}{
			"port":             k.Port,
			"datenVerzeichnis": k.DatenVerzeich,
			"oeffentlicheUrl":  k.OeffentlicheURL,
			"ldapAktiv":        k.LDAPAktiv,
			"ldapServer":       k.LDAPServer,
			"oidcAktiv":        k.OIDCAktiv,
			"oidcAussteller":   k.OIDCAussteller,
		},
		"datenbankGroesse": dbGroesse,
		"warnungen":        k.Warnungen(),
	})
}

// IndexNeuAufbauen re-derives the search text for every page. Needed after the
// dictionary changed, and useful when the index is suspected of being stale.
func (s *Server) IndexNeuAufbauen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	// Leeren und neu ziehen. Der Nachzug läuft ohnehin in Stapeln, deshalb
	// hält das auch eine große Instanz nicht lange auf.
	if _, err := s.Pool.Exec(r.Context(), `UPDATE pages SET content_text=''`); err != nil {
		writeErr(w, http.StatusInternalServerError, "Index konnte nicht geleert werden")
		return
	}
	s.IndexNachziehen(r.Context())

	var offen int
	_ = s.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM pages WHERE length(trim(content_text))=0 AND deleted_at IS NULL`).Scan(&offen)

	s.spurAusRequest(r, AktIndexNeu, "system", "suchindex", "", nil)
	writeJSON(w, http.StatusOK, map[string]int{"ohneSuchtext": offen})
}
