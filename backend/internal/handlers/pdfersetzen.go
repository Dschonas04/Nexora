// Write an annotated PDF in place of the original.
//
// The annotation is created in the browser: the page is displayed there, the
// user marks it, and the browser's library produces a PDF to be uploaded. This
// endpoint accepts that result and stores it under the same attachment ID as
// the original.
//
// It intentionally replaces rather than appends: an annotated version is not
// a second document but the same document after someone highlighted it. The
// attachment therefore keeps its address and any references — from the page
// content, a comment or a bookmark — still point to the intended file.
//
// The cost is recorded in the audit trail: the previous version is lost. If
// someone wishes to keep it they should download it beforehand; a versioning
// system for attachments would be a different feature and this endpoint was
// deliberately implemented without it.
package handlers

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// `pdfMagie` appears at the start of every PDF file. We check it because
// bytes arriving here will immediately replace an existing file: anything
// that is not a PDF must be rejected.
var pdfMagie = []byte("%PDF-")

func istPDF(mime, name string) bool {
	if strings.EqualFold(mime, "application/pdf") {
		return true
	}
	// Also check the filename, because an attachment from an import may be
	// stored as application/octet-stream.
	return strings.HasSuffix(strings.ToLower(name), ".pdf")
}

// `PDFErsetzen` accepts the annotated version.
func (s *Server) PDFErsetzen(w http.ResponseWriter, r *http.Request) {
	// The same add-on applies as for reading and writing Word documents: this
	// concerns attachments.
	if !lizenz.Frei(lizenz.Anhaenge) {
		writeErr(w, http.StatusPaymentRequired, "Anhänge sind ein Zusatz")
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
	if !istPDF(mime, name) {
		writeErr(w, http.StatusBadRequest, "das ist keine PDF-Datei")
		return
	}

	// The same upper limit as for uploads. An annotated version is somewhat
	// larger than the original but not by orders of magnitude.
	r.Body = http.MaxBytesReader(w, r.Body, MaxAnhangBytes())
	roh, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "die Datei ist zu groß")
		return
	}
	if !bytes.HasPrefix(roh, pdfMagie) {
		writeErr(w, http.StatusBadRequest, "das sind keine PDF-Daten")
		return
	}

	// Under the same ID: the attachment keeps its address and references
	// remain valid. Write first, then update the size — doing it the other
	// way would record a size that does not belong to the file.
	if _, err := s.Ablage.Schreiben(r.Context(), attID, bytes.NewReader(roh),
		int64(len(roh)), "application/pdf"); err != nil {
		writeErr(w, http.StatusInternalServerError, "Ablage nicht erreichbar")
		return
	}
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2 WHERE id=$1`, attID, len(roh))

	// Update the full-text index; otherwise searches would still find the
	// old version — a bug that might only be noticed weeks later.
	if lizenz.Frei(lizenz.Anhangsuche) {
		if txt := textAusAnhang(r.Context(), roh, "application/pdf", name); txt != "" {
			s.Pool.Exec(r.Context(),
				`UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, attID, txt)
		}
	}

	// This is an edit action, not an "upload": replacing a file is a
	// different event from adding one, and the audit trail must distinguish
	// the two.
	s.spurAusRequest(r, AktAnhangBearbeitet, "anhang", attID, name,
		map[string]any{"bytes": len(roh), "art": "pdf-markiert"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(roh)})
}
