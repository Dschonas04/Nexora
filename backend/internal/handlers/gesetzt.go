// Export als gesetztes Dokument: PDF und Word.
//
// Markdown bleibt frei, den eigenen Bestand aus dem System zu bekommen darf
// nie hinter einer Lizenz liegen, das ist der Kern des Versprechens, das eine
// BSL macht. Gesetzte Dokumente sind etwas anderes: sie sind kein Ausweg,
// sondern eine Darstellung, und die gehört zum Zusatzumfang.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"nexora/internal/dok"
	"nexora/internal/middleware"
)

// seiteAlsDokument holt eine Seite und bringt sie in die Formatiererform.
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
	return dok.AusInhalt(json.RawMessage(inhalt), titel), true
}

// dateiKopf setzt Typ und Dateiname. Zwei Angaben mit Absicht: filename für
// Klienten, die nur ASCII verstehen, filename* nach RFC 5987 für alle anderen
// damit "Übersicht" eine Übersicht bleibt.
func dateiKopf(w http.ResponseWriter, typ, name, endung string) {
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+nurASCII(name)+endung+`"; filename*=UTF-8''`+
			url.PathEscape(name+endung))
}

// ExportPDF liefert eine Seite als PDF.
func (s *Server) ExportPDF(w http.ResponseWriter, r *http.Request) {
	d, ok := s.seiteAlsDokument(r, chi.URLParam(r, "id"))
	if !ok {
		writeErr(w, http.StatusForbidden, "kein Zugriff auf diese Seite")
		return
	}
	dateiKopf(w, "application/pdf", dateiname(d.Titel), ".pdf")
	w.Write(dok.PDF(d))
}

// ExportWord liefert eine Seite als Word-Dokument.
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
