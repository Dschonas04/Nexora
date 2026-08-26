// File attachments. Metadata lives in the database, the bytes live in the
// configured Ablage, local disk or an S3 bucket, as one object per
// attachment id. That split means a backup has to cover both, otherwise rows
// point at objects that are gone.
package handlers

import (
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

// Default in case the setting is missing. The actual limit comes from
// MaxAnhangBytes and can be changed while running, but the value has to match
// client_max_body_size in the nginx in front, otherwise that one cuts off first.
const maxUploadBytes = 25 << 20

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
// crafted filename such as ../../etc/passwd from escaping the storage directory.
func (s *Server) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if _, canEdit, _, ok := s.pagePerm(r.Context(), uid, id); !ok || !canEdit {
		writeErr(w, http.StatusForbidden, "no edit access")
		return
	}

	// Cap the body before parsing it, otherwise a large upload would be buffered
	// to temporary storage before anyone checks its size.
	r.Body = http.MaxBytesReader(w, r.Body, MaxAnhangBytes())
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
	mime := typAusAngabeUndName(header.Header.Get("Content-Type"), filename)

	var attID string
	if err := s.Pool.QueryRow(r.Context(),
		`INSERT INTO attachments (page_id, owner_id, filename, mime, size)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		id, uid, filename, mime, header.Size).Scan(&attID); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save attachment")
		return
	}

	// From here on the storage decides where the bytes land, disk or object
	// store. The handler sees no difference.
	//
	// The text extraction hooks into the stream: the file passes through anyway,
	// and fetching it a second time to read it would be an avoidable round trip,
	// with an object store even one across the network.
	strom := &mitschnitt{quelle: file}
	written, err := s.Ablage.Schreiben(r.Context(), attID, strom, header.Size, mime)
	if err != nil {
		// Remove the row again: an attachment row without a file would be an entry
		// that can be clicked and then leads nowhere.
		s.Pool.Exec(r.Context(), `DELETE FROM attachments WHERE id=$1`, attID)
		writeErr(w, http.StatusInternalServerError, "Ablage nicht erreichbar")
		return
	}
	// Add the full text afterwards. Only now, after the write succeeded: an
	// attachment without search text is usable, a search text without its
	// attachment is not.
	//
	// Errors are swallowed. The upload succeeded; that the file does not become
	// searchable is a loss of convenience, not a reason to fail.
	if lizenz.Frei(lizenz.Anhangsuche) {
		if txt := textAusAnhang(r.Context(), strom.Bytes(), mime, filename); txt != "" {
			if _, err := s.Pool.Exec(r.Context(),
				`UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, attID, txt); err != nil {
				log.Printf("Anhang-Volltext (%s): %v", attID, err)
			}
		}
	}

	// header.Size comes from the client and can be wrong or absent; persist the
	// number of bytes actually written instead.
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2 WHERE id=$1`, attID, written)

	writeJSON(w, http.StatusCreated, models.Attachment{
		ID: attID, PageID: id, Filename: filename, Mime: mime, Size: written,
	})
}

// typAusAngabeUndName determines the file type.
//
// What the browser sends along on upload is a claim, and for an extension it
// does not know it sends nothing at all or "application/octet-stream". The type
// however decides whether a file later gets a preview and whether its text goes
// into the search. A PDF arriving as octet-stream is a file without properties
// after that.
//
// So when the claim says nothing, the extension is asked. The browser's claim
// takes precedence as long as it says something: it knows cases an extension
// cannot tell apart.
func typAusAngabeUndName(angabe, dateiname string) string {
	angabe = strings.TrimSpace(strings.ToLower(angabe))
	// A parameter such as "; charset=utf-8" does not belong in the column.
	if i := strings.IndexByte(angabe, ';'); i >= 0 {
		angabe = strings.TrimSpace(angabe[:i])
	}
	if angabe != "" && angabe != "application/octet-stream" && angabe != "binary/octet-stream" {
		return angabe
	}
	endung := strings.ToLower(filepath.Ext(dateiname))
	if endung != "" {
		// Our own list first. mime.TypeByExtension also reads the system's type
		// tables, and those differ from machine to machine, so the same file would
		// get two different types on a workstation and in a container. For the
		// extensions something depends on here the type is therefore fixed.
		switch endung {
		case ".md", ".markdown":
			return "text/markdown"
		case ".log", ".conf", ".ini", ".yml", ".yaml", ".toml", ".env":
			return "text/plain"
		}
		// Everything else is left to the table.
		if t := mime.TypeByExtension(endung); t != "" {
			if i := strings.IndexByte(t, ';'); i >= 0 {
				t = strings.TrimSpace(t[:i])
			}
			return t
		}
	}
	if angabe != "" {
		return angabe
	}
	return "application/octet-stream"
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
	f, err := s.Ablage.Lesen(r.Context(), attID)
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
	// The file first, then the row. The other way round a failure would leave a
	// file lying about that nobody knows of any more.
	_ = s.Ablage.Loeschen(r.Context(), attID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
