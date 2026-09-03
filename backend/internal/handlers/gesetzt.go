// Export as a typeset document: PDF and Word.
//
// Markdown stays free; getting one's own content out of the system must never
// sit behind a licence, that is the core of the promise a BSL makes. Typeset
// documents are a different thing: they are not a way out but a presentation,
// and that belongs to the paid extras.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"nexora/internal/dok"
	"nexora/internal/middleware"
)

// seiteAlsDokument fetches a page and brings it into typesetter form.
func (s *Server) seiteAlsDokument(r *http.Request, id string) (dok.Dokument, bool) {
	uid := middleware.UserID(r)
	if canRead, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok || !canRead {
		return dok.Dokument{}, false
	}
	var titel string
	var inhalt []byte
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT title, content FROM pages WHERE id=$1`, id).Scan(&titel, &inhalt); err != nil {
		return dok.Dokument{}, false
	}
	// Mit den Bildern: ein PDF, das die Bilder der Seite weglaesst, ist eine
	// Inhaltsangabe der Seite und nicht ihr Abbild.
	return dok.AusInhaltMitBildern(json.RawMessage(inhalt), titel, s.bildquelle(r.Context(), uid)), true
}

// `dateiKopf` sets the type and the file name. Two entries on purpose:
// `filename` for clients that understand only ASCII, `filename*` per RFC
// 5987 for others — so that "Overview" remains an Overview.
func dateiKopf(w http.ResponseWriter, typ, name, endung string) {
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+nurASCII(name)+endung+`"; filename*=UTF-8''`+
			url.PathEscape(name+endung))
}

// ExportPDF returns a page as a PDF.
func (s *Server) ExportPDF(w http.ResponseWriter, r *http.Request) {
	d, ok := s.seiteAlsDokument(r, chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusForbidden, "kein Zugriff auf diese Seite")
		return
	}
	dateiKopf(w, "application/pdf", dateiname(d.Titel), ".pdf")
	w.Write(dok.PDF(d))
}

// ExportWord returns a page as a Word document.
func (s *Server) ExportWord(w http.ResponseWriter, r *http.Request) {
	d, ok := s.seiteAlsDokument(r, chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusForbidden, "kein Zugriff auf diese Seite")
		return
	}
	roh, err := dok.Word(d)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Dokument konnte nicht erzeugt werden")
		return
	}
	dateiKopf(w,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		dateiname(d.Titel), ".docx")
	w.Write(roh)
}
