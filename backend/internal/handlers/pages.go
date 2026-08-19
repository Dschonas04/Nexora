// Page CRUD plus the trash, favorites and public links. The tree lives in one
// table with a self-referencing parent_id, so anything that touches a whole
// subtree does it with a recursive CTE rather than a loop in Go.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// ListPages returns the pages the user owns, flat. The frontend builds the tree
// from parent_id, which keeps this a single query no matter how deep the
// nesting goes. Pages shared with the user are not included; see
// ListSharedPages.
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

	// A row that fails to scan is skipped rather than failing the whole request,
	// so one damaged page cannot make the sidebar unusable.
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

// CreatePage adds an empty page. A decode error is tolerated because an empty
// body is a valid request: it creates an untitled page at the root.
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
	s.spurAusRequest(r, AktSeiteAngelegt, "seite", id, req.Title, nil)
	page, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load failed")
		return
	}
	writeJSON(w, http.StatusCreated, page)
}

// GetPage returns one page in full. No read access and a missing page both
// answer 404, so the response does not reveal that a page exists.
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

// updatePageReq is a patch, not a full replacement: an omitted field keeps its
// current value. ParentID and SpaceID are raw JSON because they have three
// states, and *string could not tell "field absent" from "field set to null".
type updatePageReq struct {
	Title    *string         `json:"title"`
	Content  json.RawMessage `json:"content"`
	Icon     *string         `json:"icon"`
	ParentID json.RawMessage `json:"parentId"` // absent = unchanged, null = move to root
	SpaceID  json.RawMessage `json:"spaceId"`  // absent = unchanged, null = no space
}

// UpdatePage applies a patch to a page. The frontend autosaves, so this runs
// every couple of seconds while someone types.
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

	// Snapshot the state before the edit, not after, so restoring a version puts
	// back what was there previously. Coalescing keeps autosave from filling the
	// history with one entry per keystroke; see snapshotVersion.
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
	// Someone with edit access may change the content but not move the page:
	// re-parenting is a structural change to the owner's workspace, and it could
	// pull a page out of the tree the owner can see.
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

	// content_text wird hier mitgeschrieben, nicht in einem Trigger: der
	// Fließtext lässt sich aus dem BlockNote-JSON nur in Go herausziehen. Wer
	// den Inhalt schreibt, muss ihn deshalb mitschreiben -- sonst zeigt die
	// Suche stillschweigend einen alten Stand.
	_, err = s.Pool.Exec(r.Context(),
		`UPDATE pages SET title=$2, content=$3::jsonb, icon=$4, parent_id=$5, space_id=$6,
		        content_text=$7, updated_at=now()
		 WHERE id=$1`,
		id, title, string(content), icon, parent, space, textAusInhalt(content))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	s.spurAusRequest(r, AktSeiteGeaendert, "seite", id, title, nil)

	page, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// DeletePage moves a page and its whole subtree to the trash. Children have to
// go with it, otherwise they would survive as orphans that no longer appear
// anywhere in the tree. Nothing is removed yet; see PurgePage.
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
	s.spurAusRequest(r, AktSeiteGeloescht, "seite", id, "", nil)
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

// RestorePage brings a page and its deleted subtree back. It restores whatever
// is currently below the page, so a child deleted separately beforehand comes
// back too. Ownership is checked inside the query, hence the RowsAffected test
// instead of a separate lookup.
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
	s.spurAusRequest(r, AktSeiteWieder, "seite", chi.URLParam(r, "id"), "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PurgePage deletes a page for good. The database cascades to its subpages,
// versions, attachment rows, shares and links. Only a trashed page can be
// purged, so a stray request cannot wipe a live page. Note that attachment
// files on disk are not removed here.
func (s *Server) PurgePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	// Der Titel wird VOR dem Löschen gelesen. Danach gibt es die Zeile nicht
	// mehr, und ein Prüfspureintrag "Seite <uuid> endgültig gelöscht" ohne
	// Namen ist für eine Revision wertlos.
	var titel string
	_ = s.Pool.QueryRow(r.Context(), `SELECT title FROM pages WHERE id=$1`, id).Scan(&titel)

	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM pages WHERE id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL`,
		id, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "purge failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	s.spurAusRequest(r, AktSeiteEntfernt, "seite", id, titel, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AddFavorite pins a page for this user. ON CONFLICT DO NOTHING makes a second
// click harmless instead of an error.
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

// RemoveFavorite unpins a page. Deleting a row that is not there is not an
// error, so no permission check is needed: the WHERE clause is scoped to the
// caller anyway.
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

// SharePage publishes a page read-only behind an unguessable token. COALESCE
// keeps an existing token, so re-publishing a page does not break links that
// have already been handed out. The token is 16 random bytes from the database,
// which is far too large to enumerate.
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
	s.spurAusRequest(r, AktOeffentlichAn, "seite", chi.URLParam(r, "id"), "", nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"isPublic": true, "publicToken": token})
}

// UnsharePage withdraws the public link and drops the token, so a new
// publication issues a fresh one and every old link stays dead.
func (s *Server) UnsharePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	_, err := s.Pool.Exec(r.Context(),
		`UPDATE pages SET is_public=false, public_token=NULL WHERE id=$1 AND owner_id=$2`,
		chi.URLParam(r, "id"), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unshare failed")
		return
	}
	s.spurAusRequest(r, AktOeffentlichAus, "seite", chi.URLParam(r, "id"), "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"isPublic": false})
}
