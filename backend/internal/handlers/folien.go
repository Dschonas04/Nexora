// Reading presentation attachments (.pptx) for the viewer. Read only, like
// spreadsheets: the server returns the slides as positioned boxes and the
// viewer draws them.
package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/dok"
	"nexora/internal/middleware"
)

func istFolien(mime, name string) bool {
	if strings.EqualFold(mime, "application/vnd.openxmlformats-officedocument.presentationml.presentation") {
		return true
	}
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".pptx") || strings.HasSuffix(n, ".ppsx")
}

// FolienLesen returns a presentation attachment as slides.
func (s *Server) FolienLesen(w http.ResponseWriter, r *http.Request) {
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
	if !istFolien(mime, name) {
		writeErr(w, http.StatusBadRequest, "das ist keine PowerPoint-Datei")
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
	p, err := dok.AusFolien(roh)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}
