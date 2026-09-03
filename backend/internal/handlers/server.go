// Package handlers implements the HTTP API. Every handler is a method on
// Server, reads the caller from the request context and decides access itself;
// see access.go for the two helper functions they all rely on.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexora/internal/ablage"
	"nexora/internal/lizenz"
	"nexora/internal/models"
	"nexora/internal/puls"
)

const cookieName = "nexora_token"

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
	// DatenbankURL ist die Adresse, mit der sich pg_dump verbinden kann. Nur
	// dafür: alles andere geht über den Vorrat.
	DatenbankURL string
	// Puls zählt die Anfragen der letzten Minute. Darf nil sein; dann meldet
	// die Systemansicht keine Live-Werte, und sonst ändert sich nichts.
	Puls *puls.Messer
}

// writeJSON sends `v` as JSON. An encoding error is intentionally ignored:
// once the status line is written the response cannot be replaced with an
// error payload.
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

// setAuthCookieFuer sets the session cookie and marks it Secure when the
// original browser-to-proxy connection used HTTPS.
//
// Secure cannot be set unconditionally: clients on plain HTTP would not send
// back cookies marked Secure. Conversely, leaving Secure off on an HTTPS site
// weakens security by allowing cookie leaks via mixed content.
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

// ueberTLS returns whether the BROWSER spoke over TLS, not whether the local
// connection is encrypted.
//
// This distinction matters because proxies terminate TLS. The header
// `X-Forwarded-Proto` describes how the browser connected to the proxy. If
// we relied only on `r.TLS` we might mark cookies Secure for requests that
// arrived over an internal TLS connection even though the browser used plain
// HTTP, causing the browser not to send the cookie back. Therefore X-Forwarded-Proto
// is checked first and r.TLS only as a fallback.
func ueberTLS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if weg := r.Header.Get("X-Forwarded-Proto"); weg != "" {
		return strings.EqualFold(weg, "https")
	}
	return r.TLS != nil
}

// clearAuthCookie expires the cookie on the client. The token may remain
// valid server-side until it is explicitly revoked; clearing the cookie only
// removes it from the browser.
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

// gemeinsamBeschrieben returns whether more than one account may write to
// a page: via an edit share, via space-level rights or because the space is
// open for writing.
//
// This determines whether the browser opens a real-time collaboration channel
// for the page. The check is made here so that pages owned only by their
// creator do not create sessions where a second participant will never join.
func (s *Server) gemeinsamBeschrieben(ctx context.Context, pageID string) bool {
	if !lizenz.Frei(lizenz.Echtzeit) || !s.echtzeitAn() {
		return false
	}
	var mehrere bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM page_shares sh
		                WHERE sh.page_id = p.id AND sh.permission = 'edit')
		    OR EXISTS (SELECT 1 FROM spaces so
		                WHERE so.id = p.space_id AND so.oeffentlich = 'schreiben')
		    OR ($2 AND EXISTS (SELECT 1 FROM space_rechte sr
		                        WHERE sr.space_id = p.space_id
		                          AND sr.recht IN ('schreiben', 'verwalten')))
		  FROM pages p WHERE p.id = $1`,
		pageID, lizenz.Frei(lizenz.Gruppen)).Scan(&mehrere)
	return err == nil && mehrere
}

// loadPage returns a full (non-deleted) page with tags and, for a logged-in
// viewer, per-viewer fields such as favorite and permission flags. A
// viewerID of "" is used for anonymous/public views and omits per-user fields.
// Callers must check read access (see pagePerm) before serving the page.
func (s *Server) loadPage(ctx context.Context, viewerID, pageID string) (*models.Page, error) {
	var p models.Page
	var content []byte
	err := s.Pool.QueryRow(ctx,
		`SELECT id, owner_id, parent_id, space_id, title, content, icon, is_public, public_token, breite, created_at, updated_at
		 FROM pages WHERE id = $1 AND deleted_at IS NULL`, pageID).Scan(
		&p.ID, &p.OwnerID, &p.ParentID, &p.SpaceID, &p.Title, &content, &p.Icon,
		&p.IsPublic, &p.PublicToken, &p.Breite, &p.CreatedAt, &p.UpdatedAt,
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
		p.Gemeinsam = canEdit && s.gemeinsamBeschrieben(ctx, pageID)
	}
	return &p, nil
}
