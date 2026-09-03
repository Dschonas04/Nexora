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
	"sicherung_token": {
		Art:        "text",
		Titel:      "Losungswort für die Sicherung",
		Erklaerung: "Leer heißt: die Sicherung gibt es nur aus dem Panel heraus, mit Anmeldung. Gesetzt heißt: ein Skript kann sie abholen, indem es dieses Wort mitschickt.",
		Warnung:    "Dieses Wort wiegt schwerer als jedes andere hier: es gibt den gesamten Bestand heraus, samt Passwort-Hashes und Freigabe-Tokens. Verwaltet wird es unter Wartung.",
	},
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
		Warnung:    "Der nginx davor hat eine eigene Grenze. Ein hier erhöhter Wert bringt nichts, solange client_max_body_size im vorgeschalteten nginx kleiner ist: der bricht die Übertragung schon ab, bevor Nexora sie überhaupt sieht. Was von beidem wirklich gilt, sagt die Messung weiter unten.",
	},
	"sitzung_stunden": {
		Art:        "zahl",
		Titel:      "Gültigkeit einer Anmeldung (Stunden)",
		Erklaerung: "Eine Sitzung, die benutzt wird, verlängert sich selbst, sobald die Hälfte der Zeit verstrichen ist. Wer täglich arbeitet, wird also nicht abgemeldet; wer wegbleibt, schon.",
		Warnung:    "Wirkt auf neue und auf verlängerte Sitzungen. Bereits offene behalten ihre Frist bis zur nächsten Verlängerung.",
	},
	"papierkorb_tage": {
		Art:        "zahl",
		Titel:      "Papierkorb leert sich nach (Tagen)",
		Erklaerung: "0 heißt: nie von selbst. Gilt für die ganze Instanz und läuft stündlich; gelöschte Seiten verschwinden dann endgültig, samt ihrer Anhänge.",
		Warnung:    "Endgültig heißt endgültig. Danach hilft nur noch eine Sicherung der Datenbank.",
	},
	"such_woerterbuch": {
		Art:        "text",
		Titel:      "Wörterbuch der Volltextsuche",
		Erklaerung: "german, english oder simple. simple stemmt gar nicht und trifft dafür in keiner Sprache daneben.",
		Warnung:    "Eine Änderung wirkt erst nach einem Neuaufbau des Suchindex. Die Spalte wurde mit dem alten Wörterbuch erzeugt.",
	},
	"echtzeit": {
		Art:        "janein",
		Titel:      "Gemeinsames Bearbeiten",
		Erklaerung: "Mehrere Konten schreiben gleichzeitig an derselben Seite, jeder sieht die Änderungen der anderen sofort. Wer mitschreiben darf, entscheidet die Freigabe der Seite: nur wer sie bearbeiten darf.",
		Warnung:    "Aus heißt nicht abgeschaltet, sondern zurück auf das alte Verhalten: die Seite wird beim Speichern ganz geschrieben, und wer zuletzt speichert, gewinnt. Der Hinweis auf den Konflikt bleibt.",
	},
	"seitenbreite": {
		Art:        "auswahl",
		Titel:      "Breite einer Seite",
		Erklaerung: "Wie breit der Text steht, wenn eine Seite nichts eigenes sagt. „voll“ nutzt das ganze Fenster, „normal“ hält einen schmalen Satzspiegel wie in einem Buch.",
		Warnung:    "Eine Seite, an der jemand die Breite selbst gesetzt hat, behält sie. Diese Angabe gilt für alle übrigen.",
	},
	"design_grundton": {
		Art:        "auswahl",
		Titel:      "Grundton",
		Erklaerung: "Gilt für alle Konten dieser Instanz, nicht nur für das eigene.",
	},
	"design_akzent": {
		Art:        "farbe",
		Titel:      "Akzentfarbe",
		Erklaerung: "Wird für Verknüpfungen, ausgewählte Einträge und Knöpfe benutzt.",
	},
}

// grundtoene are the permitted values for design_grundton. A fixed list rather
// than free input: the colour values behind them live in the stylesheet, and an
// unknown name would produce an interface without colours.
var grundtoene = map[string]bool{"weiss": true, "grau": true, "dunkel": true}

// speicher keeps the values in memory.
var speicher struct {
	sync.RWMutex
	werte map[string]string
	basis config.Konfig // what came from the file, as a fallback
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
	case "papierkorb_tage":
		return strconv.Itoa(k.PapierkorbTage)
	case "sitzung_stunden":
		return strconv.Itoa(k.SitzungStunden)
	case "such_woerterbuch":
		return k.SuchWoerterbuch
	case "echtzeit":
		return "ja"
	case "seitenbreite":
		return "voll"
	case "design_grundton":
		return "grau"
	case "design_akzent":
		return "#2383e2"
	}
	return ""
}

func janein(b bool) string {
	if b {
		return "ja"
	}
	return "nein"
}

// RegistrierungOffen, ErlaubteDomaenen and MaxAnhangBytes are the questions the
// handlers actually ask. They read from the cache so a change takes effect at
// once, without a restart.
func RegistrierungOffen() bool { return wert("registrierung_offen") == "ja" }

// echtzeitAn reports whether collaborative editing is allowed. The UI asks
// first and the realtime layer checks again: a disabled feature that can be
// abused via an open path is not considered off.
func (s *Server) echtzeitAn() bool { return wert("echtzeit") == "ja" }

// Seitenbreite is the default width for pages that do not specify one. An
// unknown value would create a CSS class that does not exist and therefore
// fall back to the narrow layout — so we validate it here rather than in the
// client.
func Seitenbreite() string {
	b := wert("seitenbreite")
	if !breiten[b] || b == "" {
		return "voll"
	}
	return b
}

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

// PapierkorbTage is the deadline after which a deleted page disappears by
// itself. 0 means never, and then the trash stays until somebody empties it.
func PapierkorbTage() int {
	n, err := strconv.Atoi(wert("papierkorb_tage"))
	if err != nil || n < 0 {
		return 30
	}
	return n
}

// EinfuhrGrenze is the ceiling for one import as a whole.
//
// Derived from the attachment limit rather than a setting of its own: an archive
// contains many files, and the attachment limit still applies to each one inside
// it. A second knob governing the same thing in the large would be one more
// adjustment that eventually contradicts the first.
func EinfuhrGrenze() int64 {
	return MaxAnhangBytes() * 8
}

// SitzungDauer is how long a newly issued token stays valid. Only new sign-ins
// are affected: a token already handed out carries its own expiry and cannot be
// shortened afterwards.
func SitzungDauer() time.Duration {
	return time.Duration(SitzungStunden()) * time.Hour
}

// SitzungStunden is the same figure in hours. The database counts in hours, not
// in nanoseconds.
func SitzungStunden() int {
	n, err := strconv.Atoi(wert("sitzung_stunden"))
	if err != nil || n <= 0 {
		return 12
	}
	return n
}

// istHexFarbe checks #rrggbb. Deliberately narrow: the value is written into a
// CSS variable in the browser, and whatever ends up there has to be harmless.
func istHexFarbe(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Design serves the appearance to every signed-in account, not only to admins.
// Without that an ordinary user could never see the configured colours, since
// the settings page itself is closed to them.
func (s *Server) Design(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"grundton": wert("design_grundton"),
		"akzent":   wert("design_akzent"),
		// Die Breite steht hier und nicht bei den Einstellungen: die sind der
		// Verwaltung vorbehalten, und diese Angabe braucht jeder, der eine
		// Seite ansieht.
		"seitenbreite": Seitenbreite(),
	})
}

// ListEinstellungen returns every changeable setting with its description.
func (s *Server) ListEinstellungen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	// Origin and last change travel along: an administrator should see whether a
	// value still comes from the file or whether somebody set it here.
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

	// A fixed order: a map iterates at random, and a page whose fields swap
	// places on every load is unusable.
	reihenfolge := []string{
		"registrierung_offen", "erlaubte_domaenen",
		"max_anhang_mb", "sitzung_stunden", "papierkorb_tage", "such_woerterbuch",
		"echtzeit",
		"seitenbreite",
		"design_grundton", "design_akzent",
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
		// Unknown keys are rejected rather than stored. Otherwise arbitrary rows
		// could be placed here that a later reader would trust.
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
		// Zero is only a value for the trash, where it means "never by itself".
		// For an attachment limit or a session lifetime it would be an instance
		// that accepts nothing or signs nobody in.
		if err != nil || n < 0 || (n == 0 && req.Schluessel != "papierkorb_tage") {
			writeErr(w, http.StatusBadRequest, "erwartet eine Zahl größer null")
			return
		}
		if req.Schluessel == "papierkorb_tage" && n > 3650 {
			writeErr(w, http.StatusBadRequest, "höchstens 3650 Tage")
			return
		}
		if req.Schluessel == "max_anhang_mb" && n > 2048 {
			writeErr(w, http.StatusBadRequest, "höchstens 2048 MB")
			return
		}
		// An upper bound so nobody accidentally sets a session to years: 8760
		// hours are one year, and beyond that the figure is a typo in practice.
		if req.Schluessel == "sitzung_stunden" && n > 8760 {
			writeErr(w, http.StatusBadRequest, "höchstens 8760 Stunden, das ist ein Jahr")
			return
		}
	case "auswahl":
		if req.Schluessel == "design_grundton" && !grundtoene[wertNeu] {
			writeErr(w, http.StatusBadRequest, "erwartet weiss, grau oder dunkel")
			return
		}
		// Die leere Breite gibt es nur an einer SEITE ("wie die Instanz es
		// vorgibt"); als Vorgabe selbst wäre sie ein Verweis auf sich.
		if req.Schluessel == "seitenbreite" && (wertNeu == "" || !breiten[wertNeu]) {
			writeErr(w, http.StatusBadRequest, "erwartet normal, breit oder voll")
			return
		}
	case "farbe":
		// Only #rrggbb. Anything else would land unchecked in a CSS variable, and
		// a string with brackets would be a way in.
		if !istHexFarbe(wertNeu) {
			writeErr(w, http.StatusBadRequest, "erwartet eine Farbe wie #2383e2")
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

	// Self-registration must not be switched off while there is no account at
	// all, or nobody could get in any more, since the first account becomes the
	// administrator.
	if req.Schluessel == "registrierung_offen" && wertNeu == "nein" {
		var anzahl int
		_ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&anzahl)
		if anzahl == 0 {
			writeErr(w, http.StatusBadRequest, "es existiert noch kein Konto. Das erste muss sich anlegen können.")
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

// einstellungSchreiben legt einen Wert ab und zieht den Zwischenspeicher nach.
//
// Herausgezogen, weil neben SetzeEinstellung inzwischen ein zweiter Weg schreibt
// (das erzeugte Losungswort für die Kennzahlen). Zweimal dasselbe INSERT wäre
// zweimal die Gelegenheit, den Zwischenspeicher zu vergessen, und dann stünde
// der neue Wert in der Datenbank und der alte gälte weiter.
func (s *Server) einstellungSchreiben(ctx context.Context, schluessel, wertNeu string) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO einstellungen (schluessel, wert, geaendert_von) VALUES ($1, $2, $3)
		 ON CONFLICT (schluessel) DO UPDATE
		 SET wert = EXCLUDED.wert, geaendert_am = now(), geaendert_von = EXCLUDED.geaendert_von`,
		schluessel, wertNeu, "")
	if err != nil {
		return err
	}
	speicher.Lock()
	speicher.werte[schluessel] = wertNeu
	speicher.Unlock()
	return nil
}

// speicherOeffentlicheURL ist die Adresse, unter der die Instanz erreichbar
// ist, soweit sie jemand eingetragen hat.
func speicherOeffentlicheURL() string {
	speicher.RLock()
	defer speicher.RUnlock()
	return speicher.basis.OeffentlicheURL
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

	var dbGroesse, dbVersion string
	_ = s.Pool.QueryRow(ctx, `SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbGroesse)
	_ = s.Pool.QueryRow(ctx, `SHOW server_version`).Scan(&dbVersion)

	// The largest tables, so it is visible where the space goes. Without that
	// "8711 kB" stays a number that says nothing.
	type tabelle struct {
		Name   string `json:"name"`
		Zeilen int64  `json:"zeilen"`
		Platz  string `json:"platz"`
	}
	tabellen := []tabelle{}
	if rows, err := s.Pool.Query(ctx, `
		SELECT c.relname,
		       coalesce(s.n_live_tup, 0),
		       pg_size_pretty(pg_total_relation_size(c.oid))
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		WHERE c.relkind = 'r' AND n.nspname = 'public'
		ORDER BY pg_total_relation_size(c.oid) DESC
		LIMIT 12`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var tb tabelle
			if rows.Scan(&tb.Name, &tb.Zeilen, &tb.Platz) == nil {
				tabellen = append(tabellen, tb)
			}
		}
	}

	// Attachments take space on disk, not in the database, so the total belongs
	// in a line of its own.
	var anhangBytes int64
	_ = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(size), 0) FROM attachments`).Scan(&anhangBytes)

	// Who signed in last and how many failed attempts there were: the two
	// questions one asks a security overview first.
	var letzteAnmeldung, letzterFehlversuch string
	_ = s.Pool.QueryRow(ctx,
		`SELECT to_char(max(zeitpunkt), 'YYYY-MM-DD HH24:MI') FROM pruefspur WHERE aktion=$1`,
		AktAnmeldung).Scan(&letzteAnmeldung)
	_ = s.Pool.QueryRow(ctx,
		`SELECT to_char(max(zeitpunkt), 'YYYY-MM-DD HH24:MI') FROM pruefspur WHERE aktion=$1`,
		AktAnmeldungFehl).Scan(&letzterFehlversuch)

	fehlversuche24 := zahl(`SELECT count(*) FROM pruefspur
	                        WHERE aktion='` + AktAnmeldungFehl + `' AND zeitpunkt > now() - interval '24 hours'`)

	// Admins by name: a number alone does not answer who can actually look
	// into everything.
	type konto struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	admins := []konto{}
	if rows, err := s.Pool.Query(ctx,
		`SELECT name, email FROM users WHERE role='admin' ORDER BY created_at`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var a konto
			if rows.Scan(&a.Name, &a.Email) == nil {
				admins = append(admins, a)
			}
		}
	}

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
		"datenbank": map[string]interface{}{
			"groesse":  dbGroesse,
			"version":  dbVersion,
			"tabellen": tabellen,
		},
		"anhaengeBytes": anhangBytes,
		"sicherheit": map[string]interface{}{
			"admins":             admins,
			"letzteAnmeldung":    letzteAnmeldung,
			"letzterFehlversuch": letzterFehlversuch,
			"fehlversuche24h":    fehlversuche24,
			"registrierungOffen": RegistrierungOffen(),
			"erlaubteDomaenen":   ErlaubteDomaenen(),
			"sitzungStunden":     SitzungStunden(),
		},
		"warnungen": k.Warnungen(),
		// The services this one talks to. Not the Docker stack itself, which
		// Nexora does not see, see verbund.go.
		"verbund": s.verbund(ctx),
	})
}

// IndexNeuAufbauen re-derives the search text for every page. Needed after the
// dictionary changed, and useful when the index is suspected of being stale.
func (s *Server) IndexNeuAufbauen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	// Empty it and pull it again. The refill runs in batches anyway, so this
	// does not hold up a large instance for long.
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
