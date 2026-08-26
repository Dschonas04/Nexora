// Spaces are flat, top-level containers, a second axis next to page nesting.
// A page belongs to at most one. A space has exactly one owner, but it can be
// reached by others in two ways: through a right granted on it (Zusatzumfang),
// or because it was opened to the whole instance.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ListSpaces returns the spaces the caller can reach: their own, those they
// hold a right on, and the public ones.
func (s *Server) ListSpaces(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	// Own spaces, those held by a right, and the public ones. Without the second
	// part the shared pages would appear in the sidebar without the space they
	// belong to, loose instead of ordered.
	//
	// Sorted so that the public spaces stand on top: they are the shared ground
	// and therefore usually what one is looking for.
	rows, err := s.Pool.Query(r.Context(),
		`SELECT sp.id, sp.owner_id, sp.name, sp.created_at, sp.oeffentlich,
		        (sp.owner_id <> $1) AS fremd,
		        (sp.owner_id = $1
		         OR EXISTS (SELECT 1 FROM users WHERE id = $1 AND role = 'admin')
		         OR ($2 AND EXISTS (
		               SELECT 1 FROM space_rechte sr
		                WHERE sr.space_id = sp.id AND sr.recht = 'verwalten'
		                  AND (sr.user_id = $1
		                       OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
		                                           WHERE gm.user_id = $1))))) AS darf_verwalten
		 FROM spaces sp
		 WHERE sp.owner_id = $1
		    OR sp.oeffentlich <> 'nein'
		    OR ($2 AND EXISTS (
		          SELECT 1 FROM space_rechte sr
		           WHERE sr.space_id = sp.id
		             AND (sr.user_id = $1
		                  OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
		                                      WHERE gm.user_id = $1))))
		 ORDER BY (sp.oeffentlich = 'nein'), sp.name`, uid, lizenz.Frei(lizenz.Gruppen))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.Space{}
	for rows.Next() {
		var sp models.Space
		if err := rows.Scan(&sp.ID, &sp.OwnerID, &sp.Name, &sp.CreatedAt,
			&sp.Oeffentlich, &sp.Fremd, &sp.DarfVerwalten); err == nil {
			list = append(list, sp)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type spaceReq struct {
	Name string `json:"name"`
}

// CreateSpace adds a space. Names are not unique: two spaces may share a name,
// which is deliberate because the id is what identifies them.
func (s *Server) CreateSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req spaceReq
	_ = decode(r, &req)
	if req.Name == "" {
		req.Name = "New space"
	}
	var sp models.Space
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO spaces (owner_id, name) VALUES ($1, $2)
		 RETURNING id, owner_id, name, created_at, oeffentlich`,
		uid, req.Name).Scan(&sp.ID, &sp.OwnerID, &sp.Name, &sp.CreatedAt, &sp.Oeffentlich)
	sp.DarfVerwalten = true
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create space")
		return
	}
	// The counterpart to the deletion: only both entries together make a trail
	// along which the stock of spaces can be followed.
	s.spurAusRequest(r, AktSpaceAngelegt, "space", sp.ID, sp.Name, nil)
	writeJSON(w, http.StatusCreated, sp)
}

// RenameSpace changes the name. Ownership is part of the WHERE clause, so a
// request for someone else's space simply affects no rows and reports 404.
func (s *Server) RenameSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req spaceReq
	_ = decode(r, &req)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	tag, err := s.Pool.Exec(r.Context(),
		`UPDATE spaces SET name=$3 WHERE id=$1 AND owner_id=$2`, chi.URLParam(r, "id"), uid, req.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "rename failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "space not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DeleteSpace removes a space. Its pages survive and fall back to "no space"
// through the ON DELETE SET NULL on pages.space_id: deleting a container must
// never delete the content in it.
func (s *Server) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	// Fetch the name beforehand: after the deletion it is gone, and an audit
	// entry naming only an id no longer answers the question "which space was
	// that?".
	var name string
	_ = s.Pool.QueryRow(r.Context(), `SELECT name FROM spaces WHERE id=$1`, id).Scan(&name)

	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM spaces WHERE id=$1 AND owner_id=$2`, id, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "space not found")
		return
	}

	// Deleting a space has consequences: the pages inside it lose their
	// assignment and the rights granted on it disappear with it. Until now that
	// left no trace, and afterwards there was no way to tell the space had ever
	// existed.
	s.spurAusRequest(r, AktSpaceGeloescht, "space", id, name, nil)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type spaceOeffentlichReq struct {
	Oeffentlich string `json:"oeffentlich"` // "nein" | "lesen" | "schreiben"
}

// SetSpaceOeffentlich opens a space to the whole instance, or takes that back.
//
// What is meant is explicitly "open to every logged in account of this
// instance", not "open on the internet". Anonymous access still runs solely
// through the share link of a single page. That distinction is important enough
// not to hide it behind the same word: whoever makes a space public shall not
// thereby put anything on the net.
//
// Unlike the space rights this depends on no paid extra. A shared space is what
// makes a wiki a wiki in the first place; putting it behind a licence would turn
// the free core into a single seat filing cabinet. The finely graded rights per
// group remain the paid extra.
func (s *Server) SetSpaceOeffentlich(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if !s.darfSpaceVerwalten(r.Context(), uid, id) {
		writeErr(w, http.StatusForbidden, "nur Eigentümer, Verwalter oder Admin")
		return
	}
	var req spaceOeffentlichReq
	_ = decode(r, &req)
	// Whatever is not explicitly allowed becomes 'nein'. A typo in the call shall
	// close a space, never open it.
	wert := "nein"
	switch req.Oeffentlich {
	case "lesen", "schreiben":
		wert = req.Oeffentlich
	}
	tag, err := s.Pool.Exec(r.Context(),
		`UPDATE spaces SET oeffentlich=$2 WHERE id=$1`, id, wert)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Änderung fehlgeschlagen")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "space not found")
		return
	}
	s.spurAusRequest(r, AktSpaceOeffentlich, "space", id, "",
		map[string]interface{}{"sichtbarkeit": wert})
	writeJSON(w, http.StatusOK, map[string]string{"oeffentlich": wert})
}
