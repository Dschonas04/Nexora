// The files that belong to an export.

// An export that returns the images was not the original behaviour. Previously
// the archive contained only Markdown with addresses like
// /api/pages/<id>/attachments/<id>: references that point to nothing outside
// this instance. When someone wanted to move their wiki away they would take
// the text but leave the images behind.

// Therefore files are placed into a folder inside the archive and the
// addresses in the text are rewritten to point there. The archive becomes
// self-contained and readable by any Markdown viewer, even years later.
package handlers

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// dateiOrdner is the folder inside the archive where attachments reside.
const dateiOrdner = "dateien"

// exportDatei describes an attachment as it ends up in the archive.
type exportDatei struct {
	ID     string
	Seite  string
	Name   string // original filename as uploaded
	Pfad   string // where it lives inside the archive, including folder
	Groess int64
}

// anhaengeSammeln reads attachments of the exported pages and assigns a
// unique location for each inside the archive.
//
// Two pages may share the same filename like "Plan.png"; in the archive they
// cannot have the same name. A counter is appended rather than silently
// overwriting the second one — the same rule we apply to pages.
func (s *Server) anhaengeSammeln(ctx context.Context, seiten []string) []exportDatei {
	if len(seiten) == 0 {
		return nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id::text, page_id::text, filename, size FROM attachments
		  WHERE page_id = ANY($1) ORDER BY created_at`, seiten)
	if err != nil {
		return nil
	}
	defer rows.Close()

	vergeben := map[string]int{}
	var liste []exportDatei
	for rows.Next() {
		var d exportDatei
		if rows.Scan(&d.ID, &d.Seite, &d.Name, &d.Groess) != nil {
			continue
		}
		d.Pfad = dateiOrdner + "/" + eindeutigerDateiname(vergeben, d.Name)
		liste = append(liste, d)
	}
	return liste
}

// eindeutigerDateiname appends a number to an already taken name, placed
// BEFORE the extension: "Plan-2.png" not "Plan.png-2", otherwise programs
// would no longer recognise the file type.
func eindeutigerDateiname(vergeben map[string]int, name string) string {
	sauber := dateiname(name)
	if sauber == "seite" && strings.TrimSpace(name) == "" {
		sauber = "datei"
	}
	endung := path.Ext(sauber)
	stamm := strings.TrimSuffix(sauber, endung)

	vergeben[sauber]++
	if n := vergeben[sauber]; n > 1 {
		return fmt.Sprintf("%s-%d%s", stamm, n, endung)
	}
	return sauber
}

// adressenAufDateien rewrites attachment addresses in the Markdown to the
// paths inside the archive.

// Only attachments of the exported pages are rewritten: references pointing
// to a page that isn't included remain unchanged. A link that points to
// nothing is more honest than one that points to a file missing from the
// archive.
func adressenAufDateien(md string, dateien []exportDatei) string {
	if len(dateien) == 0 || !strings.Contains(md, "/attachments/") {
		return md
	}
	for _, d := range dateien {
		alt := "/api/pages/" + d.Seite + "/attachments/" + d.ID
		// In spitzen Klammern, denn ein Dateiname darf Leerzeichen tragen und
		// wuerde den Verweis sonst nach dem ersten Wort beenden.
		md = strings.ReplaceAll(md, "("+alt+")", "(<"+d.Pfad+">)")
		md = strings.ReplaceAll(md, alt, d.Pfad)
	}
	return md
}

// anhangListe appends to a page a list of files that are not referenced in
// the text.

// An image referenced in the text is mentioned; an attachment recorded in the
// adjacent column may be nowhere. Without this list it would lie in the
// archive and never be found.
func anhangListe(md string, dateien []exportDatei) string {
	var fehlend []exportDatei
	for _, d := range dateien {
		if !strings.Contains(md, d.Pfad) {
			fehlend = append(fehlend, d)
		}
	}
	if len(fehlend) == 0 {
		return md
	}
	sort.Slice(fehlend, func(i, j int) bool { return fehlend[i].Name < fehlend[j].Name })

	var b strings.Builder
	b.WriteString(strings.TrimRight(md, "\n"))
	b.WriteString("\n\n## Anhänge\n\n")
	for _, d := range fehlend {
		fmt.Fprintf(&b, "- [%s](<%s>)\n", klammersicher(d.Name), d.Pfad)
	}
	return b.String()
}

// dateienSchreiben places attachments into the archive.

// The stream goes from the storage straight into the ZIP without buffering:
// a storage with a few hundred images would otherwise occupy memory twice.
// A missing file is skipped rather than treated as an error — the archive
// will be incomplete but will be produced, and the missing file was already
// gone.
func (s *Server) dateienSchreiben(ctx context.Context, zw *zip.Writer, dateien []exportDatei) {
	for _, d := range dateien {
		f, err := s.Ablage.Lesen(ctx, d.ID)
		if err != nil {
			continue
		}
		// Deflate gives no benefit for a JPEG and costs time; therefore we
		// store such formats without compression, and deflate only where it
		// makes sense.
		verfahren := zip.Deflate
		if schonGepackt(d.Name) {
			verfahren = zip.Store
		}
		kopf, err := zw.CreateHeader(&zip.FileHeader{Name: d.Pfad, Method: verfahren})
		if err != nil {
			f.Close()
			return
		}
		_, _ = io.Copy(kopf, f)
		f.Close()
	}
}

// schonGepackt sagt, ob ein Format bereits verlustbehaftet oder gepackt ist.
func schonGepackt(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".zip", ".mp4", ".mp3", ".pdf", ".docx", ".xlsx", ".pptx":
		return true
	}
	return false
}
