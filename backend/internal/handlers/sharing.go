// Per-user sharing and account administration. This is the sharing that names
// people; the anonymous public link lives in pages.go.
package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ListShares returns who a page is shared with. Restricted to the owner and
// admins: someone the page was shared with should not learn who else has it.
func (s *Server) ListShares(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, _, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || (!isOwner && !s.isAdmin(r.Context(), uid)) {
		writeErr(w, http.StatusForbidden, "owner only")
		return
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT u.id, u.name, u.email, ps.permission FROM page_shares ps
		 JOIN users u ON u.id = ps.user_id WHERE ps.page_id=$1 ORDER BY u.name`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.ShareEntry{}
	for rows.Next() {
		var e models.ShareEntry
		if err := rows.Scan(&e.UserID, &e.Name, &e.Email, &e.Permission); err == nil {
			list = append(list, e)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type shareReq struct {
	Email      string `json:"email"`
	Permission string `json:"permission"` // "read" | "edit"
}

// AddShare grants another account access, looked up by email address. An
// existing share is updated rather than rejected, so the dialog can change a
// permission without removing and re-adding the entry.
func (s *Server) AddShare(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, _, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || (!isOwner && !s.isAdmin(r.Context(), uid)) {
		writeErr(w, http.StatusForbidden, "owner only")
		return
	}
	var req shareReq
	_ = decode(r, &req)
	// Only two permissions exist, and anything unrecognised falls back to the
	// weaker one, so a typo cannot accidentally hand out write access.
	perm := "read"
	if req.Permission == "edit" {
		perm = "edit"
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		writeErr(w, http.StatusBadRequest, "email required")
		return
	}

	var targetID string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE lower(email)=$1`, email).Scan(&targetID); err != nil {
		writeErr(w, http.StatusNotFound, "no user with that email")
		return
	}
	var ownerID string
	_ = s.Pool.QueryRow(r.Context(), `SELECT owner_id FROM pages WHERE id=$1`, id).Scan(&ownerID)
	// Sharing with the owner would add a row that grants nothing but shows up in
	// the dialog as if it mattered.
	if targetID == ownerID {
		writeErr(w, http.StatusBadRequest, "owner already has full access")
		return
	}

	_, err := s.Pool.Exec(r.Context(),
		`INSERT INTO page_shares (page_id, user_id, permission) VALUES ($1, $2, $3)
		 ON CONFLICT (page_id, user_id) DO UPDATE SET permission=EXCLUDED.permission`,
		id, targetID, perm)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "share failed")
		return
	}
	s.spurAusRequest(r, AktFreigabe, "seite", id, "",
		map[string]interface{}{"an": targetID, "recht": perm})

	// A share the recipient never hears about is a row in a table. It does appear
	// in their sidebar, but only if they happen to look.
	var name string
	_ = s.Pool.QueryRow(r.Context(), `SELECT name FROM users WHERE id=$1`, uid).Scan(&name)
	s.zustellen(r.Context(), targetID, PostFreigabe, id, "", uid, name,
		s.seitenTitel(r.Context(), id), "")

	writeJSON(w, http.StatusOK, map[string]string{"userId": targetID, "permission": perm})
}

// RemoveShare revokes access. It takes effect immediately because pagePerm
// reads the table on every request rather than caching.
func (s *Server) RemoveShare(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, _, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || (!isOwner && !s.isAdmin(r.Context(), uid)) {
		writeErr(w, http.StatusForbidden, "owner only")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`DELETE FROM page_shares WHERE page_id=$1 AND user_id=$2`, id, chi.URLParam(r, "userId"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unshare failed")
		return
	}
	s.spurAusRequest(r, AktFreigabeWeg, "seite", id, "",
		map[string]interface{}{"von": chi.URLParam(r, "userId")})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListUsers is the directory behind the share dialog and the admin view.
//
// Deliberately open to every signed-in account, not just admins: without it
// nobody could share a page with a colleague. The trade-off is that any account
// can read the names and email addresses of all the others, which is acceptable
// for a workspace among people who know each other and would not be for a
// public instance.
func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, email, name, role, created_at FROM users ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.User{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt); err == nil {
			list = append(list, u)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type roleReq struct {
	Role string `json:"role"`
}

// SetUserRole promotes or demotes an account. Anything other than "admin" is
// read as "user", so an unknown value cannot grant privileges. An admin cannot
// demote themselves, which stops the workspace from ending up with none.
func (s *Server) SetUserRole(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "admin only")
		return
	}
	var req roleReq
	_ = decode(r, &req)
	role := "user"
	if req.Role == "admin" {
		role = "admin"
	}
	target := chi.URLParam(r, "id")
	if target == uid && role != "admin" {
		writeErr(w, http.StatusBadRequest, "cannot demote yourself")
		return
	}
	tag, err := s.Pool.Exec(r.Context(), `UPDATE users SET role=$2 WHERE id=$1`, target, role)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	s.spurAusRequest(r, AktRolleGeaendert, "konto", target, "", map[string]interface{}{"neue_rolle": role})
	writeJSON(w, http.StatusOK, map[string]string{"role": role})
}
