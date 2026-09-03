// Eine markierte PDF-Datei an die Stelle der alten schreiben.
//
// Markiert wird im Browser: dort liegt die Seite vor Augen, dort wird gezogen,
// und dort steht auch die Bibliothek, die eine PDF-Datei ergänzen kann. Dieser
// Weg nimmt das Ergebnis entgegen und legt es unter derselben Kennung ab wie
// die Vorlage.
//
// Ersetzt und nicht danebengelegt, so gewollt: eine markierte Fassung ist nicht
// ein zweites Dokument, sondern dasselbe Dokument, nachdem jemand etwas
// angestrichen hat. Der Anhang behält damit seine Adresse, und jeder Verweis
// darauf -- aus dem Text, aus einem Kommentar, aus einem Lesezeichen -- zeigt
// weiterhin auf das, was gemeint war.
//
// Was das kostet, steht in der Prüfspur: die alte Fassung ist danach fort. Wer
// sie behalten will, lädt sie vorher herunter; ein Versionsverlauf für Anhänge
// wäre etwas anderes, und diesen Weg hier hat der Betreiber ausdrücklich so
// bestellt.
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

// pdfMagie steht am Anfang jeder PDF-Datei. Geprüft wird sie, weil hier Bytes
// hereinkommen, die gleich eine vorhandene Datei überschreiben: was das nicht
// ist, darf nicht durch.
var pdfMagie = []byte("%PDF-")

func istPDF(mime, name string) bool {
	if strings.EqualFold(mime, "application/pdf") {
		return true
	}
	// Auch nach dem Namen, denn ein Anhang aus einer Einfuhr trägt manchmal nur
	// application/octet-stream.
	return strings.HasSuffix(strings.ToLower(name), ".pdf")
}

// PDFErsetzen nimmt die markierte Fassung entgegen.
func (s *Server) PDFErsetzen(w http.ResponseWriter, r *http.Request) {
	// Derselbe Zusatz wie beim Lesen und Schreiben von Word-Dateien: es geht um
	// Anhänge.
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

	// Dieselbe Obergrenze wie beim Hochladen. Eine markierte Fassung ist etwas
	// größer als die Vorlage, aber nicht um Größenordnungen.
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

	// Unter derselben Kennung: der Anhang behält seine Adresse, jeder Verweis
	// bleibt gültig. Erst schreiben, dann die Größe nachtragen -- andersherum
	// trüge die Zeile eine Zahl, die nicht zur Datei gehört.
	if _, err := s.Ablage.Schreiben(r.Context(), attID, bytes.NewReader(roh),
		int64(len(roh)), "application/pdf"); err != nil {
		writeErr(w, http.StatusInternalServerError, "Ablage nicht erreichbar")
		return
	}
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2 WHERE id=$1`, attID, len(roh))

	// Der Volltext wird nachgezogen, sonst fände die Suche weiter die alte
	// Fassung -- ein Fehler, den man erst Wochen später bemerkt.
	if lizenz.Frei(lizenz.Anhangsuche) {
		if txt := textAusAnhang(r.Context(), roh, "application/pdf", name); txt != "" {
			s.Pool.Exec(r.Context(),
				`UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, attID, txt)
		}
	}

	// Eigene Aktion und nicht "hochgeladen": eine Datei zu ersetzen ist etwas
	// anderes, als eine hinzuzufügen, und die Prüfspur muss beides
	// auseinanderhalten können.
	s.spurAusRequest(r, AktAnhangBearbeitet, "anhang", attID, name,
		map[string]any{"bytes": len(roh), "art": "pdf-markiert"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(roh)})
}
