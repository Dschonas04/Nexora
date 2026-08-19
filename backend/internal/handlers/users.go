// Admin-only account management. Self-registration lives in auth.go; this is
// the path an admin uses to add and remove accounts from the admin view.
package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nexora/internal/auth"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

type newUserReq struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUser lets an admin add an account directly, including its role. The
// validation mirrors Register, minus the first-account-becomes-admin rule: by
// the time an admin uses this, one already exists.
func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}

	var req newUserReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		writeErr(w, http.StatusBadRequest, "valid email required")
		return
	}
	if req.Name == "" {
		req.Name = strings.Split(req.Email, "@")[0]
	}
	if len(req.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}
	role := "user"
	if req.Role == "admin" {
		role = "admin"
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}

	var u models.User
	err = s.Pool.QueryRow(r.Context(),
		`INSERT INTO users (email, name, password_hash, role) VALUES ($1, $2, $3, $4)
		 RETURNING id, email, name, role, created_at`,
		req.Email, req.Name, hash, role,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeErr(w, http.StatusConflict, "email already registered")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not create user")
		return
	}
	s.spurAusRequest(r, AktKontoAngelegt, "konto", u.ID, u.Email, map[string]interface{}{"rolle": u.Role})
	writeJSON(w, http.StatusCreated, u)
}

// DeleteUser removes an account. Everything owned by it goes too, through the
// cascades on pages, tags, spaces and attachment rows, so this is not a soft
// delete and there is no trash to recover from.
//
// An admin cannot delete themselves, which also keeps the last admin from
// disappearing by accident.
func (s *Server) DeleteUser(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	target := chi.URLParam(r, "id")
	if target == uid {
		writeErr(w, http.StatusBadRequest, "cannot delete yourself")
		return
	}
	// Adresse vor dem Löschen lesen: danach ist die Zeile fort, und ein
	// Prüfspureintrag ohne Namen wäre für eine Revision wertlos.
	var mail string
	_ = s.Pool.QueryRow(r.Context(), `SELECT email FROM users WHERE id=$1`, target).Scan(&mail)

	tag, err := s.Pool.Exec(r.Context(), `DELETE FROM users WHERE id=$1`, target)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	s.spurAusRequest(r, AktKontoGeloescht, "konto", target, mail, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
