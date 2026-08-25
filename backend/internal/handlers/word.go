// Word-Anhänge im Browser lesen und schreiben.
//
// Zwei Aufrufe: einer liefert die Datei als Editorblöcke, der andere nimmt
// Blöcke entgegen und legt daraus wieder eine .docx an derselben Stelle ab.
//
// Was dabei verloren geht, steht in internal/dok/word_lesen.go und wird der
// Nutzerin vorher gesagt. Kurz: Inhalt bleibt, Aufmachung nicht.
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

// wordTypen sind die Kennungen, unter denen eine .docx ankommen kann.
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

// WordLesen gibt einen Word-Anhang als Editorblöcke zurück.
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

// WordSchreiben legt die geänderten Blöcke wieder als .docx ab.
func (s *Server) WordSchreiben(w http.ResponseWriter, r *http.Request) {
	// Schreiben in eine Datei ist Bearbeiten eines Anhangs, dieselbe Lizenz
	// wie das Hochladen.
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

	// Unter derselben Kennung ablegen: der Anhang behält seine Adresse, alle
	// Verweise darauf bleiben gültig. Erst schreiben, dann die Größe
	// nachführen, andersherum stünde in der Zeile eine Zahl, die nicht zur
	// Datei gehört.
	if _, err := s.Ablage.Schreiben(r.Context(), attID, bytes.NewReader(roh),
		int64(len(roh)), mime); err != nil {
		writeErr(w, http.StatusInternalServerError, "Ablage nicht erreichbar")
		return
	}
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2 WHERE id=$1`, attID, len(roh))

	// Der Volltext wird nachgezogen, sonst fände die Suche noch den alten
	// Stand, und das ist die Art Fehler, die man erst Wochen später bemerkt.
	if lizenz.Frei(lizenz.Anhangsuche) {
		if txt := textAusAnhang(r.Context(), roh, mime, name); txt != "" {
			s.Pool.Exec(r.Context(),
				`UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, attID, txt)
		}
	}

	// Eigene Aktion statt "hochgeladen": eine Datei zu ersetzen ist etwas
	// anderes als eine hinzuzufügen, und in der Spur muss man das
	// unterscheiden können.
	s.spurAusRequest(r, AktAnhangBearbeitet, "anhang", attID, name,
		map[string]any{"bytes": len(roh)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(roh)})
}
