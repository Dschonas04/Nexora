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

func (s *Server) Search(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []models.PageMeta{})
		return
	}
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

type publicPage struct {
	Title     string          `json:"title"`
	Content   json.RawMessage `json:"content"`
	Icon      string          `json:"icon"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

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
