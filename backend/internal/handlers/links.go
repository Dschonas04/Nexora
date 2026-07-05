package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// visiblePagesFilter appends a permission clause (and its args) so a query only
// returns pages the user may read. Admins see everything.
func (s *Server) linkTargets(r *http.Request, uid, sourceID string) ([]models.PageMeta, error) {
	var rows pgx.Rows
	var err error
	if s.isAdmin(r.Context(), uid) {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at
			 FROM page_links pl JOIN pages p ON p.id = pl.target_id
			 WHERE pl.source_id=$1 AND p.deleted_at IS NULL
			 ORDER BY p.title`, sourceID)
	} else {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at
			 FROM page_links pl JOIN pages p ON p.id = pl.target_id
			 WHERE pl.source_id=$1 AND p.deleted_at IS NULL AND (
			   p.owner_id=$2 OR p.id IN (SELECT page_id FROM page_shares WHERE user_id=$2)
			 )
			 ORDER BY p.title`, sourceID, uid)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon, &p.UpdatedAt); err == nil {
			list = append(list, p)
		}
	}
	return list, nil
}

// ListLinks returns the manual outgoing links of a page (pages it links to).
func (s *Server) ListLinks(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	list, err := s.linkTargets(r, uid, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type addLinkReq struct {
	TargetID string `json:"targetId"`
}

// AddLink creates a manual link from the page to another page. Requires edit
// rights on the source page; the target must exist and not be the page itself.
func (s *Server) AddLink(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "no edit permission")
		return
	}
	var req addLinkReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.TargetID == "" || req.TargetID == id {
		writeErr(w, http.StatusBadRequest, "invalid target")
		return
	}
	ct, err := s.Pool.Exec(r.Context(),
		`INSERT INTO page_links (source_id, target_id)
		 SELECT $1, $2 WHERE EXISTS (SELECT 1 FROM pages WHERE id=$2 AND deleted_at IS NULL)
		 ON CONFLICT DO NOTHING`, id, req.TargetID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "add failed")
		return
	}
	if ct.RowsAffected() == 0 {
		// Either the link already exists or the target is gone; not fatal.
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// RemoveLink deletes a manual link. Requires edit rights on the source page.
func (s *Server) RemoveLink(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "no edit permission")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`DELETE FROM page_links WHERE source_id=$1 AND target_id=$2`,
		id, chi.URLParam(r, "targetId")); err != nil {
		writeErr(w, http.StatusInternalServerError, "remove failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
