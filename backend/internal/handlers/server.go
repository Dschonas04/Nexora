package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexora/internal/models"
)

const (
	cookieName = "nexora_token"
	tokenTTL   = 7 * 24 * time.Hour
)

type Server struct {
	Pool    *pgxpool.Pool
	Secret  []byte
	DataDir string // directory where uploaded attachments are stored on disk
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(tokenTTL),
		MaxAge:   int(tokenTTL.Seconds()),
	})
}

func (s *Server) clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// loadPage returns a full (non-deleted) page with tags and, for a logged-in
// viewer, its favorite flag and permission flags. viewerID == "" is used for
// public pages and skips per-user fields. Callers are responsible for checking
// read access (see pagePerm) before returning the page to a user.
func (s *Server) loadPage(ctx context.Context, viewerID, pageID string) (*models.Page, error) {
	var p models.Page
	var content []byte
	err := s.Pool.QueryRow(ctx,
		`SELECT id, owner_id, parent_id, space_id, title, content, icon, is_public, public_token, created_at, updated_at
		 FROM pages WHERE id = $1 AND deleted_at IS NULL`, pageID).Scan(
		&p.ID, &p.OwnerID, &p.ParentID, &p.SpaceID, &p.Title, &content, &p.Icon,
		&p.IsPublic, &p.PublicToken, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Content = json.RawMessage(content)

	p.Tags = []models.Tag{}
	rows, err := s.Pool.Query(ctx, `SELECT t.id, t.name, t.color FROM tags t
		JOIN page_tags pt ON pt.tag_id = t.id WHERE pt.page_id = $1 ORDER BY t.name`, pageID)
	if err == nil {
		for rows.Next() {
			var t models.Tag
			if err := rows.Scan(&t.ID, &t.Name, &t.Color); err == nil {
				p.Tags = append(p.Tags, t)
			}
		}
		rows.Close()
	}

	if viewerID != "" {
		_, canEdit, isOwner, _ := s.pagePerm(ctx, viewerID, pageID)
		p.CanEdit = canEdit
		p.IsOwner = isOwner
		var exists bool
		_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM favorites WHERE user_id=$1 AND page_id=$2)`,
			viewerID, pageID).Scan(&exists)
		p.IsFavorite = exists
	}
	return &p, nil
}
