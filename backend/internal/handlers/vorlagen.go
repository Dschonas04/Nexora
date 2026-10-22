// Templates: the starting point of a page, kept apart from the pages.
//
// The first version of this feature was a tick on an ordinary page. That page
// then stood in the tree, the search, the graph and the export, and could be
// used only for pages at the top level. Here a template is a row of its own:
// it appears nowhere but in the choice offered on an empty page, and applying
// it COPIES title, icon and content -- a template changed later leaves the
// pages made from it alone.
//
// The built-in templates (meeting notes, how-to, project page) live in the
// interface, translated there; the service only keeps the ones people save.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// Vorlage is one saved template.
type Vorlage struct {
	ID         string          `json:"id"`
	Titel      string          `json:"title"`
	Icon       string          `json:"icon"`
	Inhalt     json.RawMessage `json:"content"`
	FuerAlle   bool            `json:"forAll"`
	Eigen      bool            `json:"own"`
	Besitzer   string          `json:"ownerName"`
	ErstelltAm time.Time       `json:"createdAt"`
}

// maxVorlagen limits the templates of one account. A choice of three hundred
// is no choice any more, and the limit keeps a script from filling the table.
const maxVorlagen = 100

// ListVorlagen returns the caller's own templates and those offered to all,
// content included: templates are small, and the choice applies one with a
// single click instead of a second round trip.
func (s *Server) ListVorlagen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT v.id, v.titel, v.icon, v.inhalt, v.fuer_alle, v.owner_id = $1, coalesce(u.name, '')
		 FROM vorlagen v LEFT JOIN users u ON u.id = v.owner_id
		 WHERE v.owner_id = $1 OR v.fuer_alle
		 ORDER BY v.fuer_alle, lower(v.titel)`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	liste := []Vorlage{}
	for rows.Next() {
		var v Vorlage
		var inhalt []byte
		if err := rows.Scan(&v.ID, &v.Titel, &v.Icon, &inhalt, &v.FuerAlle, &v.Eigen, &v.Besitzer); err == nil {
			v.Inhalt = json.RawMessage(inhalt)
			liste = append(liste, v)
		}
	}
	writeJSON(w, http.StatusOK, liste)
}

type vorlageReq struct {
	Titel    string          `json:"title"`
	Icon     string          `json:"icon"`
	Inhalt   json.RawMessage `json:"content"`
	FuerAlle bool            `json:"forAll"`
}

// VorlageAnlegen saves a template, usually from the page that is open.
func (s *Server) VorlageAnlegen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req vorlageReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Titel = strings.TrimSpace(req.Titel)
	if req.Titel == "" {
		writeErr(w, http.StatusBadRequest, "Die Vorlage braucht einen Namen.")
		return
	}
	if len(req.Titel) > 200 {
		req.Titel = req.Titel[:200]
	}
	// The content has to be a BlockNote document, an array; anything else would
	// be copied into a page later and break the editor there.
	var probe []json.RawMessage
	if len(req.Inhalt) == 0 {
		req.Inhalt = json.RawMessage("[]")
	} else if err := json.Unmarshal(req.Inhalt, &probe); err != nil {
		writeErr(w, http.StatusBadRequest, "Inhalt ist kein Dokument")
		return
	}
	if req.FuerAlle && !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "Nur eine Administratorin oder ein Administrator kann Vorlagen für alle anlegen.")
		return
	}
	var anzahl int
	_ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM vorlagen WHERE owner_id=$1`, uid).Scan(&anzahl)
	if anzahl >= maxVorlagen {
		writeErr(w, http.StatusBadRequest, "Zu viele Vorlagen.")
		return
	}
	var v Vorlage
	var inhalt []byte
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO vorlagen (owner_id, titel, icon, inhalt, fuer_alle)
		 VALUES ($1, $2, $3, $4::jsonb, $5)
		 RETURNING id, titel, icon, inhalt, fuer_alle, erstellt_am`,
		uid, req.Titel, req.Icon, string(req.Inhalt), req.FuerAlle).
		Scan(&v.ID, &v.Titel, &v.Icon, &inhalt, &v.FuerAlle, &v.ErstelltAm)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Vorlage konnte nicht gespeichert werden")
		return
	}
	v.Inhalt = json.RawMessage(inhalt)
	v.Eigen = true
	s.spurAusRequest(r, AktVorlageAngelegt, "vorlage", v.ID, v.Titel, map[string]interface{}{"forAll": v.FuerAlle})
	writeJSON(w, http.StatusCreated, v)
}

// VorlageLoeschen removes a template: its owner, or an admin for one offered
// to all.
func (s *Server) VorlageLoeschen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	var titel string
	err := s.Pool.QueryRow(r.Context(),
		`DELETE FROM vorlagen WHERE id=$1 AND (owner_id=$2 OR (fuer_alle AND $3))
		 RETURNING titel`, id, uid, s.isAdmin(r.Context(), uid)).Scan(&titel)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Vorlage nicht gefunden")
		return
	}
	s.spurAusRequest(r, AktVorlageGeloescht, "vorlage", id, titel, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
