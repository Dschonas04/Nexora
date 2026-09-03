// Exporting a whole space as a ZIP of Markdown files.
//
// The export's goal is portability rather than convenience: it answers
// "what happens if we stop using this". A wiki nobody can leave is a wiki
// nobody should adopt.
//
// The archive is streamed directly to the response instead of built in memory
// first. A space with hundreds of pages and attachments would otherwise use
// a large amount of RAM twice (buffer + response).
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
// spaceID may be "ohne": this collects pages that do not belong to any
// space. Otherwise those pages would be excluded from every export even though
// they are part of the content.
func (s *Server) ExportSpace(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	spaceID := chi.URLParam(r, "id")
	admin := s.isAdmin(r.Context(), uid)

	var spaceName string
	if spaceID == "ohne" {
		// The UI uses the term "Ablage" rather than "Space", and the name is
		// used in the archive filename.
		spaceName = "Ohne Ablage"
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

	// The same visibility rules apply as elsewhere. Allowing an export to
	// reach further than opening a single page would be an easy way to leak
	// other people's content.
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

	// Optionally render the whole space as a single typeset document (PDF or
	// Word) rather than a folder of individual files. That is useful for
	// printing or quick reading. The Markdown archive remains the default as
	// it is the machine-readable escape route.
	switch r.URL.Query().Get("format") {
	case "pdf", "word", "docx":
		sort.Slice(seiten, func(i, j int) bool { return seiten[i].Titel < seiten[j].Titel })
		docs := make([]dok.Dokument, 0, len(seiten))
		// A single image source for the whole export: it caches what it has
		// already read so that a repeated logo is fetched only once per export.
		bilder := s.bildquelle(r.Context(), uid)
		for _, p := range seiten {
			docs = append(docs, dok.AusInhaltMitBildern(p.Inhalt, p.Titel, bilder))
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

	// Duplicate titles are allowed but duplicate file names are not. A counter
	// is appended to create unique filenames rather than silently overwriting.
	vergeben := map[string]int{}
	eindeutig := func(basis string) string {
		vergeben[basis]++
		if n := vergeben[basis]; n > 1 {
			return fmt.Sprintf("%s-%d", basis, n)
		}
		return basis
	}

	// Collect the attachments for all exported pages first because the page
	// text refers to paths in the archive and needs them available.
	kennungen := make([]string, 0, len(seiten))
	for _, p := range seiten {
		kennungen = append(kennungen, p.ID)
	}
	dateien := s.anhaengeSammeln(r.Context(), kennungen)
	proSeite := map[string][]exportDatei{}
	for _, d := range dateien {
		proSeite[d.Seite] = append(proSeite[d.Seite], d)
	}

	var verzeichnis strings.Builder
	verzeichnis.WriteString("# " + spaceName + "\n\n")
	verzeichnis.WriteString(fmt.Sprintf("%d Seiten, ausgegeben am %s.\n\n",
		len(seiten), time.Now().Format("02.01.2006 15:04")))
	if len(dateien) > 0 {
		verzeichnis.WriteString(fmt.Sprintf("Die Bilder und Anhänge liegen in %s/ und sind aus den Seiten heraus verknüpft.\n\n",
			dateiOrdner))
	}

	sort.Slice(seiten, func(i, j int) bool { return seiten[i].Titel < seiten[j].Titel })

	// Filenames are determined before the index because the index models the
	// page tree and needs the name of a page even if the page itself is written
	// later in the archive.
	namen := map[string]string{}
	for _, p := range seiten {
		namen[p.ID] = eindeutig(dateiname(p.Titel)) + ".md"
	}

	for _, p := range seiten {
		kopf, err := zw.CreateHeader(&zip.FileHeader{
			Name:     namen[p.ID],
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
		md = adressenAufDateien(md, proSeite[p.ID])
		md = anhangListe(md, proSeite[p.ID])
		if _, err := io.WriteString(kopf, md); err != nil {
			return
		}
	}

	// The index models the page tree rather than a flat list: previously a
	// deep space appeared as a flat enumeration and readers could not tell the
	// hierarchy.
	schreibeVerzeichnis(&verzeichnis, seiten, namen)

	s.dateienSchreiben(r.Context(), zw, dateien)

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
		map[string]any{"seiten": len(seiten), "dateien": len(dateien)})
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

// schreibeVerzeichnis renders the page tree as an indented list.
//
// The order is derived from parent_id, not from the query order: a child
// page may appear alphabetically before its parent. Pages whose parent was
// not included — because it belongs to another space or is restricted — are
// shown at the top instead of being omitted.
func schreibeVerzeichnis(b *strings.Builder, seiten []exportSeite, namen map[string]string) {
	kinder := map[string][]exportSeite{}
	dabei := map[string]bool{}
	for _, p := range seiten {
		dabei[p.ID] = true
	}
	for _, p := range seiten {
		eltern := ""
		if p.ParentID != nil && dabei[*p.ParentID] {
			eltern = *p.ParentID
		}
		kinder[eltern] = append(kinder[eltern], p)
	}

	var stufe func(eltern string, tiefe int)
	stufe = func(eltern string, tiefe int) {
		for _, p := range kinder[eltern] {
			titel := p.Titel
			if strings.TrimSpace(titel) == "" {
				titel = "Ohne Titel"
			}
			// Spitze Klammern um das Ziel: ein Dateiname mit Leerzeichen wuerde
			// den Verweis sonst nach dem ersten Wort beenden. Prozentzeichen
			// taeten es auch, sind aber unlesbar, und diese Datei will gelesen
			// werden.
			fmt.Fprintf(b, "%s- [%s](<%s>)\n", strings.Repeat("  ", tiefe), titel, namen[p.ID])
			stufe(p.ID, tiefe+1)
		}
	}
	stufe("", 0)
}
