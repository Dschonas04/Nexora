// How wide the text of a page sits.
//
// The measure used to be fixed: 720 pixels, whether the page holds a scribbled
// note or a table with twelve columns. On a wide screen a hand's breadth of
// paper stayed empty left and right, and the table wrapped all the same.
//
// The value hangs on the page and not on the account: width belongs to the
// typesetting of the text like a heading does, and whoever opens a page of
// tables should see it the way its author set it.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// breiten are the permitted values. A fixed list and not a number: behind the
// names sit values in the stylesheet, and a free pixel count coming from the
// browser would be a number nobody checks any more.
//
// The empty value belongs to the list: it means "as the instance prescribes"
// and is the initial state of every page. Without it every page would be nailed
// forever to whatever happened to be the default when it was created.
var breiten = map[string]bool{"": true, "normal": true, "breit": true, "voll": true}

type breiteReq struct {
	Breite string `json:"breite"`
}

// SetzeBreite changes the measure of a page. Whoever may write may do this as
// well: it is a property of the text, not of the sharing.
func (s *Server) SetzeBreite(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "Seite nicht gefunden")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "nur mit Schreibrecht")
		return
	}

	var req breiteReq
	if err := decode(r, &req); err != nil || !breiten[req.Breite] {
		writeErr(w, http.StatusBadRequest, "erwartet normal, breit oder voll")
		return
	}

	// Without touching updated_at: the width is no change to the content, and a
	// new revision in the history would be too much for one adjustment of the
	// measure. An open editor would also lose its base and report a conflict on
	// the next save that does not exist.
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE pages SET breite=$2 WHERE id=$1 AND deleted_at IS NULL`, id, req.Breite); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"breite": req.Breite})
}
