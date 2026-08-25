// Page templates.
//
// A template is an ordinary page with a flag set. That decision shapes
// everything else here: templates are written in the same editor, live in the
// same tree, and follow the same permission rules as any other page. A separate
// store would have meant a second editing surface and a second notion of who
// may see what, for no gain a reader would notice.
//
// Creating from a template copies the content once. It does not link the two,
// changing a template later leaves pages made from it alone, which is what
// people expect from the word "template" and not from the word "reference".
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ListVorlagen returns the templates the caller may use: their own, plus any
// shared with them, plus everything when they are an admin.
func (s *Server) ListVorlagen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	admin := s.isAdmin(r.Context(), uid)

	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at,
		        (p.owner_id <> $1) AS fremd
		 FROM pages p
		 WHERE p.ist_vorlage
		   AND p.deleted_at IS NULL
		   AND (p.owner_id = $1 OR $2
		        OR EXISTS (SELECT 1 FROM page_shares sh
		                   WHERE sh.page_id = p.id AND sh.user_id = $1))
		 ORDER BY p.title`, uid, admin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon,
			&p.UpdatedAt, &p.Shared); err == nil {
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// SetzeVorlage marks a page as a template, or takes the mark away.
//
// Only the owner decides. An admin could technically reach the row, but
// declaring someone else's page a template would change what it means to them
// without asking, it would show up in every colleague's new-page menu.
func (s *Server) SetzeVorlage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	_, _, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || !isOwner {
		writeErr(w, http.StatusForbidden, "nur der Eigentümer einer Seite kann sie zur Vorlage machen")
		return
	}

	var neu bool
	if err := s.Pool.QueryRow(r.Context(),
		`UPDATE pages SET ist_vorlage = NOT ist_vorlage WHERE id=$1 RETURNING ist_vorlage`,
		id).Scan(&neu); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gesetzt werden")
		return
	}

	aktion := AktVorlageAus
	if neu {
		aktion = AktVorlageAn
	}
	s.spurAusRequest(r, aktion, "seite", id, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"istVorlage": neu})
}

// inhaltAusVorlage liest den Inhalt einer Vorlage, wenn der Aufrufer sie
// benutzen darf. Fehlt sie oder ist sie keine, kommt nichts zurück, die neue
// Seite entsteht dann einfach leer, statt dass der Aufruf scheitert.
func (s *Server) inhaltAusVorlage(r *http.Request, uid, vorlageID string) (json.RawMessage, string) {
	if vorlageID == "" {
		return nil, ""
	}
	canRead, _, _, ok := s.pagePerm(r.Context(), uid, vorlageID)
	if !ok || !canRead {
		return nil, ""
	}
	var inhalt []byte
	var icon string
	var istVorlage bool
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT content, icon, ist_vorlage FROM pages WHERE id=$1`,
		vorlageID).Scan(&inhalt, &icon, &istVorlage); err != nil || !istVorlage {
		return nil, ""
	}
	return json.RawMessage(inhalt), icon
}
