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

// ueberTLS sagt, ob der BROWSER verschlüsselt spricht -- nicht, ob diese eine
// Verbindung verschlüsselt ist.
//
// Der Unterschied ist der Grund für diese Funktion. Seit der Innenverkehr des
// Verbunds verschlüsselt ist, kommt jede Anfrage über TLS bei diesem Dienst an,
// auch wenn davor jemand über gewöhnliches HTTP auf die Oberfläche zugreift.
// Würde hier r.TLS entscheiden, bekäme dieser Jemand einen Keks mit dem
// Merkmal Secure, sein Browser würde ihn über HTTP nicht zurückschicken, und er
// wäre nach der Anmeldung sofort wieder abgemeldet. Genau das ist am 02.09.2026
// passiert.
//
// Deshalb zählt X-Forwarded-Proto zuerst: das ist die Angabe des Gegenstücks
// darüber, womit der Browser gekommen ist. Erst wenn niemand davorsteht, gilt
// die eigene Verbindung.
func ueberTLS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if weg := r.Header.Get("X-Forwarded-Proto"); weg != "" {
		return strings.EqualFold(weg, "https")
	}
	return r.TLS != nil
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

// gemeinsamBeschrieben sagt, ob an dieser Seite mehr als ein Konto schreiben
// darf: durch eine Freigabe mit Bearbeitungsrecht, durch ein Recht auf ihrem
// Bereich oder weil der Bereich für alle offen steht.
//
// Die Frage entscheidet, ob der Browser die Leitung zum gemeinsamen Schreiben
// öffnet. Sie fällt bewusst hier und nicht dort: eine Seite, die nur ihrem
// Besitzer gehört, soll gar nicht erst eine Sitzung aufmachen, in der nie
// jemand zweites sitzen wird.
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
// viewer, its favorite flag and permission flags. viewerID == "" is used for
// public pages and skips per-user fields. Callers are responsible for checking
// read access (see pagePerm) before returning the page to a user.
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
