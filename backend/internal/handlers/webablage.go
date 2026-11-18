// SPDX-License-Identifier: LicenseRef-Nexora-Enterprise
// Copyright (c) 2026 Jonas Groll. See LICENSE-ENTERPRISE.

// Public spaces: a whole space published on the web.
//
// The share link of a single page passes one page on. A company that keeps its
// handbook, product documentation or help pages in Nexora wants more: the whole
// space under one address, with its page tree, readable by anyone without an
// account. That is this file.
//
// The rules are those of the single page link, applied to a space: one random
// token is the whole authorization, withdrawing drops the token, and only what
// belongs to the space leaves the instance. Pages in the trash stay hidden,
// files of pages outside the space stay locked, and nobody can write.
package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// webDateien is where files of a published space are served.
func webDateien(token string) string { return "/api/web/" + token + "/files/" }

type spaceWebReq struct {
	An bool `json:"published"`
}

// SetSpaceWeb publishes a space on the web or takes it down again.
func (s *Server) SetSpaceWeb(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if !s.darfSpaceVerwalten(r.Context(), uid, id) {
		writeErr(w, http.StatusForbidden, "nur Eigentümer, Verwalter oder Admin")
		return
	}
	var req spaceWebReq
	_ = decode(r, &req)

	var token *string
	var err error
	if req.An {
		err = s.Pool.QueryRow(r.Context(),
			`UPDATE spaces SET web_token = COALESCE(web_token, encode(gen_random_bytes(16), 'hex'))
			  WHERE id=$1 RETURNING web_token`, id).Scan(&token)
	} else {
		err = s.Pool.QueryRow(r.Context(),
			`UPDATE spaces SET web_token = NULL WHERE id=$1 RETURNING web_token`, id).Scan(&token)
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "space not found")
		return
	}
	s.spurAusRequest(r, AktSpaceWeb, "space", id, "", map[string]any{"veroeffentlicht": req.An})
	wert := ""
	if token != nil {
		wert = *token
	}
	writeJSON(w, http.StatusOK, map[string]any{"published": req.An, "webToken": wert})
}

type webSeiteKurz struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
	Title    string  `json:"title"`
	Icon     string  `json:"icon"`
}

// webBerechtigt limits a query on pages (alias p) to pages whose owner may
// write in the space. A page owner can set the space of their own page freely,
// so without this anybody could slip a page into a company's published space
// and put it on the web under that name.
const webBerechtigt = `(p.owner_id = sp.owner_id
	   OR sp.oeffentlich = 'schreiben'
	   OR EXISTS (SELECT 1 FROM users u WHERE u.id = p.owner_id AND u.role = 'admin')
	   OR EXISTS (SELECT 1 FROM space_rechte sr
	               WHERE sr.space_id = sp.id AND sr.recht IN ('schreiben', 'verwalten')
	                 AND (sr.user_id = p.owner_id
	                      OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
	                                          WHERE gm.user_id = p.owner_id))))`

// webSpace finds the published space behind a token.
func (s *Server) webSpace(r *http.Request) (id, name string, ok bool) {
	token := chi.URLParam(r, "token")
	if len(token) < 16 {
		return "", "", false
	}
	err := s.Pool.QueryRow(r.Context(),
		`SELECT id::text, name FROM spaces WHERE web_token = $1`, token).Scan(&id, &name)
	return id, name, err == nil
}

// webSeiten lists the pages of a published space in the order of the sidebar.
func (s *Server) webSeiten(r *http.Request, spaceID string) ([]webSeiteKurz, error) {
	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id::text, p.parent_id::text, p.title, p.icon
		   FROM pages p JOIN spaces sp ON sp.id = p.space_id
		  WHERE p.space_id = $1 AND p.deleted_at IS NULL AND `+webBerechtigt+`
		  ORDER BY p.sort_order, p.created_at`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	liste := []webSeiteKurz{}
	for rows.Next() {
		var p webSeiteKurz
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Title, &p.Icon); err == nil {
			liste = append(liste, p)
		}
	}
	// A parent outside the space (or in the trash) is not published: the page
	// then stands at the top instead of hanging from something invisible.
	da := map[string]bool{}
	for _, p := range liste {
		da[p.ID] = true
	}
	for i := range liste {
		if liste[i].ParentID != nil && !da[*liste[i].ParentID] {
			liste[i].ParentID = nil
		}
	}
	return liste, rows.Err()
}

// GetWebAblage returns name and page tree of a published space.
func (s *Server) GetWebAblage(w http.ResponseWriter, r *http.Request) {
	id, name, ok := s.webSpace(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	seiten, err := s.webSeiten(r, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "pages": seiten})
}

type webSeite struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Icon      string          `json:"icon"`
	Breite    string          `json:"width"`
	Content   json.RawMessage `json:"content"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// GetWebSeite returns one page of a published space.
func (s *Server) GetWebSeite(w http.ResponseWriter, r *http.Request) {
	spaceID, _, ok := s.webSpace(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var p webSeite
	var inhalt []byte
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT p.id::text, p.title, p.content, p.icon, p.breite, p.updated_at
		   FROM pages p JOIN spaces sp ON sp.id = p.space_id
		  WHERE p.id::text = $1 AND p.space_id = $2 AND p.deleted_at IS NULL AND `+webBerechtigt,
		chi.URLParam(r, "pageId"), spaceID).
		Scan(&p.ID, &p.Title, &inhalt, &p.Icon, &p.Breite, &p.UpdatedAt); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// Pictures and files of every page in the space move onto the public path.
	// A file of a page outside the space keeps its locked address.
	seiten, _ := s.webSeiten(r, spaceID)
	text := string(inhalt)
	ziel := webDateien(chi.URLParam(r, "token"))
	for _, x := range seiten {
		text = strings.ReplaceAll(text, "/api/pages/"+x.ID+"/attachments/", ziel)
	}
	p.Content = json.RawMessage(text)
	writeJSON(w, http.StatusOK, p)
}

// WebDatei serves a file of a page in a published space.
func (s *Server) WebDatei(w http.ResponseWriter, r *http.Request) {
	spaceID, _, ok := s.webSpace(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "nicht gefunden")
		return
	}
	attID := chi.URLParam(r, "attId")
	var filename, mime string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT a.filename, a.mime
		   FROM attachments a
		   JOIN pages p ON p.id = a.page_id
		   JOIN spaces sp ON sp.id = p.space_id
		  WHERE a.id::text = $1 AND p.space_id = $2 AND p.deleted_at IS NULL AND `+webBerechtigt,
		attID, spaceID).Scan(&filename, &mime); err != nil {
		writeErr(w, http.StatusNotFound, "nicht gefunden")
		return
	}
	f, err := s.Ablage.Lesen(r.Context(), attID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Datei fehlt")
		return
	}
	defer f.Close()
	// Same header as everywhere: an uploaded HTML or SVG file must not become a
	// document on this origin.
	anhangKopf(w, mime, filename)
	io.Copy(w, f)
}
