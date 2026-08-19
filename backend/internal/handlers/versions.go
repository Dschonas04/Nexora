// Version history. Snapshots are written by the page update path, never by the
// client, so history cannot be forged or skipped from the frontend.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// snapshotVersion records the current page state in the history, coalescing
// rapid successive edits: a new snapshot is skipped if one was written for this
// page within the last two minutes. Without that window autosave would write a
// snapshot every few seconds and bury the useful revisions.
//
// Errors are ignored on purpose. A failed snapshot must not block the edit
// itself, since losing a history entry is far less bad than losing the save.
func (s *Server) snapshotVersion(ctx context.Context, cur *models.Page, authorID string) {
	var recent bool
	_ = s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM page_versions
		 WHERE page_id=$1 AND created_at > now() - interval '2 minutes')`, cur.ID).Scan(&recent)
	if recent {
		return
	}
	_, _ = s.Pool.Exec(ctx,
		`INSERT INTO page_versions (page_id, title, content, icon, author_id)
		 VALUES ($1, $2, $3::jsonb, $4, $5)`,
		cur.ID, cur.Title, string(cur.Content), cur.Icon, authorID)
}

// ListVersions returns the history of a page without the content, which keeps
// the response small; the panel fetches a single version when one is opened.
// Capped at 100 entries, so a page edited for years still loads.
// The author is LEFT JOINed because a deleted account leaves author_id NULL.
func (s *Server) ListVersions(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT v.id, v.title, v.icon, COALESCE(u.name, 'unknown'), v.created_at
		 FROM page_versions v LEFT JOIN users u ON u.id = v.author_id
		 WHERE v.page_id=$1 ORDER BY v.created_at DESC LIMIT 100`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.PageVersion{}
	for rows.Next() {
		var v models.PageVersion
		if err := rows.Scan(&v.ID, &v.Title, &v.Icon, &v.AuthorName, &v.CreatedAt); err == nil {
			list = append(list, v)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// GetVersion returns one version with its content, for the preview pane. The
// version id is matched together with the page id so a version from a page the
// caller may not read cannot be fetched through a page they can.
func (s *Server) GetVersion(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	var v models.PageVersion
	var content []byte
	err := s.Pool.QueryRow(r.Context(),
		`SELECT v.id, v.title, v.content, v.icon, COALESCE(u.name, 'unknown'), v.created_at
		 FROM page_versions v LEFT JOIN users u ON u.id = v.author_id
		 WHERE v.id=$1 AND v.page_id=$2`, chi.URLParam(r, "versionId"), id).Scan(
		&v.ID, &v.Title, &content, &v.Icon, &v.AuthorName, &v.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "version not found")
		return
	}
	v.Content = json.RawMessage(content)
	writeJSON(w, http.StatusOK, v)
}

// RestoreVersion overwrites the page with an older revision. The current state
// is snapshotted first, so restoring is itself undoable. Nothing is deleted from
// the history: a restore only adds to it.
func (s *Server) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "read-only access")
		return
	}

	var title, icon string
	var content []byte
	err := s.Pool.QueryRow(r.Context(),
		`SELECT title, content, icon FROM page_versions WHERE id=$1 AND page_id=$2`,
		chi.URLParam(r, "versionId"), id).Scan(&title, &content, &icon)
	if err != nil {
		writeErr(w, http.StatusNotFound, "version not found")
		return
	}

	if cur, err := s.loadPage(r.Context(), uid, id); err == nil {
		s.snapshotVersion(r.Context(), cur, uid)
	}

	_, err = s.Pool.Exec(r.Context(),
		// Auch hier muss content_text mit -- ein zurückgeholter Stand, den die
		// Suche nicht findet, wäre schlimmer als gar keine Suche.
		`UPDATE pages SET title=$2, content=$3::jsonb, icon=$4,
		        content_text=$5, updated_at=now() WHERE id=$1`,
		id, title, string(content), icon, textAusInhalt(content))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "restore failed")
		return
	}
	page, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}
