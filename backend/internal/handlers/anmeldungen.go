// Login attempts: recording every one of them, and reading them back.
//
// The attempts already stood in the audit trail before this file existed, but
// only as two bare rows with an address and an IP. That answers "did somebody
// try" and nothing beyond it. What an administrator actually asks after a
// suspicious night is: from where, over which way in, and why it failed.
//
// So the entries stay in the audit trail (one record of what happened, no
// second place that could disagree with it) and this file adds two things: a
// single writer that every sign-in path uses, so the extra fields cannot be
// forgotten in one of them, and a read endpoint of its own.
//
// That endpoint is free of charge, unlike the rest of the audit trail. Who is
// knocking at the door is not a reporting feature, it belongs to running the
// thing at all.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// Wege, over which one can sign in. Written into the entry so that a failed
// attempt at the directory is not mistaken for one at the password form.
const (
	WegPasswort = "passwort"
	WegLDAP     = "ldap"
	WegSSO      = "sso"
)

// Gruende for a failure. Deliberately more precise than the answer the caller
// receives: the response says "invalid credentials" in every case so that
// nobody can find out through it which addresses exist. The trail is read by
// administrators only, and there the distinction is the whole point, because a
// hundred attempts against one existing account look different from a hundred
// against a hundred invented ones.
const (
	GrundUnbekannt = "Kennung unbekannt"
	GrundPasswort  = "Passwort falsch"
	GrundGesperrt  = "Konto gesperrt"
	GrundVerzeichn = "Verzeichnis hat abgelehnt"
)

// anmeldeSpur records one attempt.
//
// u may be nil, which is the normal case for a failure: there is nothing to
// refer to then except the string that was typed in.
func (s *Server) anmeldeSpur(r *http.Request, weg, kennung, grund string, u *models.User) {
	e := models.Spureintrag{
		Aktion:      AktAnmeldungFehl,
		AkteurEmail: kennung,
		ObjektArt:   "konto",
		IP:          absenderIP(r),
	}
	einzelheiten := map[string]string{
		"weg":     weg,
		"browser": browserKurz(r),
	}
	if grund != "" {
		einzelheiten["grund"] = grund
	}
	if u != nil {
		e.Aktion = AktAnmeldung
		e.AkteurID = u.ID
		e.AkteurName = u.Name
		e.AkteurEmail = u.Email
		e.ObjektID = u.ID
		e.ObjektTitel = u.Name
		// What was typed is kept even on success, because signing in with the
		// user name and with the address are two different events and only this
		// field tells them apart.
		if kennung != "" && !strings.EqualFold(kennung, u.Email) {
			einzelheiten["kennung"] = kennung
		}
	}
	if b, err := json.Marshal(einzelheiten); err == nil {
		e.Details = b
	}
	s.spur(r.Context(), e)
}

// browserKurz keeps the user agent short. The full string of a current browser
// runs to two hundred characters of which two matter, and an audit trail that
// has to be scrolled sideways is not read.
func browserKurz(r *http.Request) string {
	ua := strings.TrimSpace(r.UserAgent())
	if ua == "" {
		return ""
	}
	if len(ua) > 300 {
		ua = ua[:300]
	}
	return ua
}

// Anmeldeversuch is one attempt as the interface receives it. Flat, because
// every field is shown in its own column and nothing here needs nesting.
type Anmeldeversuch struct {
	Zeitpunkt time.Time `json:"zeitpunkt"`
	Erfolg    bool      `json:"erfolg"`
	Kennung   string    `json:"kennung"`
	Name      string    `json:"name"`
	KontoID   string    `json:"kontoId"`
	IP        string    `json:"ip"`
	Weg       string    `json:"weg"`
	Grund     string    `json:"grund"`
	Browser   string    `json:"browser"`
}

// Herkunft is one address, summed up. An attack shows itself in this table and
// not in the single entries: fifty rows scroll past, one line saying "fifty
// failures from one address" does not.
type Herkunft struct {
	IP       string    `json:"ip"`
	Versuche int       `json:"versuche"`
	Fehl     int       `json:"fehl"`
	Konten   int       `json:"konten"`
	Letzter  time.Time `json:"letzter"`
}

// ListAnmeldungen returns the attempts, newest first, plus a summary.
//
// Administrators only. Not because the data is secret in itself, but because
// every entry names an address and a way in, and put together that is a map of
// who works here and when.
func (s *Server) ListAnmeldungen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.isAdmin(ctx, middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	q := r.URL.Query()
	grenze := 200
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 1000 {
		grenze = n
	}
	tage := 30
	if n, err := strconv.Atoi(q.Get("tage")); err == nil && n >= 0 && n <= 365 {
		tage = n
	}
	// 0 means "everything there is". Written as a large number rather than as a
	// second query, so the filter stays one expression.
	if tage == 0 {
		tage = 36500
	}

	// nur=fehl or nur=erfolg. Anything else is no filter.
	nurFehl := q.Get("nur") == "fehl"
	nurErfolg := q.Get("nur") == "erfolg"

	rows, err := s.Pool.Query(ctx, `
		SELECT zeitpunkt, aktion, akteur_name, akteur_email,
		       coalesce(akteur_id::text, ''), coalesce(ip, ''),
		       coalesce(details->>'weg', ''), coalesce(details->>'grund', ''),
		       coalesce(details->>'browser', ''), coalesce(details->>'kennung', '')
		  FROM pruefspur
		 WHERE aktion IN ($1, $2)
		   AND zeitpunkt > now() - ($3 || ' days')::interval
		   AND ($4 = false OR aktion = $2)
		   AND ($5 = false OR aktion = $1)
		   AND ($6 = '' OR ip = $6)
		 ORDER BY zeitpunkt DESC, id DESC
		 LIMIT $7`,
		AktAnmeldung, AktAnmeldungFehl, strconv.Itoa(tage),
		nurFehl, nurErfolg, strings.TrimSpace(q.Get("ip")), grenze)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Anmeldungen konnten nicht gelesen werden")
		return
	}
	defer rows.Close()

	versuche := []Anmeldeversuch{}
	for rows.Next() {
		var v Anmeldeversuch
		var aktion, email, getippt string
		if rows.Scan(&v.Zeitpunkt, &aktion, &v.Name, &email, &v.KontoID,
			&v.IP, &v.Weg, &v.Grund, &v.Browser, &getippt) != nil {
			continue
		}
		v.Erfolg = aktion == AktAnmeldung
		// On a failure the typed string sits in akteur_email, on a success the
		// account's address does, and what was typed lands in the details only
		// when it differed. Both cases end in the same column.
		v.Kennung = email
		if getippt != "" {
			v.Kennung = getippt + " → " + email
		}
		// Entries from before this file have no way recorded. Guessing one
		// would be wrong; an empty field is honest.
		versuche = append(versuche, v)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"versuche":        versuche,
		"zusammenfassung": s.anmeldeZahlen(ctx),
		"herkunft":        s.anmeldeHerkunft(ctx),
		"tage":            tage,
	})
}

// anmeldeZahlen counts the last day and the last week in one query. One pass
// over the table instead of six.
func (s *Server) anmeldeZahlen(ctx context.Context) map[string]interface{} {
	var e24, f24, e7, f7, ips24 int
	var letzteA, letzterF *time.Time
	_ = s.Pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE aktion=$1 AND zeitpunkt > now() - interval '24 hours'),
		       count(*) FILTER (WHERE aktion=$2 AND zeitpunkt > now() - interval '24 hours'),
		       count(*) FILTER (WHERE aktion=$1 AND zeitpunkt > now() - interval '7 days'),
		       count(*) FILTER (WHERE aktion=$2 AND zeitpunkt > now() - interval '7 days'),
		       count(DISTINCT ip) FILTER (WHERE zeitpunkt > now() - interval '24 hours'),
		       max(zeitpunkt) FILTER (WHERE aktion=$1),
		       max(zeitpunkt) FILTER (WHERE aktion=$2)
		  FROM pruefspur
		 WHERE aktion IN ($1, $2)`,
		AktAnmeldung, AktAnmeldungFehl).
		Scan(&e24, &f24, &e7, &f7, &ips24, &letzteA, &letzterF)

	return map[string]interface{}{
		"erfolge24h":         e24,
		"fehl24h":            f24,
		"erfolge7t":          e7,
		"fehl7t":             f7,
		"adressen24h":        ips24,
		"letzteAnmeldung":    letzteA,
		"letzterFehlversuch": letzterF,
	}
}

// anmeldeHerkunft groups the last week by address, the ones with the most
// failures first.
func (s *Server) anmeldeHerkunft(ctx context.Context) []Herkunft {
	liste := []Herkunft{}
	rows, err := s.Pool.Query(ctx, `
		SELECT coalesce(ip, ''), count(*), count(*) FILTER (WHERE aktion=$2),
		       count(DISTINCT lower(akteur_email)), max(zeitpunkt)
		  FROM pruefspur
		 WHERE aktion IN ($1, $2)
		   AND zeitpunkt > now() - interval '7 days'
		   AND ip IS NOT NULL AND ip <> ''
		 GROUP BY ip
		 ORDER BY count(*) FILTER (WHERE aktion=$2) DESC, count(*) DESC
		 LIMIT 15`, AktAnmeldung, AktAnmeldungFehl)
	if err != nil {
		return liste
	}
	defer rows.Close()
	for rows.Next() {
		var h Herkunft
		if rows.Scan(&h.IP, &h.Versuche, &h.Fehl, &h.Konten, &h.Letzter) == nil {
			liste = append(liste, h)
		}
	}
	return liste
}
