// Exporting a whole space as a ZIP of Markdown files.
//
// The point of an export is not convenience, it is the answer to "what happens
// if we stop using this". A wiki nobody can leave is a wiki nobody should
// enter, and that question comes up in every serious evaluation.
//
// The archive is written straight to the response instead of being assembled in
// memory first. A space with a few hundred pages and their attachments would
// otherwise sit in RAM twice, once as the buffer, once as the response.
package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/dok"
	"nexora/internal/middleware"
)

type exportSeite struct {
	ID        string
	ParentID  *string
	Titel     string
	Inhalt    json.RawMessage
	Geaendert time.Time
}

// ExportSpace liefert einen Space als ZIP.
//
// spaceID darf "ohne" sein: dann kommen die Seiten, die keinem Space angehören.
// Sonst blieben sie beim Export außen vor, obwohl sie genauso zum Bestand
// gehören, und niemand würde es merken.
func (s *Server) ExportSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	spaceID := chi.URLParam(r, "id")
	admin := s.isAdmin(r.Context(), uid)

	var spaceName string
	if spaceID == "ohne" {
		spaceName = "Ohne Space"
	} else {
		// Ein Admin darf jede Seite lesen, dann muss er auch den Space
		// exportieren dürfen, in dem sie liegt. Sonst widerspräche sich die
		// Regel: die Inhalte wären einzeln zugänglich, gebündelt aber nicht.
		//
		// Welche Seiten am Ende im Archiv landen, entscheidet ohnehin die
		// Abfrage weiter unten, nicht diese Zeile.
		if err := s.Pool.QueryRow(r.Context(),
			`SELECT name FROM spaces WHERE id=$1 AND (owner_id=$2 OR $3)`,
			spaceID, uid, admin).Scan(&spaceName); err != nil {
			writeErr(w, http.StatusNotFound, "Space nicht gefunden")
			return
		}
	}

	// Gefiltert wird nach derselben Regel wie überall. Ein Export, der weiter
	// reicht als das Öffnen einer Seite, wäre der bequemste Weg, an fremde
	// Inhalte zu kommen.
	abfrage := `
		SELECT p.id, p.parent_id, p.title, p.content, p.updated_at
		FROM pages p
		WHERE p.deleted_at IS NULL
		  AND ` + spaceBedingung(spaceID) + `
		  AND (p.owner_id = $1 OR $2
		       OR EXISTS (SELECT 1 FROM page_shares sh
		                  WHERE sh.page_id = p.id AND sh.user_id = $1))
		ORDER BY p.title`

	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
	}
	var err error
	if spaceID == "ohne" {
		rows, err = s.Pool.Query(r.Context(), abfrage, uid, admin)
	} else {
		rows, err = s.Pool.Query(r.Context(), abfrage, uid, admin, spaceID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var seiten []exportSeite
	for rows.Next() {
		var p exportSeite
		var inhalt []byte
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Titel, &inhalt, &p.Geaendert); err == nil {
			p.Inhalt = json.RawMessage(inhalt)
			seiten = append(seiten, p)
		}
	}

	if len(seiten) == 0 {
		writeErr(w, http.StatusNotFound, "keine Seiten in diesem Space")
		return
	}

	name := dateiname(spaceName)

	// Ein Space als EIN gesetztes Dokument statt als Archiv voller Einzelteile:
	// zum Durchblättern, Drucken und Weiterreichen ist das die brauchbarere
	// Form. Das Archiv aus Markdown-Dateien bleibt die Vorgabe, es ist der
	// Ausweg aus dem System, und der soll maschinenlesbar sein.
	switch r.URL.Query().Get("format") {
	case "pdf", "word", "docx":
		sort.Slice(seiten, func(i, j int) bool { return seiten[i].Titel < seiten[j].Titel })
		docs := make([]dok.Dokument, 0, len(seiten))
		for _, p := range seiten {
			docs = append(docs, dok.AusInhalt(p.Inhalt, p.Titel))
		}
		if r.URL.Query().Get("format") == "pdf" {
			dateiKopf(w, "application/pdf", name, ".pdf")
			w.Write(dok.PDFMehrere(docs, spaceName))
		} else {
			roh, err := dok.WordMehrere(docs)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "Dokument konnte nicht erzeugt werden")
				return
			}
			dateiKopf(w,
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				name, ".docx")
			w.Write(roh)
		}
		s.spurAusRequest(r, AktExport, "space", spaceID, spaceName,
			map[string]interface{}{"seiten": len(seiten), "format": r.URL.Query().Get("format")})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+nurASCII(name)+`.zip"; filename*=UTF-8''`+
			url.PathEscape(name+".zip"))

	zw := zip.NewWriter(w)
	// Kein defer für Close: ein Fehler beim Abschließen muss protokolliert
	// werden können, und nach dem Schreiben des Kopfes lässt sich ohnehin kein
	// Fehlerstatus mehr senden.

	// Doppelte Titel sind erlaubt, doppelte Dateinamen nicht. Der Zähler hängt
	// eine Nummer an, statt die zweite Seite stillschweigend zu überschreiben.
	vergeben := map[string]int{}
	eindeutig := func(basis string) string {
		vergeben[basis]++
		if n := vergeben[basis]; n > 1 {
			return fmt.Sprintf("%s-%d", basis, n)
		}
		return basis
	}

	var verzeichnis strings.Builder
	verzeichnis.WriteString("# " + spaceName + "\n\n")
	verzeichnis.WriteString(fmt.Sprintf("%d Seiten, ausgegeben am %s.\n\n",
		len(seiten), time.Now().Format("02.01.2006 15:04")))

	sort.Slice(seiten, func(i, j int) bool { return seiten[i].Titel < seiten[j].Titel })

	for _, p := range seiten {
		datei := eindeutig(dateiname(p.Titel)) + ".md"

		kopf, err := zw.CreateHeader(&zip.FileHeader{
			Name:     datei,
			Method:   zip.Deflate,
			Modified: p.Geaendert,
		})
		if err != nil {
			return
		}

		md := MarkdownAusInhalt(p.Inhalt)
		if p.Titel != "" && !beginntMitUeberschrift(md, p.Titel) {
			md = "# " + p.Titel + "\n\n" + md
		}
		if _, err := io.WriteString(kopf, md); err != nil {
			return
		}

		// Spitze Klammern um das Ziel: ein Dateiname mit Leerzeichen bricht
		// den Verweis sonst nach dem ersten Wort ab. Prozentkodierung täte es
		// auch, wäre aber unlesbar, und diese Datei soll man lesen können.
		verzeichnis.WriteString(fmt.Sprintf("- [%s](<%s>)\n", p.Titel, datei))
	}

	// Ein Inhaltsverzeichnis obendrauf. Ohne das ist ein Archiv mit hundert
	// Dateien eine Ablage, kein Dokument.
	// Mit eigenem Kopf statt zw.Create: ohne Modified trägt der Eintrag im
	// Archiv das Jahr 1980, und ein Datum aus der Zeit vor der Datei sieht nach
	// einem kaputten Archiv aus.
	if kopf, err := zw.CreateHeader(&zip.FileHeader{
		Name:     "INHALT.md",
		Method:   zip.Deflate,
		Modified: time.Now(),
	}); err == nil {
		io.WriteString(kopf, verzeichnis.String())
	}

	zw.Close()
	s.spurAusRequest(r, AktExport, "space", spaceID, spaceName,
		map[string]any{"seiten": len(seiten)})
}

// spaceBedingung liefert die passende WHERE-Zeile. Getrennt, weil "ohne Space"
// ein IS NULL braucht und keinen Parameter, die Bedingung im String zu bauen
// ist hier ungefährlich, weil sie aus einem festen Vergleich stammt und nicht
// aus einer Eingabe.
func spaceBedingung(spaceID string) string {
	if spaceID == "ohne" {
		return "p.space_id IS NULL"
	}
	return "p.space_id = $3"
}
