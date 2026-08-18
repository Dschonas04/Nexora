// Tags, favorites, search, and the anonymous read view behind a public link.
// These are the small endpoints that did not warrant a file of their own.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ListTags returns the caller's tags. Tags are per user, so two people can each
// keep their own vocabulary without seeing the other's.
func (s *Server) ListTags(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, name, color FROM tags WHERE owner_id=$1 ORDER BY name`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	list := []models.Tag{}
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err == nil {
			list = append(list, t)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type createTagReq struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CreateTag adds a tag, or recolors it if that name already exists. Treating a
// duplicate as an update rather than an error means the frontend can create a
// tag optimistically without checking first.
func (s *Server) CreateTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req createTagReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if req.Color == "" {
		req.Color = "#6b7280"
	}
	var t models.Tag
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO tags (owner_id, name, color) VALUES ($1, $2, $3)
		 ON CONFLICT (owner_id, name) DO UPDATE SET color = EXCLUDED.color
		 RETURNING id, name, color`,
		uid, req.Name, req.Color).Scan(&t.ID, &t.Name, &t.Color)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create tag")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// DeleteTag removes a tag. The page_tags rows go with it through the cascade,
// so the tag disappears from every page at once.
func (s *Server) DeleteTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM tags WHERE id=$1 AND owner_id=$2`, chi.URLParam(r, "id"), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type attachTagReq struct {
	TagID string `json:"tagId"`
}

// AttachTag puts a tag on a page. Both ownership checks sit inside the INSERT
// ... SELECT, so the row only appears when the caller owns the page and the tag.
// A request that fails those checks writes nothing and still answers 200, since
// from the client's point of view there is nothing to retry.
func (s *Server) AttachTag(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req attachTagReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO page_tags (page_id, tag_id)
		 SELECT $2, $3
		 WHERE EXISTS (SELECT 1 FROM pages WHERE id=$2 AND owner_id=$1)
		   AND EXISTS (SELECT 1 FROM tags  WHERE id=$3 AND owner_id=$1)
		 ON CONFLICT DO NOTHING`,
		uid, chi.URLParam(r, "id"), req.TagID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "attach failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DetachTag removes a tag from a page.
func (s *Server) DetachTag(w http.ResponseWriter, r *http.Request) {
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM page_tags WHERE page_id=$1 AND tag_id=$2`,
		chi.URLParam(r, "id"), chi.URLParam(r, "tagId"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "detach failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListFavorites returns the pages this user has pinned, most recently edited
// first.
func (s *Server) ListFavorites(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id, p.parent_id, p.title, p.icon, p.updated_at FROM pages p
		 JOIN favorites f ON f.page_id = p.id
		 WHERE f.user_id=$1 ORDER BY p.updated_at DESC`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Title, &p.Icon, &p.UpdatedAt); err == nil {
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// Search does a case-insensitive substring match over titles and content.
//
// content::text ILIKE means the whole JSON document is searched as a string, so
// a hit can come from markup rather than visible text. It also cannot use an
// index and scans every page of the user. That is fine for a personal
// workspace; a large instance would want a tsvector column instead.
//
// Only the caller's own pages are searched, not pages shared with them.
func (s *Server) Search(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []models.PageMeta{})
		return
	}
	// The pattern goes in as a parameter, so % and _ typed by the user widen
	// their own search but cannot break out of the query.
	pattern := "%" + q + "%"
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, parent_id, title, icon, updated_at FROM pages
		 WHERE owner_id=$1 AND (title ILIKE $2 OR content::text ILIKE $2)
		 ORDER BY updated_at DESC LIMIT 50`, uid, pattern)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()
	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Title, &p.Icon, &p.UpdatedAt); err == nil {
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// publicPage is a deliberately narrow view: title, content, icon and the date.
// No owner, no id, no tags, nothing that would leak workspace structure to an
// anonymous visitor.
type publicPage struct {
	Title     string          `json:"title"`
	Content   json.RawMessage `json:"content"`
	Icon      string          `json:"icon"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// GetPublicPage serves a page to anyone holding its token. This is the only
// read endpoint outside the auth middleware. is_public is checked as well as
// the token, so revoking a link takes effect even if the old token were reused.
func (s *Server) GetPublicPage(w http.ResponseWriter, r *http.Request) {
	var p publicPage
	var content []byte
	err := s.Pool.QueryRow(r.Context(),
		`SELECT title, content, icon, updated_at FROM pages
		 WHERE public_token=$1 AND is_public=true`, chi.URLParam(r, "token")).
		Scan(&p.Title, &content, &p.Icon, &p.UpdatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	p.Content = json.RawMessage(content)
	writeJSON(w, http.StatusOK, p)
}
