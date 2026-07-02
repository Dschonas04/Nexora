package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

const maxUploadBytes = 25 << 20 // 25 MiB, mirrors the nginx client_max_body_size

// ListAttachments returns the files attached to a page.
func (s *Server) ListAttachments(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT id, page_id, filename, mime, size, created_at FROM attachments
		 WHERE page_id=$1 ORDER BY created_at DESC`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	list := []models.Attachment{}
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(&a.ID, &a.PageID, &a.Filename, &a.Mime, &a.Size, &a.CreatedAt); err == nil {
			list = append(list, a)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// UploadAttachment stores an uploaded file on disk and records its metadata.
func (s *Server) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, canEdit, _, ok := s.pagePerm(r.Context(), uid, id); !ok || !canEdit {
		writeErr(w, http.StatusForbidden, "no edit access")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = "file"
	}
	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	var attID string
	if err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO attachments (page_id, owner_id, filename, mime, size)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		id, uid, filename, mime, header.Size).Scan(&attID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save attachment")
		return
	}

	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		s.Pool.Exec(r.Context(), `DELETE FROM attachments WHERE id=$1`, attID)
		writeErr(w, http.StatusInternalServerError, "storage unavailable")
		return
	}
	dst, err := os.Create(filepath.Join(s.DataDir, attID))
	if err != nil {
		s.Pool.Exec(r.Context(), `DELETE FROM attachments WHERE id=$1`, attID)
		writeErr(w, http.StatusInternalServerError, "storage unavailable")
		return
	}
	written, err := io.Copy(dst, file)
	dst.Close()
	if err != nil {
		os.Remove(filepath.Join(s.DataDir, attID))
		s.Pool.Exec(r.Context(), `DELETE FROM attachments WHERE id=$1`, attID)
		writeErr(w, http.StatusInternalServerError, "write failed")
		return
	}
	// header.Size can be unreliable; persist the real byte count.
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2 WHERE id=$1`, attID, written)

	writeJSON(w, http.StatusCreated, models.Attachment{
		ID: attID, PageID: id, Filename: filename, Mime: mime, Size: written,
	})
}

// DownloadAttachment streams a stored file back to the client.
func (s *Server) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	attID := chi.URLParam(r, "attId")
	var filename, mime string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT filename, mime FROM attachments WHERE id=$1 AND page_id=$2`, attID, id).Scan(&filename, &mime); err != nil {
		writeErr(w, http.StatusNotFound, "attachment not found")
		return
	}
	f, err := os.Open(filepath.Join(s.DataDir, attID))
	if err != nil {
		writeErr(w, http.StatusNotFound, "file missing")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", "inline; filename=\""+strings.ReplaceAll(filename, "\"", "")+"\"")
	io.Copy(w, f)
}

// DeleteAttachment removes an attachment (metadata + file on disk).
func (s *Server) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, canEdit, _, ok := s.pagePerm(r.Context(), uid, id); !ok || !canEdit {
		writeErr(w, http.StatusForbidden, "no edit access")
		return
	}
	attID := chi.URLParam(r, "attId")
	tag, err := s.Pool.Exec(r.Context(),
		`DELETE FROM attachments WHERE id=$1 AND page_id=$2`, attID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "attachment not found")
		return
	}
	os.Remove(filepath.Join(s.DataDir, attID))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
