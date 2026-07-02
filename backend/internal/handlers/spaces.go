package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

func (s *Server) ListSpaces(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, owner_id, name, created_at FROM spaces WHERE owner_id=$1 ORDER BY name`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.Space{}
	for rows.Next() {
		var sp models.Space
		if err := rows.Scan(&sp.ID, &sp.OwnerID, &sp.Name, &sp.CreatedAt); err == nil {
			list = append(list, sp)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type spaceReq struct {
	Name string `json:"name"`
}

func (s *Server) CreateSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var req spaceReq
	_ = decode(r, &req)
	if req.Name == "" {
		req.Name = "New space"
	}
	var sp models.Space
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO spaces (owner_id, name) VALUES ($1, $2) RETURNING id, owner_id, name, created_at`,
		uid, req.Name).Scan(&sp.ID, &sp.OwnerID, &sp.Name, &sp.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create space")
		return
	}
	writeJSON(w, http.StatusCreated, sp)
}

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

// DeleteSpace removes a space; its pages fall back to "no space" (FK SET NULL).
func (s *Server) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM spaces WHERE id=$1 AND owner_id=$2`, chi.URLParam(r, "id"), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "space not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
