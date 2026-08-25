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

// Die Arten. Sie stehen als Zeichenketten in der Zeile, weil die Oberfläche
// sie zum Formulieren des Satzes braucht, "hat auf deinen Kommentar
// geantwortet" liest sich anders als "hat dich erwähnt".
const (
	PostKommentar = "kommentar"
	PostAntwort   = "antwort"
	PostErwaehnt  = "erwaehnung"
	PostFreigabe  = "freigabe"
)

// Nachricht is one entry in the inbox.
type Nachricht struct {
	ID            string     `json:"id"`
	Art           string     `json:"art"`
	PageID        *string    `json:"pageId"`
	KommentarID   *string    `json:"kommentarId"`
	AusloeserName string     `json:"ausloeserName"`
	SeitenTitel   string     `json:"seitenTitel"`
	Text          string     `json:"text"`
	GelesenAm     *time.Time `json:"gelesenAm"`
	ErstelltAm    time.Time  `json:"erstelltAm"`
}

// maxAuszug ist die Länge des Ausschnitts, der mitgeschrieben wird. Genug, um
// zu erkennen, worum es geht, ohne die Tabelle zu einer zweiten Kopie aller
// Kommentare zu machen.
const maxAuszug = 280

// zustellen legt eine Nachricht an.
//
// Alles wird geschluckt, was schiefgehen kann: ein Kommentar, der sich nicht
// speichern lässt, weil die Benachrichtigung darüber scheitert, wäre die
// falsche Rangfolge. Der Fehler geht ins Protokoll, der Kommentar steht.
func (s *Server) zustellen(ctx context.Context, empfaenger, art, pageID, kommentarID,
	ausloeserID, ausloeserName, seitenTitel, text string) {

	// Niemand bekommt Post von sich selbst. Das ist keine Feinheit: wer einen
	// Kommentar auf der eigenen Seite schreibt, löste sonst mit jedem Satz eine
	// Nachricht an sich aus.
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
		bedingung = " AND gelesen_am IS NULL"
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, art, page_id, kommentar_id, ausloeser_name, seiten_titel,
		        text, gelesen_am, erstellt_am
		 FROM postfach WHERE empfaenger_id = $1`+bedingung+`
		 ORDER BY erstellt_am DESC LIMIT 100`, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Postfach nicht lesbar")
		return
	}
	defer rows.Close()

	liste := []Nachricht{}
	for rows.Next() {
		var n Nachricht
		if err := rows.Scan(&n.ID, &n.Art, &n.PageID, &n.KommentarID, &n.AusloeserName,
			&n.SeitenTitel, &n.Text, &n.GelesenAm, &n.ErstelltAm); err == nil {
			liste = append(liste, n)
		}
	}
	writeJSON(w, http.StatusOK, liste)
}

// PostfachAnzahl liefert nur die Zahl der ungelesenen Nachrichten.
//
// Eigener Aufruf, weil die Leiste ihn regelmäßig wiederholt: eine Zahl zu
// holen ist etwas anderes, als hundert Zeilen zu holen und sie wegzuwerfen.
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

// PostfachGelesen markiert eine Nachricht als gelesen, oder alle.
//
// Der Empfänger steht in der Bedingung, nicht in einer vorherigen Prüfung: so
// kann eine fremde Kennung nichts bewirken, egal woher sie kommt.
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

// PostfachLeeren wirft die gelesenen Nachrichten weg. Die ungelesenen bleiben
// sonst wäre der Knopf ein Weg, etwas zu übersehen, das man nie gesehen hat.
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

// erwaehnte findet die Konten, die in einem Text mit @ genannt werden.
//
// Verglichen wird gegen die Namensliste der Instanz und nicht gegen ein Muster:
// Namen enthalten Leerzeichen, und "@Anna Schmidt" ließe sich sonst nicht von
// "@Anna" gefolgt von einem Nachnamen unterscheiden. Für eine Instanz dieser
// Größe ist die Liste billig; für zehntausend Konten wäre das der falsche Weg.
//
// Nur wer die Seite lesen darf, bekommt Post. Sonst wäre die Erwähnung ein
// Weg, jemandem den Ausschnitt einer Seite zuzustellen, die er nicht sehen darf.
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

// seitenTitel liest den Titel einer Seite für die Nachricht. Er wird kopiert
// und nicht verknüpft, damit die Zeile auch dann noch etwas sagt, wenn die
// Seite inzwischen umbenannt wurde.
func (s *Server) seitenTitel(ctx context.Context, pageID string) string {
	var titel string
	_ = s.Pool.QueryRow(ctx, `SELECT title FROM pages WHERE id = $1`, pageID).Scan(&titel)
	if titel == "" {
		return "Ohne Titel"
	}
	return titel
}
