// Audit trail: recording what happened, and reading it back.
//
// Two rules shape this file.
//
// Writing never fails a request. If the trail cannot be written, the user's
// action still goes through and the problem lands in the log. The opposite —
// refusing to save a page because an audit row failed — turns a bookkeeping
// problem into an outage.
//
// Writing is not gated by the license. The trail is recorded on every
// installation; only *reading* it needs the Pruefspur feature. Recording only
// while licensed would leave a hole in the history exactly over the unlicensed
// period, which is the one thing an audit trail must not have.
package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// Aktionsnamen. Constants rather than free text so a filter in the interface
// and the writing side cannot drift apart.
const (
	AktAnmeldung      = "anmeldung"
	AktAnmeldungFehl  = "anmeldung.fehlgeschlagen"
	AktAbmeldung      = "abmeldung"
	AktKontoAngelegt  = "konto.angelegt"
	AktKontoGeloescht = "konto.geloescht"
	AktRolleGeaendert = "konto.rolle"

	AktSeiteAngelegt  = "seite.angelegt"
	AktSeiteGeaendert = "seite.geaendert"
	AktSeiteGeloescht = "seite.geloescht" // in den Papierkorb
	AktSeiteEntfernt  = "seite.entfernt"  // endgültig
	AktSeiteWieder    = "seite.wiederhergestellt"
	AktVersionZurueck = "version.zurueckgeholt"
	AktFreigabe       = "freigabe.erteilt"
	AktFreigabeWeg    = "freigabe.entzogen"
	AktOeffentlichAn  = "oeffentlich.an"
	AktOeffentlichAus = "oeffentlich.aus"
	AktAnhangHoch     = "anhang.hochgeladen"
	AktAnhangWeg      = "anhang.entfernt"
	AktLizenzGeladen  = "lizenz.geladen"

	AktKommentar          = "kommentar.angelegt"
	AktKommentarGeaendert = "kommentar.geaendert"
	AktKommentarGeloescht = "kommentar.geloescht"
	AktKommentarErledigt  = "kommentar.erledigt"

	AktEinstellung        = "einstellung.geaendert"
	AktEinstellungZurueck = "einstellung.zurueckgesetzt"
	AktIndexNeu           = "suchindex.neu"
	AktS3Test             = "objektspeicher.getestet"
	AktVorlageAn          = "vorlage.gesetzt"
	AktVorlageAus         = "vorlage.aufgehoben"
	AktExport             = "space.exportiert"
	AktGruppeAngelegt     = "gruppe.angelegt"
	AktGruppeGeloescht    = "gruppe.geloescht"
	AktGruppeBeitritt     = "gruppe.beigetreten"
	AktGruppeAustritt     = "gruppe.ausgetreten"
	AktSpaceRecht         = "spacerecht.erteilt"
	AktSpaceRechtWeg      = "spacerecht.entzogen"
	AktAnhangIndex        = "anhangindex.nachgezogen"
)

// spur writes one entry. Callers pass what they know; empty fields stay empty.
//
// It takes a context rather than the request so it can also be used from
// startup code, and it deliberately does not return an error: there is nothing
// a caller could usefully do with one.
func (s *Server) spur(ctx context.Context, e models.Spureintrag) {
	details := e.Details
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO pruefspur (akteur_id, akteur_name, akteur_email, aktion,
		                        objekt_art, objekt_id, objekt_titel, details, ip)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`,
		nullWennLeer(e.AkteurID), e.AkteurName, e.AkteurEmail, e.Aktion,
		e.ObjektArt, e.ObjektID, e.ObjektTitel, string(details), e.IP)
	if err != nil {
		log.Printf("Prüfspur (%s): %v", e.Aktion, err)
	}
}

// spurAusRequest is the common case: the acting user comes from the session and
// the address from the request. It looks the name up once per call, which is a
// cheap price for a trail that stays readable after the account is gone.
func (s *Server) spurAusRequest(r *http.Request, aktion, art, id, titel string, details map[string]interface{}) {
	uid := middleware.UserID(r)

	var name, email string
	if uid != "" {
		_ = s.Pool.QueryRow(r.Context(),
			`SELECT name, email FROM users WHERE id=$1`, uid).Scan(&name, &email)
	}

	var roh json.RawMessage
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			roh = b
		}
	}

	s.spur(r.Context(), models.Spureintrag{
		AkteurID:    uid,
		AkteurName:  name,
		AkteurEmail: email,
		Aktion:      aktion,
		ObjektArt:   art,
		ObjektID:    id,
		ObjektTitel: titel,
		Details:     roh,
		IP:          absenderIP(r),
	})
}

// absenderIP prefers the address chi's RealIP middleware resolved. Behind the
// nginx in front of Nexora the socket address is always the proxy, which would
// make every entry useless.
func absenderIP(r *http.Request) string {
	if r.RemoteAddr == "" {
		return ""
	}
	// RemoteAddr is host:port; the port is noise in an audit trail.
	if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

func nullWennLeer(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ListPruefspur returns the trail, newest first.
//
// Admin only, and on top of that behind the Pruefspur feature. The check is
// explicit here rather than left to the route, because this handler is the one
// place where every user's actions become visible to someone else — that
// deserves to be readable at the point of use.
func (s *Server) ListPruefspur(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	q := r.URL.Query()
	grenze := 200
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 1000 {
		grenze = n
	}

	// Optional filters. Empty means "no filter", which is why each one is a
	// separate OR against the empty string rather than a built-up query.
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, zeitpunkt, coalesce(akteur_id::text, ''), akteur_name, akteur_email,
		        aktion, objekt_art, objekt_id, objekt_titel, details, ip
		 FROM pruefspur
		 WHERE ($1 = '' OR aktion = $1)
		   AND ($2 = '' OR akteur_id::text = $2)
		   AND ($3 = '' OR objekt_id = $3)
		 ORDER BY zeitpunkt DESC, id DESC
		 LIMIT $4`,
		q.Get("aktion"), q.Get("akteur"), q.Get("objekt"), grenze)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Prüfspur konnte nicht gelesen werden")
		return
	}
	defer rows.Close()

	list := []models.Spureintrag{}
	for rows.Next() {
		var e models.Spureintrag
		var details []byte
		if err := rows.Scan(&e.ID, &e.Zeitpunkt, &e.AkteurID, &e.AkteurName, &e.AkteurEmail,
			&e.Aktion, &e.ObjektArt, &e.ObjektID, &e.ObjektTitel, &details, &e.IP); err == nil {
			e.Details = json.RawMessage(details)
			list = append(list, e)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// PruefspurAktionen lists the action names that actually occur, so the filter in
// the interface offers real values instead of a hand-kept list that drifts.
func (s *Server) PruefspurAktionen(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT aktion, count(*) FROM pruefspur GROUP BY aktion ORDER BY count(*) DESC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Prüfspur konnte nicht gelesen werden")
		return
	}
	defer rows.Close()

	type zeile struct {
		Aktion string `json:"aktion"`
		Anzahl int    `json:"anzahl"`
	}
	list := []zeile{}
	for rows.Next() {
		var z zeile
		if err := rows.Scan(&z.Aktion, &z.Anzahl); err == nil {
			list = append(list, z)
		}
	}
	writeJSON(w, http.StatusOK, list)
}
