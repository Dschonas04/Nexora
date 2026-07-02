package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ListPages returns the pages the user owns (the sidebar tree).
func (s *Server) ListPages(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, parent_id, space_id, title, icon, updated_at FROM pages
		 WHERE owner_id = $1 AND deleted_at IS NULL ORDER BY sort_order, created_at`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon, &p.UpdatedAt); err == nil {
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// ListSharedPages returns pages other users have shared with me.
func (s *Server) ListSharedPages(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id, p.parent_id, p.space_id, p.title, p.icon, p.updated_at
		 FROM pages p JOIN page_shares ps ON ps.page_id = p.id
		 WHERE ps.user_id = $1 AND p.deleted_at IS NULL
		 ORDER BY p.updated_at DESC`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon, &p.UpdatedAt); err == nil {
			p.Shared = true
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type createPageReq struct {
	Title    string  `json:"title"`
	ParentID *string `json:"parentId"`
	SpaceID  *string `json:"spaceId"`
}

func (s *Server) CreatePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req createPageReq
	_ = decode(r, &req)
	if req.Title == "" {
		req.Title = "Untitled"
	}
	// A child page inherits its parent's space when none is given explicitly.
	if req.SpaceID == nil && req.ParentID != nil {
		var sid *string
		if err := s.Pool.QueryRow(r.Context(),
			`SELECT space_id FROM pages WHERE id=$1 AND owner_id=$2`, *req.ParentID, uid).Scan(&sid); err == nil {
			req.SpaceID = sid
		}
	}

	var id string
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO pages (owner_id, parent_id, space_id, title) VALUES ($1, $2, $3, $4) RETURNING id`,
		uid, req.ParentID, req.SpaceID, req.Title,
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
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	page, err := s.loadPage(r.Context(), uid, id)
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
	SpaceID  json.RawMessage `json:"spaceId"`  // absent = unchanged, null = no space
}

func (s *Server) UpdatePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	_, canEdit, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "read-only access")
		return
	}

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

	// Snapshot the pre-edit state into the version history (coalesced).
	s.snapshotVersion(r.Context(), cur, uid)

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
	// Moving pages in the hierarchy / between spaces stays owner-only.
	parent := cur.ParentID
	space := cur.SpaceID
	if isOwner {
		if len(req.ParentID) > 0 {
			var pid *string
			if err := json.Unmarshal(req.ParentID, &pid); err == nil {
				parent = pid
			}
		}
		if len(req.SpaceID) > 0 {
			var sid *string
			if err := json.Unmarshal(req.SpaceID, &sid); err == nil {
				space = sid
			}
		}
	}

	_, err = s.Pool.Exec(r.Context(),
		`UPDATE pages SET title=$2, content=$3::jsonb, icon=$4, parent_id=$5, space_id=$6, updated_at=now()
		 WHERE id=$1`,
		id, title, string(content), icon, parent, space)
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

// DeletePage soft-deletes a page and its whole subtree (moves it to the trash).
func (s *Server) DeletePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, _, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || (!isOwner && !s.isAdmin(r.Context(), uid)) {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`WITH RECURSIVE sub AS (
			SELECT id FROM pages WHERE id=$1
			UNION ALL
			SELECT p.id FROM pages p JOIN sub ON p.parent_id = sub.id
		 )
		 UPDATE pages SET deleted_at=now() WHERE id IN (SELECT id FROM sub) AND deleted_at IS NULL`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListTrash returns the user's soft-deleted pages.
func (s *Server) ListTrash(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, parent_id, space_id, title, icon, deleted_at FROM pages
		 WHERE owner_id=$1 AND deleted_at IS NOT NULL ORDER BY deleted_at DESC`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.PageMeta{}
	for rows.Next() {
		var p models.PageMeta
		if err := rows.Scan(&p.ID, &p.ParentID, &p.SpaceID, &p.Title, &p.Icon, &p.UpdatedAt); err == nil {
			list = append(list, p)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// RestorePage brings a page (and its deleted subtree) back from the trash.
func (s *Server) RestorePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	tag, err := s.Pool.Exec(r.Context(),
		`WITH RECURSIVE sub AS (
			SELECT id FROM pages WHERE id=$1 AND owner_id=$2
			UNION ALL
			SELECT p.id FROM pages p JOIN sub ON p.parent_id = sub.id
		 )
		 UPDATE pages SET deleted_at=NULL, updated_at=now() WHERE id IN (SELECT id FROM sub)`, id, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "restore failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PurgePage permanently removes a trashed page (and its subtree via cascade).
func (s *Server) PurgePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM pages WHERE id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL`,
		chi.URLParam(r, "id"), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "purge failed")
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
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO favorites (user_id, page_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, id)
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
		 WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL RETURNING public_token`,
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
