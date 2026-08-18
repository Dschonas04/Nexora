// Explicit page-to-page links, created through the UI. They are stored in
// page_links and are independent of the [[wiki-links]] a user types into the
// text; both kinds feed the backlinks list and the knowledge graph.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// linkTargets returns the pages a page links to, filtered to what the caller may
// read. A link to a page they have no access to is silently left out rather than
// shown as an unreachable entry.
//
// The admin branch is a separate query rather than a condition, because the
// permission clause carries an extra parameter that the admin query does not
// need.
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

// ListLinks returns a page's outgoing links.
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

// AddLink links the page to another one. Edit rights are required on the source
// page only: a link is a property of the page it starts from, and needing rights
// on the target as well would make it impossible to link to a page one may read
// but not change.
//
// Self-links are rejected because they would add a loop to the graph that
// carries no information.
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
		// Either the link is already there or the target has since been deleted.
		// Neither is worth an error: the desired state is reached in the first
		// case, and in the second there is nothing the client could do about it.
		// 200 instead of 201 tells the caller nothing new was created.
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// RemoveLink deletes a link. As with AddLink, rights on the source page decide.
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
