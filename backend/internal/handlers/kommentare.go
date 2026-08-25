// Comments on a page.
//
// Who may do what follows the page, not the comment: anyone who can read the
// page can read and write comments on it, because a comment nobody may answer
// is not a conversation. Editing and deleting stay with the author, plus the
// page owner and admins — someone has to be able to clear out an insult without
// asking the person who wrote it.
package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// Kommentar is one entry. A deleted one keeps its place in the thread with an
// empty text, so the replies hanging off it do not lose their context.
type Kommentar struct {
	ID          string     `json:"id"`
	ElternID    *string    `json:"elternId"`
	AutorID     string     `json:"autorId"`
	AutorName   string     `json:"autorName"`
	Text        string     `json:"text"`
	Erledigt    bool       `json:"erledigt"`
	ErstelltAm  time.Time  `json:"erstelltAm"`
	GeaendertAm *time.Time `json:"geaendertAm"`
	Geloescht   bool       `json:"geloescht"`
	/** True when the caller may edit or delete this one. Computed per request. */
	Darf bool `json:"darf"`
}

const maxKommentarLaenge = 10_000

// ListKommentare returns every comment on a page, oldest first. The interface
// builds the two levels out of elternId; sending a flat list keeps the query
// simple and the order unambiguous.
func (s *Server) ListKommentare(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	canRead, _, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || !canRead {
		writeErr(w, http.StatusForbidden, "kein Zugriff auf diese Seite")
		return
	}
	admin := s.isAdmin(r.Context(), uid)

	rows, err := s.Pool.Query(r.Context(),
		`SELECT k.id, k.eltern_id, coalesce(k.autor_id::text, ''), k.autor_name,
		        k.text, k.erledigt, k.erstellt_am, k.geaendert_am, k.geloescht_am
		 FROM kommentare k
		 WHERE k.page_id = $1
		 ORDER BY k.erstellt_am ASC`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Kommentare konnten nicht gelesen werden")
		return
	}
	defer rows.Close()

	list := []Kommentar{}
	for rows.Next() {
		var k Kommentar
		var geloescht *time.Time
		if err := rows.Scan(&k.ID, &k.ElternID, &k.AutorID, &k.AutorName,
			&k.Text, &k.Erledigt, &k.ErstelltAm, &k.GeaendertAm, &geloescht); err != nil {
			continue
		}
		if geloescht != nil {
			k.Geloescht = true
			// Der Text wird hier verworfen, nicht erst im Browser. Was der
			// Server nicht sendet, kann auch niemand aus der Antwort fischen.
			k.Text = ""
		}
		k.Darf = !k.Geloescht && (k.AutorID == uid || isOwner || admin)
		list = append(list, k)
	}
	writeJSON(w, http.StatusOK, list)
}

type kommentarReq struct {
	Text     string  `json:"text"`
	ElternID *string `json:"elternId"`
}

// CreateKommentar adds a comment, or a reply to one.
func (s *Server) CreateKommentar(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	canRead, _, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok || !canRead {
		writeErr(w, http.StatusForbidden, "kein Zugriff auf diese Seite")
		return
	}

	var req kommentarReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "Kommentar ist leer")
		return
	}
	if len(req.Text) > maxKommentarLaenge {
		writeErr(w, http.StatusBadRequest, "Kommentar ist zu lang")
		return
	}

	// Nur eine Antwortebene. Wird auf eine Antwort geantwortet, hängt der neue
	// Beitrag an deren Elternteil, der Faden bleibt flach statt sich immer
	// weiter einzurücken.
	if req.ElternID != nil {
		var grosseltern *string
		var elternSeite string
		err := s.Pool.QueryRow(r.Context(),
			`SELECT eltern_id, page_id::text FROM kommentare WHERE id=$1`,
			*req.ElternID).Scan(&grosseltern, &elternSeite)
		if err != nil || elternSeite != id {
			// Ein Elternteil von einer anderen Seite wäre ein Weg, Kommentare
			// über Seitengrenzen einzuschleusen.
			writeErr(w, http.StatusBadRequest, "Bezugskommentar gehört nicht zu dieser Seite")
			return
		}
		if grosseltern != nil {
			req.ElternID = grosseltern
		}
	}

	var name string
	_ = s.Pool.QueryRow(r.Context(), `SELECT name FROM users WHERE id=$1`, uid).Scan(&name)

	var k Kommentar
	err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO kommentare (page_id, eltern_id, autor_id, autor_name, text)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, eltern_id, autor_id::text, autor_name, text, erledigt, erstellt_am`,
		id, req.ElternID, uid, name, req.Text).
		Scan(&k.ID, &k.ElternID, &k.AutorID, &k.AutorName, &k.Text, &k.Erledigt, &k.ErstelltAm)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Kommentar konnte nicht gespeichert werden")
		return
	}
	k.Darf = true

	s.spurAusRequest(r, AktKommentar, "seite", id, "", nil)
	s.postfachAusKommentar(r.Context(), uid, name, id, k.ID, req.ElternID, req.Text)
	writeJSON(w, http.StatusCreated, k)
}

// postfachAusKommentar stellt zu, was dieser Kommentar auslöst.
//
// Die Reihenfolge ist eine Rangfolge: wer erwähnt wurde, bekommt die Erwähnung
// und nicht zusätzlich die allgemeine Nachricht. Zwei Zeilen für denselben
// Kommentar wären keine doppelte Aufmerksamkeit, sondern doppelte Arbeit beim
// Wegräumen.
func (s *Server) postfachAusKommentar(ctx context.Context, uid, name, pageID, kommentarID string, elternID *string, text string) {
	titel := s.seitenTitel(ctx, pageID)
	bedient := map[string]bool{uid: true}

	for id := range s.erwaehnte(ctx, text, pageID) {
		if bedient[id] {
			continue
		}
		bedient[id] = true
		s.zustellen(ctx, id, PostErwaehnt, pageID, kommentarID, uid, name, titel, text)
	}

	// A reply: the author of the comment being answered.
	if elternID != nil {
		var autor *string
		if err := s.Pool.QueryRow(ctx,
			`SELECT autor_id::text FROM kommentare WHERE id=$1`, *elternID).Scan(&autor); err == nil &&
			autor != nil && !bedient[*autor] {
			bedient[*autor] = true
			s.zustellen(ctx, *autor, PostAntwort, pageID, kommentarID, uid, name, titel, text)
		}
	}

	// And the owner of the page, the person a question is aimed at.
	var eigner string
	if err := s.Pool.QueryRow(ctx,
		`SELECT owner_id::text FROM pages WHERE id=$1`, pageID).Scan(&eigner); err == nil && !bedient[eigner] {
		s.zustellen(ctx, eigner, PostKommentar, pageID, kommentarID, uid, name, titel, text)
	}
}

// darfAendern resolves whether the caller may touch one comment: its author,
// the page owner, or an admin.
func (s *Server) darfAendern(r *http.Request, kommentarID string) (seiteID string, ok bool) {
	uid := middleware.UserID(r)
	var autor *string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT page_id::text, autor_id::text FROM kommentare WHERE id=$1 AND geloescht_am IS NULL`,
		kommentarID).Scan(&seiteID, &autor); err != nil {
		return "", false
	}
	if autor != nil && *autor == uid {
		return seiteID, true
	}
	_, _, isOwner, vorhanden := s.pagePerm(r.Context(), uid, seiteID)
	if vorhanden && isOwner {
		return seiteID, true
	}
	return seiteID, s.isAdmin(r.Context(), uid)
}

// UpdateKommentar rewrites the text.
func (s *Server) UpdateKommentar(w http.ResponseWriter, r *http.Request) {
	kid := chi.URLParam(r, "kommentarId")
	seite, ok := s.darfAendern(r, kid)
	if !ok {
		writeErr(w, http.StatusForbidden, "nicht erlaubt")
		return
	}

	var req kommentarReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" || len(req.Text) > maxKommentarLaenge {
		writeErr(w, http.StatusBadRequest, "Kommentar ist leer oder zu lang")
		return
	}

	_, err := s.Pool.Exec(r.Context(),
		`UPDATE kommentare SET text=$2, geaendert_am=now() WHERE id=$1`, kid, req.Text)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Kommentar konnte nicht geändert werden")
		return
	}
	s.spurAusRequest(r, AktKommentarGeaendert, "seite", seite, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DeleteKommentar empties the text and marks the row deleted. The row itself
// stays so replies hanging off it keep their place in the thread.
func (s *Server) DeleteKommentar(w http.ResponseWriter, r *http.Request) {
	kid := chi.URLParam(r, "kommentarId")
	seite, ok := s.darfAendern(r, kid)
	if !ok {
		writeErr(w, http.StatusForbidden, "nicht erlaubt")
		return
	}
	_, err := s.Pool.Exec(r.Context(),
		`UPDATE kommentare SET text='', geloescht_am=now() WHERE id=$1`, kid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Kommentar konnte nicht gelöscht werden")
		return
	}
	s.spurAusRequest(r, AktKommentarGeloescht, "seite", seite, "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ToggleErledigt marks a thread as settled or opens it again. Only the top of a
// thread carries the flag; a reply cannot be "done" on its own.
func (s *Server) ToggleErledigt(w http.ResponseWriter, r *http.Request) {
	kid := chi.URLParam(r, "kommentarId")
	seite, ok := s.darfAendern(r, kid)
	if !ok {
		writeErr(w, http.StatusForbidden, "nicht erlaubt")
		return
	}
	var erledigt bool
	err := s.Pool.QueryRow(r.Context(),
		`UPDATE kommentare SET erledigt = NOT erledigt
		 WHERE id=$1 AND eltern_id IS NULL
		 RETURNING erledigt`, kid).Scan(&erledigt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "nur ein Faden lässt sich erledigen")
		return
	}
	s.spurAusRequest(r, AktKommentarErledigt, "seite", seite, "",
		map[string]interface{}{"erledigt": erledigt})
	writeJSON(w, http.StatusOK, map[string]bool{"erledigt": erledigt})
}
