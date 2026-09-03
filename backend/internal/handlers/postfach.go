// The inbox: what was addressed to an account since it last looked.
//
// Until now a comment reached nobody. It sat under a page and waited for
// somebody to open that page again by chance, and a question that stays
// unanswered for a week has stopped being a question. The same for a page
// somebody shared with you: it appeared in a list you had no reason to check.
//
// Three things land here, and deliberately only three: a comment on a page you
// own, a reply to a comment you wrote, and your name in somebody's comment.
// Everything else, a page changed, a tag added, somebody looked at your work
// is noise, and an inbox that carries noise is an inbox people stop opening.
package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// The kinds. They are stored as strings in the row because the interface needs
// them to phrase the sentence; "replied to your comment" reads differently from
// "mentioned you".
const (
	PostKommentar = "kommentar"
	PostAntwort   = "antwort"
	PostErwaehnt  = "erwaehnung"
	PostFreigabe  = "freigabe"
)

// Nachricht is one entry in the inbox.
type Nachricht struct {
	ID            string  `json:"id"`
	Art           string  `json:"art"`
	PageID        *string `json:"pageId"`
	KommentarID   *string `json:"kommentarId"`
	AusloeserID   *string `json:"ausloeserId"`
	AusloeserName string  `json:"ausloeserName"`
	// Der Stand des Profilbildes dessen, der die Nachricht ausgelöst hat, damit
	// in der Liste ein Gesicht steht und nicht nur ein Name. Fehlt, wenn das
	// Konto keines hat oder inzwischen gelöscht wurde.
	AusloeserBild *time.Time `json:"ausloeserBild,omitempty"`
	SeitenTitel   string     `json:"seitenTitel"`
	Text          string     `json:"text"`
	GelesenAm     *time.Time `json:"gelesenAm"`
	ErstelltAm    time.Time  `json:"erstelltAm"`
}

// maxAuszug is the length of the excerpt that is stored along. Enough to see
// what it is about, without turning the table into a second copy of all
// comments.
const maxAuszug = 280

// zustellen creates a message.
//
// Everything that can go wrong is swallowed: a comment that cannot be saved
// because the notification about it fails would be the wrong order of
// importance. The error goes into the log, the comment stands.
func (s *Server) zustellen(ctx context.Context, empfaenger, art, pageID, kommentarID,
	ausloeserID, ausloeserName, seitenTitel, text string) {

	// Nobody gets mail from themselves. That is not a nicety: whoever writes a
	// comment on their own page would otherwise trigger a message to themselves
	// with every sentence.
	if empfaenger == "" || empfaenger == ausloeserID {
		return
	}
	if len(text) > maxAuszug {
		text = strings.TrimSpace(text[:maxAuszug]) + " …"
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO postfach (empfaenger_id, art, page_id, kommentar_id,
		                       ausloeser_id, ausloeser_name, seiten_titel, text)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		empfaenger, art, nullWennLeer(pageID), nullWennLeer(kommentarID),
		nullWennLeer(ausloeserID), ausloeserName, seitenTitel, text)
	if err != nil {
		log.Printf("Postfach (%s an %s): %v", art, empfaenger, err)
	}
}

// ListPostfach returns an account's messages, newest first.
func (s *Server) ListPostfach(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	bedingung := ""
	if r.URL.Query().Get("ungelesen") != "" {
		bedingung = " AND p.gelesen_am IS NULL"
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT p.id, p.art, p.page_id, p.kommentar_id, p.ausloeser_id, p.ausloeser_name,
		        p.seiten_titel, p.text, p.gelesen_am, p.erstellt_am, u.bild_stand
		 FROM postfach p
		 -- LEFT JOIN, denn ausloeser_id wird beim Löschen eines Kontos auf NULL
		 -- gesetzt: die Nachricht bleibt, das Gesicht dazu nicht.
		 LEFT JOIN users u ON u.id = p.ausloeser_id
		 WHERE p.empfaenger_id = $1`+bedingung+`
		 ORDER BY p.erstellt_am DESC LIMIT 100`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Postfach nicht lesbar")
		return
	}
	defer rows.Close()

	liste := []Nachricht{}
	for rows.Next() {
		var n Nachricht
		if err := rows.Scan(&n.ID, &n.Art, &n.PageID, &n.KommentarID, &n.AusloeserID,
			&n.AusloeserName, &n.SeitenTitel, &n.Text, &n.GelesenAm, &n.ErstelltAm,
			&n.AusloeserBild); err == nil {
			liste = append(liste, n)
		}
	}
	writeJSON(w, http.StatusOK, liste)
}

// PostfachAnzahl returns only the number of unread messages.
//
// A call of its own, because the sidebar repeats it regularly: fetching one
// number is a different thing from fetching a hundred rows and throwing them
// away.
func (s *Server) PostfachAnzahl(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	var n int
	err := s.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM postfach WHERE empfaenger_id = $1 AND gelesen_am IS NULL`, uid).Scan(&n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Postfach nicht lesbar")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"ungelesen": n})
}

// PostfachGelesen marks one message as read, or all of them.
//
// The recipient is part of the condition and not of an earlier check: that way
// a foreign id can achieve nothing, no matter where it comes from.
func (s *Server) PostfachGelesen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	var err error
	if id == "" || id == "alle" {
		_, err = s.Pool.Exec(r.Context(),
			`UPDATE postfach SET gelesen_am = now()
			 WHERE empfaenger_id = $1 AND gelesen_am IS NULL`, uid)
	} else {
		_, err = s.Pool.Exec(r.Context(),
			`UPDATE postfach SET gelesen_am = now()
			 WHERE id = $2 AND empfaenger_id = $1 AND gelesen_am IS NULL`, uid, id)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht gespeichert")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PostfachLeeren throws away the read messages. The unread ones stay, otherwise
// the button would be a way to miss something one has never seen.
func (s *Server) PostfachLeeren(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM postfach WHERE empfaenger_id = $1 AND gelesen_am IS NOT NULL`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "nicht gelöscht")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"geloescht": tag.RowsAffected()})
}

// erwaehnte finds the accounts named with an @ in a text.
//
// Compared against the instance's list of names rather than against a pattern:
// names contain spaces, and "@Anna Schmidt" could otherwise not be told apart
// from "@Anna" followed by a surname. For an instance of this size the list is
// cheap; for ten thousand accounts this would be the wrong way.
//
// Only somebody allowed to read the page gets mail. Otherwise the mention would
// be a way to deliver an excerpt of a page to somebody not allowed to see it.
func (s *Server) erwaehnte(ctx context.Context, text, pageID string) map[string]string {
	treffer := map[string]string{}
	if !strings.Contains(text, "@") {
		return treffer
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text, name FROM users WHERE name <> ''`)
	if err != nil {
		return treffer
	}
	defer rows.Close()

	klein := strings.ToLower(text)
	for rows.Next() {
		var id, name string
		if rows.Scan(&id, &name) != nil {
			continue
		}
		if !strings.Contains(klein, "@"+strings.ToLower(name)) {
			continue
		}
		if canRead, _, _, ok := s.pagePerm(ctx, id, pageID); !ok || !canRead {
			continue
		}
		treffer[id] = name
	}
	return treffer
}

// seitenTitel reads the title of a page for the message. It is copied and not
// linked, so that the row still says something when the page has been renamed
// in the meantime.
func (s *Server) seitenTitel(ctx context.Context, pageID string) string {
	var titel string
	_ = s.Pool.QueryRow(ctx, `SELECT title FROM pages WHERE id = $1`, pageID).Scan(&titel)
	if titel == "" {
		return "Ohne Titel"
	}
	return titel
}
