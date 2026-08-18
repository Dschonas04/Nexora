// File attachments. Metadata lives in the database, the bytes live on disk in
// DataDir, one file per attachment id. That split means a backup has to cover
// both, otherwise rows point at files that are gone.
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

// ListAttachments returns the metadata of the files on a page. Read access to
// the page is enough, matching what a reader sees in the editor.
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

// UploadAttachment stores an uploaded file. The row is inserted first to obtain
// an id, which then names the file on disk; every failure after that point rolls
// the row back so no orphan metadata survives.
//
// The file is named by its id, never by the name the client sent. That keeps a
// crafted filename such as ../../etc/passwd from escaping DataDir.
func (s *Server) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, canEdit, _, ok := s.pagePerm(r.Context(), uid, id); !ok || !canEdit {
		writeErr(w, http.StatusForbidden, "no edit access")
		return
	}

	// Cap the body before parsing it, otherwise a large upload would be buffered
	// to temporary storage before anyone checks its size.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	// Keep the base name for display only. Any directory part is dropped here,
	// and the stored file is named by its id anyway.
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
	// header.Size comes from the client and can be wrong or absent; persist the
	// number of bytes actually written instead.
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2 WHERE id=$1`, attID, written)

	writeJSON(w, http.StatusCreated, models.Attachment{
		ID: attID, PageID: id, Filename: filename, Mime: mime, Size: written,
	})
}

// DownloadAttachment streams a file back. Access is decided by the page, not by
// the attachment, so revoking a share also cuts off its files.
//
// Content-Disposition is inline so images and PDFs preview in the browser. The
// mime type is the one the uploader claimed, which is why a page should only be
// shared with people who are trusted not to upload hostile content.
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

// DeleteAttachment removes the row and then the file. In that order, because a
// leftover file on disk is harmless while a row without a file shows up in the
// UI as a download that always fails.
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
