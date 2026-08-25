// Groups and space permissions.
//
// Per-page sharing does not scale: letting fourteen colleagues into an area
// means fourteen clicks per page. A group answers that, and the space is the
// level where it is granted.
//
// Groups belong to the installation, not to an account. A department is not a
// private matter, and two people who mean the same group should mean the same
// group.
package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

type Gruppe struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Beschreibung string    `json:"beschreibung"`
	ErstelltAm   time.Time `json:"erstelltAm"`
	Mitglieder   int       `json:"mitglieder"`
}

type Mitglied struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Rolle string `json:"rolle"`
	Drin  bool   `json:"drin"`
}

// ListGruppen returns every group with its member count.
//
// Readable by every signed-in account, not just admins: without it nobody could
// tell which group to ask to be added to, and a space owner could not hand out
// a right without knowing the groups exist. Membership itself is only visible
// to admins, see ListMitglieder.
func (s *Server) ListGruppen(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT g.id, g.name, g.beschreibung, g.erstellt_am,
		        (SELECT count(*) FROM gruppen_mitglieder m WHERE m.gruppe_id = g.id)
		 FROM gruppen g ORDER BY g.name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []Gruppe{}
	for rows.Next() {
		var g Gruppe
		if err := rows.Scan(&g.ID, &g.Name, &g.Beschreibung, &g.ErstelltAm, &g.Mitglieder); err == nil {
			list = append(list, g)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type gruppeReq struct {
	Name         string `json:"name"`
	Beschreibung string `json:"beschreibung"`
}

// CreateGruppe legt eine Gruppe an. Admins only: eine Gruppe ist ein Begriff
// für die ganze Instanz, und wenn jeder einen erfinden darf, gibt es bald drei
// Gruppen namens "Vertrieb".
func (s *Server) CreateGruppe(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	var req gruppeReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "Name fehlt")
		return
	}

	var g Gruppe
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO gruppen (name, beschreibung) VALUES ($1, $2)
		 RETURNING id, name, beschreibung, erstellt_am`,
		req.Name, strings.TrimSpace(req.Beschreibung)).
		Scan(&g.ID, &g.Name, &g.Beschreibung, &g.ErstelltAm)
	if err != nil {
		// Der eindeutige Name ist Absicht, deshalb ist das kein Serverfehler
		// sondern eine Auskunft.
		writeErr(w, http.StatusConflict, "eine Gruppe dieses Namens gibt es bereits")
		return
	}
	s.spurAusRequest(r, AktGruppeAngelegt, "gruppe", g.ID, g.Name, nil)
	writeJSON(w, http.StatusCreated, g)
}

// DeleteGruppe entfernt eine Gruppe samt Mitgliedschaften und Rechten.
func (s *Server) DeleteGruppe(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	id := chi.URLParam(r, "id")

	// Name vor dem Löschen lesen, danach steht in der Prüfspur sonst nur
	// eine Kennung, mit der niemand mehr etwas anfangen kann.
	var name string
	_ = s.Pool.QueryRow(r.Context(), `SELECT name FROM gruppen WHERE id=$1`, id).Scan(&name)

	tag, err := s.Pool.Exec(r.Context(), `DELETE FROM gruppen WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "Gruppe nicht gefunden")
		return
	}
	s.spurAusRequest(r, AktGruppeGeloescht, "gruppe", id, name, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListMitglieder returns every account with a flag for membership.
//
// The whole directory rather than just the members: the same list drives adding
// and removing, and two endpoints for one dialog would only diverge.
func (s *Server) ListMitglieder(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT u.id, u.name, u.email, u.role,
		        EXISTS (SELECT 1 FROM gruppen_mitglieder m
		                 WHERE m.gruppe_id = $1 AND m.user_id = u.id)
		 FROM users u ORDER BY u.name`, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []Mitglied{}
	for rows.Next() {
		var m Mitglied
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Rolle, &m.Drin); err == nil {
			list = append(list, m)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type mitgliedReq struct {
	UserID string `json:"userId"`
	Drin   bool   `json:"drin"`
}

// SetzeMitglied adds or removes one account.
func (s *Server) SetzeMitglied(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}
	gruppeID := chi.URLParam(r, "id")

	var req mitgliedReq
	if err := decode(r, &req); err != nil || req.UserID == "" {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	var err error
	if req.Drin {
		// ON CONFLICT DO NOTHING: zweimal hinzufügen ist kein Fehler, sondern
		// derselbe Wunsch zweimal geäußert.
		_, err = s.Pool.Exec(r.Context(),
			`INSERT INTO gruppen_mitglieder (gruppe_id, user_id) VALUES ($1, $2)
			 ON CONFLICT DO NOTHING`, gruppeID, req.UserID)
	} else {
		_, err = s.Pool.Exec(r.Context(),
			`DELETE FROM gruppen_mitglieder WHERE gruppe_id=$1 AND user_id=$2`,
			gruppeID, req.UserID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}

	aktion := AktGruppeAustritt
	if req.Drin {
		aktion = AktGruppeBeitritt
	}
	s.spurAusRequest(r, aktion, "gruppe", gruppeID, "",
		map[string]any{"konto": req.UserID})
	writeJSON(w, http.StatusOK, map[string]bool{"drin": req.Drin})
}

// ── Rechte an einem Space ───────────────────────────────────────────────────

type SpaceRecht struct {
	GruppeID   *string   `json:"gruppeId"`
	GruppeName string    `json:"gruppeName"`
	UserID     *string   `json:"userId"`
	UserName   string    `json:"userName"`
	Recht      string    `json:"recht"`
	ErteiltAm  time.Time `json:"erteiltAm"`
}

// ListSpaceRechte returns who may do what in a space.
func (s *Server) ListSpaceRechte(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	spaceID := chi.URLParam(r, "id")
	if !s.darfSpaceVerwalten(r.Context(), uid, spaceID) {
		writeErr(w, http.StatusForbidden, "keine Verwaltungsrechte an diesem Space")
		return
	}

	rows, err := s.Pool.Query(r.Context(),
		`SELECT sr.gruppe_id::text, coalesce(g.name, ''),
		        sr.user_id::text, coalesce(u.name, ''),
		        sr.recht, sr.erteilt_am
		 FROM space_rechte sr
		 LEFT JOIN gruppen g ON g.id = sr.gruppe_id
		 LEFT JOIN users   u ON u.id = sr.user_id
		 WHERE sr.space_id = $1
		 ORDER BY coalesce(g.name, u.name)`, spaceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []SpaceRecht{}
	for rows.Next() {
		var x SpaceRecht
		if err := rows.Scan(&x.GruppeID, &x.GruppeName, &x.UserID, &x.UserName,
			&x.Recht, &x.ErteiltAm); err == nil {
			list = append(list, x)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type rechtReq struct {
	GruppeID string `json:"gruppeId"`
	UserID   string `json:"userId"`
	Recht    string `json:"recht"` // leer bedeutet: Recht entziehen
}

// SetzeSpaceRecht grants or revokes one right.
func (s *Server) SetzeSpaceRecht(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	spaceID := chi.URLParam(r, "id")
	if !s.darfSpaceVerwalten(r.Context(), uid, spaceID) {
		writeErr(w, http.StatusForbidden, "keine Verwaltungsrechte an diesem Space")
		return
	}

	var req rechtReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	// Genau eines von beidem, wie im Schema. Hier noch einmal, damit die
	// Antwort verständlich ist statt eine Verletzung des CHECK zu melden.
	if (req.GruppeID == "") == (req.UserID == "") {
		writeErr(w, http.StatusBadRequest, "entweder eine Gruppe oder ein Konto angeben, nicht beides")
		return
	}

	if req.Recht == "" {
		var err error
		if req.GruppeID != "" {
			_, err = s.Pool.Exec(r.Context(),
				`DELETE FROM space_rechte WHERE space_id=$1 AND gruppe_id=$2`, spaceID, req.GruppeID)
		} else {
			_, err = s.Pool.Exec(r.Context(),
				`DELETE FROM space_rechte WHERE space_id=$1 AND user_id=$2`, spaceID, req.UserID)
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "konnte nicht entzogen werden")
			return
		}
		s.spurAusRequest(r, AktSpaceRechtWeg, "space", spaceID, "",
			map[string]any{"gruppe": req.GruppeID, "konto": req.UserID})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	switch req.Recht {
	case "lesen", "schreiben", "verwalten":
	default:
		writeErr(w, http.StatusBadRequest, "erwartet lesen, schreiben oder verwalten")
		return
	}

	var err error
	if req.GruppeID != "" {
		_, err = s.Pool.Exec(r.Context(),
			`INSERT INTO space_rechte (space_id, gruppe_id, recht) VALUES ($1, $2, $3)
			 ON CONFLICT (space_id, gruppe_id) WHERE gruppe_id IS NOT NULL
			 DO UPDATE SET recht = EXCLUDED.recht, erteilt_am = now()`,
			spaceID, req.GruppeID, req.Recht)
	} else {
		_, err = s.Pool.Exec(r.Context(),
			`INSERT INTO space_rechte (space_id, user_id, recht) VALUES ($1, $2, $3)
			 ON CONFLICT (space_id, user_id) WHERE user_id IS NOT NULL
			 DO UPDATE SET recht = EXCLUDED.recht, erteilt_am = now()`,
			spaceID, req.UserID, req.Recht)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht erteilt werden")
		return
	}

	s.spurAusRequest(r, AktSpaceRecht, "space", spaceID, "",
		map[string]any{"gruppe": req.GruppeID, "konto": req.UserID, "recht": req.Recht})
	writeJSON(w, http.StatusOK, map[string]string{"recht": req.Recht})
}
