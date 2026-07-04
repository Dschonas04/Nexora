package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// Backlinks returns the pages that reference the given page through an explicit
// [[Page title]] wiki-link. It only considers pages the requester can see, so
// the linking story stays inside each user's permission scope.
func (s *Server) Backlinks(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}

	var title string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT title FROM pages WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&title); err != nil {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	target := strings.ToLower(strings.TrimSpace(title))
	if target == "" {
		writeJSON(w, http.StatusOK, []models.PageMeta{})
		return
	}

	var rows pgx.Rows
	var err error
	if s.isAdmin(r.Context(), uid) {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT id, parent_id, space_id, title, icon, updated_at, content::text
			 FROM pages WHERE deleted_at IS NULL AND id <> $1`, id)
	} else {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at, p.content::text
			 FROM pages p WHERE p.deleted_at IS NULL AND p.id <> $1 AND (
			   p.owner_id=$2 OR p.id IN (SELECT page_id FROM page_shares WHERE user_id=$2)
			 )`, id, uid)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		var content string
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon, &p.UpdatedAt, &content); err != nil {
			continue
		}
		for _, l := range wikiLinks(content) {
			if l == target {
				list = append(list, p)
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, list)
}
