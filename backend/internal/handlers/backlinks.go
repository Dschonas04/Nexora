// Backlinks: which pages point at this one. Two sources are merged here, the
// [[wiki-links]] written into page text and the explicit links from page_links.
package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// Backlinks returns the pages that reference this one, from both link kinds.
//
// Wiki-links are resolved by title, not by id, so renaming a page quietly breaks
// every [[old title]] pointing at it. That is the price of a link one can type;
// the explicit links handled below survive a rename because they store ids.
//
// Finding wiki-links means loading the content of every visible page and
// scanning it in Go, since the titles live inside the JSON document and cannot
// be indexed. Fine for a personal workspace, the wrong shape for a large one.
//
// Only pages the caller may read are considered, so backlinks never reveal that
// an inaccessible page links here.
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
	// Compare lowercased, so [[project notes]] finds a page titled "Project
	// Notes". wikiLinks lowercases its results the same way.
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
	seen := map[string]bool{}
	for rows.Next() {
		var p models.PageMeta
		var content string
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon, &p.UpdatedAt, &content); err != nil {
			continue
		}
		for _, l := range wikiLinks(content) {
			if l == target {
				list = append(list, p)
				seen[p.ID] = true
				break
			}
		}
	}

	// Explicit links pointing here. A page that both mentions this one in its
	// text and links to it explicitly must appear once, hence the seen map.
	//
	// A query error is swallowed rather than failing the request: the wiki-link
	// results above are already worth showing.
	var mrows pgx.Rows
	if s.isAdmin(r.Context(), uid) {
		mrows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at
			 FROM page_links pl JOIN pages p ON p.id = pl.source_id
			 WHERE pl.target_id=$1 AND p.deleted_at IS NULL`, id)
	} else {
		mrows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at
			 FROM page_links pl JOIN pages p ON p.id = pl.source_id
			 WHERE pl.target_id=$1 AND p.deleted_at IS NULL AND (
			   p.owner_id=$2 OR p.id IN (SELECT page_id FROM page_shares WHERE user_id=$2)
			 )`, id, uid)
	}
	if err == nil {
		defer mrows.Close()
		for mrows.Next() {
			var p models.PageMeta
			if err := mrows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon, &p.UpdatedAt); err != nil {
				continue
			}
			if !seen[p.ID] {
				list = append(list, p)
				seen[p.ID] = true
			}
		}
	}
	writeJSON(w, http.StatusOK, list)
}
