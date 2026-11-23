// Reading spreadsheet attachments (.xlsx, .ods) for the viewer.
//
// Read only: the browser cannot render a spreadsheet, so the server returns
// the values per sheet and the viewer draws a table. Writing back is not
// offered; the file stays as it was uploaded.
package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/dok"
	"nexora/internal/middleware"
)

var tabellenTypen = []string{
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"application/vnd.oasis.opendocument.spreadsheet",
}

func istTabelle(mime, name string) bool {
	for _, t := range tabellenTypen {
		if strings.EqualFold(mime, t) {
			return true
		}
	}
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".xlsx") || strings.HasSuffix(n, ".xlsm") || strings.HasSuffix(n, ".ods")
}

// TabelleLesen returns a spreadsheet attachment as sheets of text cells.
func (s *Server) TabelleLesen(w http.ResponseWriter, r *http.Request) {
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
	if !istTabelle(mime, name) {
		writeErr(w, http.StatusBadRequest, "das ist keine Tabellendatei")
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
	blaetter, err := dok.AusTabelle(roh)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sheets": blaetter})
}
