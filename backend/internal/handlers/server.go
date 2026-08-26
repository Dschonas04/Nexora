// Package handlers implements the HTTP API. Every handler is a method on
// Server, reads the caller from the request context and decides access itself;
// see access.go for the two helpers all of them rely on.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexora/internal/ablage"
	"nexora/internal/models"
)

const (
	cookieName = "nexora_token"
	// Fallback when no setting applies. The actual duration comes from
	// SitzungDauer and can be changed while running. Sessions themselves are kept
	// in the database, so a token can be revoked and renewed; this value only
	// says how long a freshly signed one stays valid.
	tokenTTL = 7 * 24 * time.Hour
)

// Server carries everything the handlers need. It is created once in main and
// shared by all requests, so it must stay read-only after startup.
type Server struct {
	Pool   *pgxpool.Pool
	Secret []byte
	// Sitzungen is the short lived cache for the session check. May be nil, then
	// every request asks the database.
	Sitzungen *sitzungsSpeicher
	// Redis is the shared cache across several instances. May be nil: without it
	// everything keeps working, only without shared storage.
	Redis *RedisSpeicher
	// SSO carries the values for signing in through an outside identity.
	SSO SSOEinstellungen
	// Ablage decides where the bytes of an attachment lie: on disk or in an S3
	// bucket. The handlers do not know the difference.
	Ablage ablage.Ablage
}

// writeJSON sends v as JSON. An encoding error is ignored on purpose: the
// status line is already on the wire at that point, so there is nothing left to
// report to the client.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr sends {"error": msg}, the single error shape the frontend expects.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// setAuthCookie installs the session cookie. It is httpOnly so page script
// cannot read it, and SameSite=Lax keeps it off cross-site requests while still
// surviving a normal link into the app. Secure is not set here because the
// reverse proxy terminates TLS; behind plain HTTP the cookie would otherwise
// never be sent at all.
func (s *Server) setAuthCookie(w http.ResponseWriter, token string) {
	s.setAuthCookieFuer(w, nil, token)
}

// setAuthCookieFuer sets the cookie and marks it Secure when the request came
// over HTTPS.
//
// Setting Secure unconditionally would not work: no cookie would arrive over a
// plain HTTP connection at all, and an instance in a home network without TLS
// would be unusable. The other way round, a session cookie without Secure on an
// HTTPS site is an invitation to grab it through a smuggled in HTTP call.
func (s *Server) setAuthCookieFuer(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   ueberTLS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(SitzungDauer()),
		MaxAge:   int(SitzungDauer().Seconds()),
	})
}

// ueberTLS recognises an encrypted request, also behind a proxy that decrypts
// itself and says so in the X-Forwarded-Proto header.
func ueberTLS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clearAuthCookie expires the cookie. The token itself stays valid until it
// runs out, so logout only clears the browser, not the server.
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
		`SELECT id, owner_id, parent_id, space_id, title, content, icon, is_public, public_token, ist_vorlage, created_at, updated_at
		 FROM pages WHERE id = $1 AND deleted_at IS NULL`, pageID).Scan(
		&p.ID, &p.OwnerID, &p.ParentID, &p.SpaceID, &p.Title, &content, &p.Icon,
		&p.IsPublic, &p.PublicToken, &p.IstVorlage, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Content = json.RawMessage(content)

	// Start from an empty slice, not nil, so the JSON carries [] instead of null
	// and the frontend can iterate without a guard.
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

	// Per-viewer fields. They are computed rather than stored because the same
	// page looks different to its owner, to someone it is shared with, and to an
	// anonymous visitor following a public link.
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
