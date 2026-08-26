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

// ExportSpace returns a space as a ZIP.
//
// spaceID may be "ohne": then the pages belonging to no space are collected.
// Otherwise they would be left out of every export even though they are just as
// much part of the content, and nobody would notice.
func (s *Server) ExportSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	spaceID := chi.URLParam(r, "id")
	admin := s.isAdmin(r.Context(), uid)

	var spaceName string
	if spaceID == "ohne" {
		spaceName = "Ohne Space"
	} else {
		// An admin may read every page, so they must also be allowed to export
		// the space it sits in. The rule would otherwise contradict itself: the
		// contents would be reachable one by one but not as a bundle.
		//
		// Which pages actually end up in the archive is decided by the query
		// below, not by this line.
		if err := s.Pool.QueryRow(r.Context(),
			`SELECT name FROM spaces WHERE id=$1 AND (owner_id=$2 OR $3)`,
			spaceID, uid, admin).Scan(&spaceName); err != nil {
			writeErr(w, http.StatusNotFound, "Space nicht gefunden")
			return
		}
	}

	// Filtered by the same rule as everywhere else. An export reaching further
	// than opening a page would be the most convenient way to other people's
	// content.
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

	// A space as ONE typeset document rather than an archive full of separate
	// parts: for leafing through, printing and passing on that is the more useful
	// form. The archive of Markdown files stays the default, because it is the
	// way out of the system and that should be machine readable.
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
	// No defer for Close: a failure while finishing has to be loggable, and once
	// the header is on the wire no error status can be sent anyway.

	// Duplicate titles are allowed, duplicate file names are not. The counter
	// appends a number instead of silently overwriting the second page.
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

		// Angle brackets around the target: a file name containing spaces would
		// otherwise end the link after the first word. Percent encoding would work
		// too but is unreadable, and this file is meant to be read.
		verzeichnis.WriteString(fmt.Sprintf("- [%s](<%s>)\n", p.Titel, datei))
	}

	// A table of contents on top. Without it an archive of a hundred files is a
	// pile, not a document.
	// With a header of its own rather than zw.Create: without Modified the entry
	// carries the year 1980, and a date from before the file existed looks like a
	// broken archive.
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

// spaceBedingung returns the matching WHERE clause. Kept separate because
// "without a space" needs an IS NULL and no parameter. Building the condition
// inside the string is harmless here, since it comes from a fixed comparison and
// not from any input.
func spaceBedingung(spaceID string) string {
	if spaceID == "ohne" {
		return "p.space_id IS NULL"
	}
	return "p.space_id = $3"
}
