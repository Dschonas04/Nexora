// File attachments. Metadata lives in the database, the bytes live in the
// configured Ablage -- local disk or an S3 bucket -- as one object per
// attachment id. That split means a backup has to cover both, otherwise rows
// point at objects that are gone.
package handlers

import (
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
	"nexora/internal/models"
)

// Vorgabe, falls die Einstellung fehlt. Die tatsächliche Grenze kommt aus
// MaxAnhangBytes und lässt sich im Betrieb ändern -- der Wert muss aber zur
// client_max_body_size im nginx davor passen, sonst bricht der schon vorher ab.
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

	// Ab hier entscheidet die Ablage, wo die Bytes landen -- Platte oder
	// Objektspeicher. Der Handler sieht keinen Unterschied.
	//
	// Der Mitschnitt hängt sich in den Strom: die Datei läuft ohnehin durch,
	// und sie zum Auslesen ein zweites Mal zu holen wäre eine vermeidbare
	// Runde -- beim Objektspeicher sogar übers Netz.
	strom := &mitschnitt{quelle: file}
	written, err := s.Ablage.Schreiben(r.Context(), attID, strom, header.Size, mime)
	if err != nil {
		// Die Zeile wieder wegnehmen: eine Anhangzeile ohne Datei wäre ein
		// Eintrag, der sich anklicken lässt und dann ins Leere führt.
		s.Pool.Exec(r.Context(), `DELETE FROM attachments WHERE id=$1`, attID)
		writeErr(w, http.StatusInternalServerError, "Ablage nicht erreichbar")
		return
	}
	// Volltext nachtragen. Erst jetzt, nach dem erfolgreichen Schreiben: ein
	// Anhang ohne Suchtext ist brauchbar, ein Suchtext ohne Anhang nicht.
	//
	// Fehler werden geschluckt. Der Upload ist gelungen; dass die Datei nicht
	// durchsuchbar wird, ist ein Verlust an Komfort, kein Grund zu scheitern.
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
	// Erst die Datei, dann die Zeile. Andersherum bliebe bei einem Fehler eine
	// Datei liegen, von der niemand mehr weiß.
	_ = s.Ablage.Loeschen(r.Context(), attID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
