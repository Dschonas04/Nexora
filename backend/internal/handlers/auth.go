package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"nexora/internal/auth"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

type registerReq struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
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
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}

	var u models.User
	err = s.Pool.QueryRow(r.Context(),
		`INSERT INTO users (email, name, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, email, name, created_at`,
		req.Email, req.Name, hash,
	).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeErr(w, http.StatusConflict, "email already registered")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not create user")
		return
	}

	s.issueSession(w, u.ID)
	writeJSON(w, http.StatusCreated, u)
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	var u models.User
	var hash string
	err := s.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, password_hash, created_at FROM users WHERE email = $1`,
		req.Email,
	).Scan(&u.ID, &u.Email, &u.Name, &hash, &u.CreatedAt)
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.issueSession(w, u.ID)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	s.clearAuthCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var u models.User
	err := s.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, created_at FROM users WHERE id = $1`, uid,
	).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) issueSession(w http.ResponseWriter, userID string) {
	token, err := auth.GenerateToken(s.Secret, userID, tokenTTL)
	if err == nil {
		s.setAuthCookie(w, token)
	}
}
