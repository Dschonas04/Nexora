// Watching a page: an inbox message when somebody else changes it.
//
// The inbox carries comments, replies, mentions and shares to everybody they
// concern. "Page changed" is different: for most readers it is noise, and an
// inbox full of noise is one people stop opening. So it reaches only those who
// asked for it, page by page.
//
// The autosave writes every half second while somebody types. One message per
// save would bury the inbox, so a watcher gets ONE message per page until they
// have read it; whatever changes after that is in the page anyway.
package handlers

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// beobachterReihe serialises the deliveries; see beobachterBenachrichtigen.
var beobachterReihe sync.Mutex

// Beobachten starts watching a page. Whoever may read it may watch it.
func (s *Server) Beobachten(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if _, err := s.Pool.Exec(r.Context(),
		`INSERT INTO beobachter (user_id, page_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "watch failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"watching": true})
}

// NichtBeobachten stops watching. Scoped to the caller, so no permission check
// is needed: a row that is not there is simply not deleted.
func (s *Server) NichtBeobachten(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if _, err := s.Pool.Exec(r.Context(),
		`DELETE FROM beobachter WHERE user_id=$1 AND page_id=$2`, uid, chi.URLParam(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, "unwatch failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"watching": false})
}

// beobachterBenachrichtigen tells the watchers of a page that it changed.
//
// In a goroutine of its own: the save that triggered it is done, and the
// person typing must not wait for the inbox of others. Every watcher is checked
// against the page again -- a share withdrawn since they started watching
// must not keep delivering news about the page.
func (s *Server) beobachterBenachrichtigen(pageID, akteurID string) {
	go func() {
		// One at a time: two saves half a second apart would otherwise both
		// find no unread message and both deliver one.
		beobachterReihe.Lock()
		defer beobachterReihe.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rows, err := s.Pool.Query(ctx,
			`SELECT b.user_id::text FROM beobachter b
			 WHERE b.page_id = $1 AND b.user_id <> $2
			   AND NOT EXISTS (SELECT 1 FROM postfach p
			                    WHERE p.empfaenger_id = b.user_id AND p.page_id = b.page_id
			                      AND p.art = $3 AND p.gelesen_am IS NULL)`,
			pageID, akteurID, PostAenderung)
		if err != nil {
			log.Printf("Beobachter (%s): %v", pageID, err)
			return
		}
		var empfaenger []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				empfaenger = append(empfaenger, id)
			}
		}
		rows.Close()
		if len(empfaenger) == 0 {
			return
		}
		var name string
		_ = s.Pool.QueryRow(ctx, `SELECT name FROM users WHERE id=$1`, akteurID).Scan(&name)
		titel := s.seitenTitel(ctx, pageID)
		for _, e := range empfaenger {
			if _, _, _, ok := s.pagePerm(ctx, e, pageID); !ok {
				continue
			}
			s.zustellen(ctx, e, PostAenderung, pageID, "", akteurID, name, titel, "")
		}
	}()
}
