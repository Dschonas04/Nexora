package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

func (s *Server) ListPages(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, parent_id, title, icon, updated_at FROM pages
		 WHERE owner_id = $1 ORDER BY sort_order, created_at`, uid)
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

type createPageReq struct {
	Title    string  `json:"title"`
	ParentID *string `json:"parentId"`
}

func (s *Server) CreatePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req createPageReq
	_ = decode(r, &req)
	if req.Title == "" {
		req.Title = "Untitled"
	}

	var id string
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO pages (owner_id, parent_id, title) VALUES ($1, $2, $3) RETURNING id`,
		uid, req.ParentID, req.Title,
	).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create page")
		return
	}
	page, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load failed")
		return
	}
	writeJSON(w, http.StatusCreated, page)
}

func (s *Server) GetPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	page, err := s.loadPage(r.Context(), uid, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type updatePageReq struct {
	Title    *string         `json:"title"`
	Content  json.RawMessage `json:"content"`
	Icon     *string         `json:"icon"`
	ParentID json.RawMessage `json:"parentId"` // absent = unchanged, null = move to root
}

func (s *Server) UpdatePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	cur, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}

	var req updatePageReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	title := cur.Title
	if req.Title != nil {
		title = *req.Title
	}
	icon := cur.Icon
	if req.Icon != nil {
		icon = *req.Icon
	}
	content := cur.Content
	if len(req.Content) > 0 {
		content = req.Content
	}
	parent := cur.ParentID
	if len(req.ParentID) > 0 { // key present in body
		var pid *string
		if err := json.Unmarshal(req.ParentID, &pid); err == nil {
			parent = pid
		}
	}

	_, err = s.Pool.Exec(r.Context(),
		`UPDATE pages SET title=$3, content=$4::jsonb, icon=$5, parent_id=$6, updated_at=now()
		 WHERE id=$1 AND owner_id=$2`,
		id, uid, title, string(content), icon, parent)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}

	page, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) DeletePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM pages WHERE id=$1 AND owner_id=$2`, chi.URLParam(r, "id"), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) AddFavorite(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO favorites (user_id, page_id)
		 SELECT $1, $2 WHERE EXISTS (SELECT 1 FROM pages WHERE id=$2 AND owner_id=$1)
		 ON CONFLICT DO NOTHING`, uid, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "favorite failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) RemoveFavorite(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM favorites WHERE user_id=$1 AND page_id=$2`, uid, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unfavorite failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) SharePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var token string
	err := s.Pool.QueryRow(r.Context(),
		`UPDATE pages SET is_public=true,
		   public_token = COALESCE(public_token, encode(gen_random_bytes(16), 'hex'))
		 WHERE id=$1 AND owner_id=$2 RETURNING public_token`,
		chi.URLParam(r, "id"), uid).Scan(&token)
	if err != nil {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"isPublic": true, "publicToken": token})
}

func (s *Server) UnsharePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	_, err := s.Pool.Exec(r.Context(),
		`UPDATE pages SET is_public=false, public_token=NULL WHERE id=$1 AND owner_id=$2`,
		chi.URLParam(r, "id"), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unshare failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"isPublic": false})
}
