// Reading and writing Word attachments in the browser.
//
// Two endpoints: one returns the file as editor blocks, the other takes blocks
// and writes a .docx back to the same place.
//
// What is lost on the way is documented in internal/dok/word_lesen.go, and the
// interface says so before anyone starts editing. In short: the content
// survives, the styling does not.
package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/dok"
	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// wordTypen are the media types a .docx can arrive under.
var wordTypen = []string{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}

func istWord(mime, name string) bool {
	for _, t := range wordTypen {
		if strings.EqualFold(mime, t) {
			return true
		}
	}
	return strings.EqualFold(strings.TrimPrefix(strings.ToLower(name[strings.LastIndex(name, ".")+1:]), "."), "docx")
}

// WordLesen returns a Word attachment as editor blocks.
func (s *Server) WordLesen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	attID := chi.URLParam(r, "attId")

	var name, mime string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT filename, mime FROM attachments WHERE id=$1 AND page_id=$2`, attID, id).
		Scan(&name, &mime); err != nil {
		writeErr(w, http.StatusNotFound, "attachment not found")
		return
	}
	if !istWord(mime, name) {
		writeErr(w, http.StatusBadRequest, "das ist keine Word-Datei")
		return
	}

	f, err := s.Ablage.Lesen(r.Context(), attID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "file missing")
		return
	}
	defer f.Close()
	roh, err := io.ReadAll(io.LimitReader(f, MaxAnhangBytes()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Datei nicht lesbar")
		return
	}

	d, err := dok.AusWord(roh)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	titel := d.Titel
	if titel == "" {
		titel = strings.TrimSuffix(name, ".docx")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"titel":   titel,
		"bloecke": dok.NachBloecken(d),
	})
}

type wordSchreibenReq struct {
	Titel   string          `json:"titel"`
	Bloecke json.RawMessage `json:"bloecke"`
}

// WordSchreiben stores the edited blocks as a .docx again.
func (s *Server) WordSchreiben(w http.ResponseWriter, r *http.Request) {
	// Writing into a file is editing an attachment, so the same license applies
	// as for uploading one.
	if !lizenz.Frei(lizenz.Anhaenge) {
		writeErr(w, http.StatusPaymentRequired, "Anhänge gehören zum Zusatzumfang")
		return
	}
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, darf, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !darf {
		writeErr(w, http.StatusForbidden, "read-only access")
		return
	}
	attID := chi.URLParam(r, "attId")

	var name, mime string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT filename, mime FROM attachments WHERE id=$1 AND page_id=$2`, attID, id).
		Scan(&name, &mime); err != nil {
		writeErr(w, http.StatusNotFound, "attachment not found")
		return
	}
	if !istWord(mime, name) {
		writeErr(w, http.StatusBadRequest, "das ist keine Word-Datei")
		return
	}

	var req wordSchreibenReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	roh, err := dok.Word(dok.AusInhalt(req.Bloecke, req.Titel))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Dokument konnte nicht erzeugt werden")
		return
	}

	// Stored under the same id: the attachment keeps its address and every link
	// to it stays valid. Write first, then update the size; the other way round
	// the row would carry a number that does not belong to the file.
	if _, err := s.Ablage.Schreiben(r.Context(), attID, bytes.NewReader(roh),
		int64(len(roh)), mime); err != nil {
		writeErr(w, http.StatusInternalServerError, "Ablage nicht erreichbar")
		return
	}
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2 WHERE id=$1`, attID, len(roh))

	// The full text is refreshed, otherwise search would still find the old
	// version, and that is the kind of bug noticed weeks later.
	if lizenz.Frei(lizenz.Anhangsuche) {
		if txt := textAusAnhang(r.Context(), roh, mime, name); txt != "" {
			s.Pool.Exec(r.Context(),
				`UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, attID, txt)
		}
	}

	// Its own audit action rather than "uploaded": replacing a file is a
	// different act from adding one, and the trail has to tell them apart.
	s.spurAusRequest(r, AktAnhangBearbeitet, "anhang", attID, name,
		map[string]any{"bytes": len(roh)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(roh)})
}
