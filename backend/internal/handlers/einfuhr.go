// Importing Markdown, a single file, or a whole archive with its structure.
//
// Export answers "what happens if we stop using this". Import answers the
// question that comes first and is asked more often: what happens to the notes
// that already exist. A wiki nobody can move into is a wiki nobody starts with,
// and the two hundred files somebody already has are the reason they are
// looking at all.
//
// The archive keeps its shape. A folder becomes a page, the files inside it
// become its subpages, and links between files are rewritten so they still
// point somewhere after the move, that is the part a copy-and-paste migration
// never gets right.
package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"nexora/internal/einlesen"
	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// einfuhrDatei is one file from the import, together with the path it had
// inside the archive. That path is what links later find it by.
type einfuhrDatei struct {
	pfad   string
	inhalt []byte
}

// einfuhrSeite is a page while it comes into being: created first, then filled
// with content once every id is known.
type einfuhrSeite struct {
	id      string
	titel   string
	pfad    string // Quellpfad im Archiv, "" bei einem Sammelknoten
	verz    string // the directory the source file came from
	kopf    einlesen.Kopf
	bloecke []einlesen.Block
	eltern  *einfuhrSeite // nil means: hangs off the import target
}

// einfuhrVorschau is the same plan, only without consequences.
type einfuhrVorschau struct {
	Seiten    int          `json:"seiten"`
	Beilagen  int          `json:"beilagen"`
	Baum      []einfuhrAst `json:"baum"`
	Warnungen []string     `json:"warnungen"`
	// Set when the import will create a space of its own. In the preview this
	// only carries the name; nothing has been created at that point.
	Ablage string `json:"ablage,omitempty"`
}

// einfuhrAst is one node of the preview. The source file stands next to the
// title because a title alone does not reveal which file it came from, and that
// is exactly the question when a page turns up somewhere unexpected.
type einfuhrAst struct {
	Titel  string       `json:"titel"`
	Quelle string       `json:"quelle"`
	Kinder []einfuhrAst `json:"kinder,omitempty"`
}

// baumAusPlan turns the flat plan into the shape the preview expects: a tree
// with children, not a list with paths.
func baumAusPlan(plan []*einfuhrSeite) []einfuhrAst {
	kinder := map[*einfuhrSeite][]*einfuhrSeite{}
	var oben []*einfuhrSeite
	for _, sp := range plan {
		if sp.eltern == nil {
			oben = append(oben, sp)
		} else {
			kinder[sp.eltern] = append(kinder[sp.eltern], sp)
		}
	}
	var bauen func([]*einfuhrSeite) []einfuhrAst
	bauen = func(liste []*einfuhrSeite) []einfuhrAst {
		out := make([]einfuhrAst, 0, len(liste))
		for _, sp := range liste {
			out = append(out, einfuhrAst{
				Titel:  sp.titel,
				Quelle: sp.pfad,
				Kinder: bauen(kinder[sp]),
			})
		}
		return out
	}
	return bauen(oben)
}

type einfuhrBericht struct {
	Seiten    int      `json:"seiten"`
	Anhaenge  int      `json:"anhaenge"`
	Wurzeln   []string `json:"wurzeln"`
	Warnungen []string `json:"warnungen"`
	// The space that was created, if the import brought one along. The
	// interface jumps into it afterwards: an import you have to go looking for
	// is half lost.
	Ablage *einfuhrAblage `json:"ablage,omitempty"`
}

type einfuhrAblage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// The names under which a directory keeps its own page. Tried in order, so a
// folder containing index.md becomes a page with content rather than an empty
// shell with a subpage called "index".
var indexNamen = []string{
	"index.md", "readme.md", "inhalt.md", "index.markdown",
	// The same for HTML: a Confluence export keeps its cover page as
	// index.html, and without these lines it would hang beside the folder as an
	// ordinary page instead of above it.
	"index.html", "index.htm", "readme.html",
}

// Import accepts Markdown: single files or a ZIP archive.
//
// The process has two passes, and that is not a detour. First every page is
// created, then the links are resolved: a link to a file that would only come
// up later would have no target in the first pass. Working in a single pass can
// resolve cross references forwards only, and a wiki links in both directions.
func (s *Server) Import(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	r.Body = http.MaxBytesReader(w, r.Body, EinfuhrGrenze())
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "Einfuhr zu groß oder unlesbar")
		return
	}
	defer r.MultipartForm.RemoveAll()

	elternID := strings.TrimSpace(r.FormValue("parentId"))
	spaceID := strings.TrimSpace(r.FormValue("spaceId"))
	// Importing a whole space: the archive brings its own instead of mixing into
	// an existing one. This is the way back from an export, so a space that was
	// exported comes back in without creating a space by hand first.
	neueAblage := strings.TrimSpace(r.FormValue("neueAblage"))
	if neueAblage != "" {
		if elternID != "" || spaceID != "" {
			writeErr(w, http.StatusBadRequest,
				"entweder eine neue Ablage oder ein vorhandenes Ziel, nicht beides")
			return
		}
		if len([]rune(neueAblage)) > 120 {
			writeErr(w, http.StatusBadRequest, "Name der Ablage ist zu lang")
			return
		}
	}

	// Who may write here is decided by the same rule as creating a single page.
	// An import is nothing but many pages at once, and it must not be a loophole
	// beside that rule.
	if elternID != "" {
		if _, darf, _, ok := s.pagePerm(r.Context(), uid, elternID); !ok || !darf {
			writeErr(w, http.StatusForbidden, "keine Schreibrechte auf der Zielseite")
			return
		}
	}
	if spaceID != "" && !s.darfInSpaceSchreiben(r.Context(), uid, spaceID) {
		writeErr(w, http.StatusForbidden, "keine Schreibrechte in dieser Ablage")
		return
	}

	// Not nil: otherwise the response carries null instead of an empty list, and
	// the caller would have to tell the two apart.
	warnungen := []string{}
	var mdDateien []einfuhrDatei
	beilagen := map[string]einfuhrDatei{}

	koepfe := r.MultipartForm.File["file"]
	if len(koepfe) == 0 {
		writeErr(w, http.StatusBadRequest, "keine Datei angegeben")
		return
	}

	for _, fk := range koepfe {
		datei, err := fk.Open()
		if err != nil {
			warnungen = append(warnungen, fk.Filename+": nicht lesbar")
			continue
		}
		name := path.Base(strings.ReplaceAll(fk.Filename, "\\", "/"))

		if strings.EqualFold(path.Ext(name), ".zip") {
			md, bei, warn := archivLesen(datei, fk.Size)
			mdDateien = append(mdDateien, md...)
			for k, v := range bei {
				beilagen[k] = v
			}
			warnungen = append(warnungen, warn...)
			datei.Close()
			continue
		}

		inhalt, err := io.ReadAll(io.LimitReader(datei, MaxAnhangBytes()))
		datei.Close()
		if err != nil {
			warnungen = append(warnungen, name+": nicht lesbar")
			continue
		}
		if istMarkdown(name) || istHTML(name) {
			mdDateien = append(mdDateien, einfuhrDatei{pfad: name, inhalt: inhalt})
		} else {
			// A single uploaded file that is not Markdown has no page to belong
			// to. Discarding it silently here would be the nastier outcome.
			warnungen = append(warnungen, name+": weder Markdown noch HTML, übergangen")
		}
	}

	if len(mdDateien) == 0 {
		writeErr(w, http.StatusBadRequest, "keine Markdown- oder HTML-Datei in der Einfuhr")
		return
	}

	// When a whole space is read back in, our own table of contents is dropped,
	// see istAusfuhrVerzeichnis.
	if neueAblage != "" {
		behalten := mdDateien[:0]
		for _, d := range mdDateien {
			if strings.EqualFold(path.Base(d.pfad), "INHALT.md") && istAusfuhrVerzeichnis(d.inhalt) {
				warnungen = append(warnungen,
					"INHALT.md der Ausfuhr übergangen. Die Ablage selbst ist das Verzeichnis.")
				continue
			}
			behalten = append(behalten, d)
		}
		mdDateien = behalten
	}

	plan := planen(mdDateien)

	// Preview: the same plan, but nothing is created. Someone importing two
	// hundred files wants to see what will come of it beforehand, because
	// undoing it would mean moving two hundred pages into the trash one by one.
	if r.FormValue("vorschau") != "" {
		writeJSON(w, http.StatusOK, einfuhrVorschau{
			Seiten:    len(plan),
			Beilagen:  len(beilagen),
			Baum:      baumAusPlan(plan),
			Warnungen: warnungen,
			Ablage:    neueAblage,
		})
		return
	}

	// Created only now, not during the preview: otherwise every glance into an
	// archive would leave an empty space behind.
	var ablage *einfuhrAblage
	if neueAblage != "" {
		var neu einfuhrAblage
		neu.Name = neueAblage
		if err := s.Pool.QueryRow(r.Context(),
			`INSERT INTO spaces (owner_id, name) VALUES ($1, $2) RETURNING id`,
			uid, neueAblage).Scan(&neu.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, "Ablage konnte nicht angelegt werden")
			return
		}
		s.spurAusRequest(r, AktSpaceAngelegt, "space", neu.ID, neu.Name,
			map[string]any{"aus": "einfuhr"})
		spaceID = neu.ID
		ablage = &neu
	}

	wurzeln, err := s.anlegen(r.Context(), uid, elternID, spaceID, plan)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Seiten konnten nicht angelegt werden")
		return
	}
	seiten := plan

	// Second pass: resolve links, attach enclosures, write the content.
	nachPfad := map[string]*einfuhrSeite{}
	for _, sp := range seiten {
		if sp.pfad != "" {
			nachPfad[sp.pfad] = sp
		}
	}
	benutzt := map[string]bool{}
	anhaenge := 0

	for _, sp := range seiten {
		zahl := s.verweiseAufloesen(r.Context(), uid, sp, nachPfad, beilagen, benutzt, &warnungen)
		anhaenge += zahl
		if err := s.inhaltSchreiben(r.Context(), sp); err != nil {
			warnungen = append(warnungen, sp.titel+": Inhalt nicht gespeichert")
		}
		s.tagsSetzen(r.Context(), uid, sp)
	}

	// Whatever was in the archive that no page refers to is attached to the page
	// of its directory. Otherwise it would vanish on import, and nobody would
	// notice something is missing.
	anhaenge += s.beilagenNachtragen(r.Context(), uid, seiten, beilagen, benutzt, &warnungen)

	// With a space of its own, that space goes into the entry, otherwise the
	// target page does. The question afterwards is "where did this go", and the
	// entry has to be able to answer it.
	art, zielID := "seite", elternID
	if ablage != nil {
		art, zielID = "space", ablage.ID
	}
	s.spurAusRequest(r, AktEinfuhr, art, zielID, einfuhrName(mdDateien),
		map[string]any{"seiten": len(seiten), "anhaenge": anhaenge, "dateien": len(koepfe)})

	writeJSON(w, http.StatusCreated, einfuhrBericht{
		Seiten:    len(seiten),
		Anhaenge:  anhaenge,
		Wurzeln:   wurzeln,
		Warnungen: warnungen,
		Ablage:    ablage,
	})
}

// einfuhrName describes the import for the audit trail.
func einfuhrName(dateien []einfuhrDatei) string {
	if len(dateien) == 1 {
		return dateien[0].pfad
	}
	return fmt.Sprintf("%d Dateien", len(dateien))
}

// archivLesen unpacks a ZIP and separates Markdown from everything else.
//
// Paths containing ".." are rejected. They could do no damage here, since
// nothing is written to disk and the paths only serve to resolve links, but an
// archive containing such a thing did not mean well, and the rest of it earns
// the same suspicion.
func archivLesen(datei io.ReaderAt, groesse int64) ([]einfuhrDatei, map[string]einfuhrDatei, []string) {
	var md []einfuhrDatei
	beilagen := map[string]einfuhrDatei{}
	var warnungen []string

	zr, err := zip.NewReader(datei, groesse)
	if err != nil {
		return nil, nil, []string{"Archiv nicht lesbar"}
	}

	for _, e := range zr.File {
		if e.FileInfo().IsDir() {
			continue
		}
		pfad := path.Clean(strings.ReplaceAll(e.Name, "\\", "/"))
		if strings.HasPrefix(pfad, "..") || strings.Contains(pfad, "../") || path.IsAbs(pfad) {
			warnungen = append(warnungen, e.Name+": verdächtiger Pfad, übergangen")
			continue
		}
		// Debris from archiving tools. Taking it along as an attachment would
		// be a gain for nobody.
		if strings.HasPrefix(pfad, "__MACOSX/") || path.Base(pfad) == ".DS_Store" ||
			strings.HasPrefix(path.Base(pfad), "._") || strings.HasPrefix(path.Base(pfad), ".") {
			continue
		}
		if int64(e.UncompressedSize64) > MaxAnhangBytes() {
			warnungen = append(warnungen, pfad+": größer als die Anhangsgrenze, übergangen")
			continue
		}

		rc, err := e.Open()
		if err != nil {
			warnungen = append(warnungen, pfad+": nicht lesbar")
			continue
		}
		inhalt, err := io.ReadAll(io.LimitReader(rc, MaxAnhangBytes()))
		rc.Close()
		if err != nil {
			warnungen = append(warnungen, pfad+": nicht lesbar")
			continue
		}

		if istMarkdown(pfad) || istHTML(pfad) {
			md = append(md, einfuhrDatei{pfad: pfad, inhalt: inhalt})
		} else {
			beilagen[pfad] = einfuhrDatei{pfad: pfad, inhalt: inhalt}
		}
	}
	// Creating things in archive order would mean relying on the mood of the
	// archiving tool. Sorted by path, a directory comes before its content, and
	// the tree grows from the top down.
	sort.Slice(md, func(i, j int) bool { return md[i].pfad < md[j].pfad })
	return md, beilagen, warnungen
}

// notionMuster recognises the id Notion appends to every file and folder name.
//
// A Notion export writes "Weekly plan 8f3a...c1", and whoever does not cut that
// off ends up with a hundred pages carrying gibberish in the title. It changes
// nothing about the path: links are resolved by path, not by title.
var notionMuster = regexp.MustCompile(`^(.*[^ -])[ -]+([0-9a-f]{32})$`)

// sauberterTitel strips that id from a name.
func sauberterTitel(name string) string {
	name = strings.TrimSpace(name)
	if m := notionMuster.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1])
	}
	return name
}

func istHTML(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return false
}

// istAusfuhrVerzeichnis recognises the table of contents our own export puts
// into every archive.
//
// When a whole space is read back in it is redundant: the list of pages is the
// space itself, and imported as a page it would stand there twice, once as an
// index and once as reality. Importing into something existing keeps it, since
// there it is the only hint of what belonged together.
//
// Only the exact shape export.go writes is recognised: a heading, a line
// "N Seiten, ausgegeben am ...", then nothing but links. A hand-written
// INHALT.md looks different and is left alone.
func istAusfuhrVerzeichnis(inhalt []byte) bool {
	zeilen := strings.Split(strings.TrimSpace(string(inhalt)), "\n")
	if len(zeilen) < 2 || !strings.HasPrefix(zeilen[0], "# ") {
		return false
	}
	gesehen := false
	for _, z := range zeilen[1:] {
		z = strings.TrimSpace(z)
		switch {
		case z == "":
		case strings.HasPrefix(z, "- [") && strings.Contains(z, "](<"):
			gesehen = true
		case strings.Contains(z, "ausgegeben am"):
		default:
			return false
		}
	}
	return gesehen
}

func istMarkdown(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mdx", ".txt":
		return true
	}
	return false
}

// planen builds the page tree from the paths in the archive without creating
// anything.
//
// A directory becomes a page. If it contains index.md (or README.md, or
// INHALT.md as our own export writes it), that document is the content of the
// page; otherwise an empty page appears that merely holds the subpages
// together. Either beats the flat heap an import otherwise produces.
//
// Kept separate from creating, so the same calculation can be used twice: once
// for the preview, which shows it, and once for the import, which acts on it.
func planen(dateien []einfuhrDatei) []*einfuhrSeite {
	var plan []*einfuhrSeite

	// Collect every directory that appears, intermediate levels included: an
	// archive holding only "a/b/c.md" still has the levels a and a/b.
	verzeichnisse := map[string]bool{"": true}
	for _, d := range dateien {
		v := path.Dir(d.pfad)
		if v == "." {
			v = ""
		}
		for v != "" {
			verzeichnisse[v] = true
			if i := strings.LastIndex(v, "/"); i > 0 {
				v = v[:i]
			} else {
				v = ""
			}
		}
	}
	sortiert := make([]string, 0, len(verzeichnisse))
	for v := range verzeichnisse {
		sortiert = append(sortiert, v)
	}
	// By depth, then by name: a parent comes into being before its child.
	sort.Slice(sortiert, func(i, j int) bool {
		ti, tj := strings.Count(sortiert[i], "/"), strings.Count(sortiert[j], "/")
		if len(sortiert[i]) == 0 || len(sortiert[j]) == 0 {
			return len(sortiert[i]) < len(sortiert[j])
		}
		if ti != tj {
			return ti < tj
		}
		return sortiert[i] < sortiert[j]
	})

	// Which file is the page of its directory.
	indexVon := map[string]string{}
	vorhanden := map[string]bool{}
	for _, d := range dateien {
		vorhanden[strings.ToLower(d.pfad)] = true
	}
	// Which paths are used up by that. Without this set the folder note would
	// come into being twice: once as the content of the folder and once as an
	// ordinary file of its own directory.
	istIndex := map[string]bool{}
	for _, v := range sortiert {
		var kandidaten []string
		for _, n := range indexNamen {
			kandidaten = append(kandidaten, path.Join(v, n))
		}
		if v != "" {
			// Obsidian keeps the folder note INSIDE the folder, under the same name.
			kandidaten = append(kandidaten, path.Join(v, path.Base(v)+".md"), path.Join(v, path.Base(v)+".html"))
			// Notion puts it BESIDE the folder, same name as the folder, one
			// level up. Both conventions are common, and both mean the same
			// thing: this text belongs to this folder.
			kandidaten = append(kandidaten, v+".md", v+".html")
		}
		for _, k := range kandidaten {
			p := strings.ToLower(strings.TrimPrefix(k, "/"))
			if vorhanden[p] && !istIndex[p] {
				indexVon[v] = p
				istIndex[p] = true
				break
			}
		}
	}

	// For every directory the page its content hangs below.
	verzSeite := map[string]*einfuhrSeite{}

	// The directory pages first, from the top down.
	for _, v := range sortiert {
		idx, hatIndex := indexVon[v]
		if v == "" && !hatIndex {
			// No cover page in the archive: the top level files hang directly
			// off the target page. An extra shell called "Import" would be a
			// level nobody asked for.
			continue
		}

		sp := &einfuhrSeite{verz: v, titel: sauberterTitel(path.Base(v)), eltern: verzSeite[elternVerzeichnis(v)]}
		if hatIndex {
			if d := findeDatei(dateien, idx); d != nil {
				sp.pfad = d.pfad
				t, k, b := dateiLesen(*d)
				sp.kopf, sp.bloecke = k, b
				if t != "" {
					sp.titel = t
				}
			}
		}
		if sp.titel == "" || sp.titel == "." {
			sp.titel = "Einfuhr"
		}
		plan = append(plan, sp)
		verzSeite[v] = sp
	}

	// Then all the remaining files.
	for i := range dateien {
		d := dateien[i]
		v := path.Dir(d.pfad)
		if v == "." {
			v = ""
		}
		if istIndex[strings.ToLower(d.pfad)] {
			continue // already part of a directory page
		}
		titel, kopf, bloecke := dateiLesen(d)
		if titel == "" {
			titel = sauberterTitel(strings.TrimSuffix(path.Base(d.pfad), path.Ext(d.pfad)))
		}
		plan = append(plan, &einfuhrSeite{
			pfad: d.pfad, verz: v, titel: titel, kopf: kopf, bloecke: bloecke,
			eltern: verzSeite[v],
		})
	}
	return plan
}

// dateiLesen picks the reader by file extension. Markdown and HTML both come
// out as blocks; beyond that the rest of the process makes no distinction.
func dateiLesen(d einfuhrDatei) (string, einlesen.Kopf, []einlesen.Block) {
	if istHTML(d.pfad) {
		titel, bloecke := einlesen.LiesHTML(string(d.inhalt))
		return titel, einlesen.Kopf{}, bloecke
	}
	return einlesen.Lies(string(d.inhalt))
}

// anlegen writes the plan to the database and fills in the ids.
func (s *Server) anlegen(ctx context.Context, uid, elternID, spaceID string, plan []*einfuhrSeite) ([]string, error) {
	var wurzeln []string
	for i, sp := range plan {
		eltern := elternID
		if sp.eltern != nil {
			eltern = sp.eltern.id
		}
		var elternWert, spaceWert any
		if eltern != "" {
			elternWert = eltern
		}
		if spaceID != "" {
			spaceWert = spaceID
		}
		var id string
		err := s.Pool.QueryRow(ctx,
			`INSERT INTO pages (owner_id, parent_id, space_id, title, content, icon, content_text, sort_order)
			 VALUES ($1, $2, $3, $4, '[]'::jsonb, $5, '', $6) RETURNING id`,
			uid, elternWert, spaceWert, sp.titel, sp.kopf.Icon, i+1).Scan(&id)
		if err != nil {
			return nil, err
		}
		sp.id = id
		if sp.eltern == nil {
			wurzeln = append(wurzeln, id)
		}
	}
	return wurzeln, nil
}

func elternVerzeichnis(v string) string {
	if i := strings.LastIndex(v, "/"); i > 0 {
		return v[:i]
	}
	return ""
}

func findeDatei(dateien []einfuhrDatei, kleinPfad string) *einfuhrDatei {
	for i := range dateien {
		if strings.ToLower(dateien[i].pfad) == kleinPfad {
			return &dateien[i]
		}
	}
	return nil
}

// verweiseAufloesen rewrites the links of one page and returns the number of
// attachments created along the way.
//
// Three cases. A link to another imported file becomes [[Title]], the link form
// Nexora writes itself, which feeds backlinks and the knowledge graph. A link
// to a bundled file becomes an attachment of this page. Everything else stays
// as it was: an address on the web is as valid after the import as before.
func (s *Server) verweiseAufloesen(ctx context.Context, uid string, sp *einfuhrSeite,
	nachPfad map[string]*einfuhrSeite, beilagen map[string]einfuhrDatei,
	benutzt map[string]bool, warnungen *[]string) int {

	anzahl := 0
	// The same bundled file can appear several times on one page; it should
	// still be uploaded only once.
	angehaengt := map[string]string{}
	// A warning that appeared for every file would stop being a warning.
	gemeldet := map[string]bool{}

	// seiteZu returns the imported page an address points to.
	seiteZu := func(adresse string) *einfuhrSeite {
		ziel := zielPfad(adresse, sp.verz)
		if ziel == "" {
			return nil
		}
		if z, ok := nachPfad[ziel]; ok {
			return z
		}
		// Obsidian likes to link without the file extension.
		if z, ok := nachPfad[ziel+".md"]; ok {
			return z
		}
		return nil
	}
	beilageZu := func(adresse string) *einfuhrDatei {
		ziel := zielPfad(adresse, sp.verz)
		if ziel == "" {
			return nil
		}
		if b, ok := beilagen[ziel]; ok {
			return &b
		}
		return nil
	}

	// anhaengen stores the bundled file as an attachment of this page and
	// returns the address it can be fetched under.
	anhaengen := func(b *einfuhrDatei) string {
		if adresse, da := angehaengt[b.pfad]; da {
			return adresse
		}
		// Attachments are a paid extra. Creating them through the import anyway
		// would be a way around the gate, and one that leads nowhere: the file
		// could afterwards be neither listed nor downloaded.
		if !lizenz.Frei(lizenz.Anhaenge) {
			if !gemeldet["anhaenge"] {
				gemeldet["anhaenge"] = true
				*warnungen = append(*warnungen, "Anhänge sind ein Zusatz. Die Dateien aus dem Archiv wurden übergangen.")
			}
			return ""
		}
		attID, err := s.anhangAnlegen(ctx, sp.id, uid, path.Base(b.pfad), b.inhalt)
		if err != nil {
			grund := "Anhang nicht gespeichert"
			if errors.Is(err, errProgrammdatei) {
				grund = "übersprungen, ausführbares Programm"
			}
			*warnungen = append(*warnungen, b.pfad+": "+grund)
			return ""
		}
		anzahl++
		benutzt[b.pfad] = true
		adresse := fmt.Sprintf("/api/pages/%s/attachments/%s", sp.id, attID)
		angehaengt[b.pfad] = adresse
		return adresse
	}

	var inlineGehen func([]einlesen.Inline) []einlesen.Inline
	inlineGehen = func(teile []einlesen.Inline) []einlesen.Inline {
		out := make([]einlesen.Inline, 0, len(teile))
		for _, t := range teile {
			if t.Type != "link" || t.Href == "" {
				t.Content = inlineGehen(t.Content)
				out = append(out, t)
				continue
			}
			if ziel := seiteZu(t.Href); ziel != nil {
				// The label is lost when it differs from the title, since a
				// wiki link carries the title of the target page. In exchange
				// the link survives files being renamed, which happens during
				// an import anyway.
				out = append(out, einlesen.Inline{
					Type: "text",
					Text: "[[" + ziel.titel + "]]",
				})
				s.verweisMerken(ctx, sp.id, ziel.id)
				continue
			}
			if b := beilageZu(t.Href); b != nil {
				if adresse := anhaengen(b); adresse != "" {
					t.Href = adresse
				}
			}
			t.Content = inlineGehen(t.Content)
			out = append(out, t)
		}
		return out
	}

	var bloeckeGehen func([]einlesen.Block) []einlesen.Block
	bloeckeGehen = func(bloecke []einlesen.Block) []einlesen.Block {
		for i := range bloecke {
			switch inhalt := bloecke[i].Content.(type) {
			case []einlesen.Inline:
				bloecke[i].Content = inlineGehen(inhalt)
			case einlesen.TabellenInhalt:
				for z := range inhalt.Rows {
					for c := range inhalt.Rows[z].Cells {
						inhalt.Rows[z].Cells[c] = inlineGehen(inhalt.Rows[z].Cells[c])
					}
				}
				bloecke[i].Content = inhalt
			}

			// Images carry their address in the properties, not in the text.
			if adresse, ok := bloecke[i].Props["url"].(string); ok && adresse != "" {
				if b := beilageZu(adresse); b != nil {
					if neu := anhaengen(b); neu != "" {
						bloecke[i].Props["url"] = neu
					}
				} else if ziel := seiteZu(adresse); ziel != nil {
					// An image pointing at a document is not an image.
					bloecke[i] = einlesen.Block{
						Type:    "paragraph",
						Content: []einlesen.Inline{{Type: "text", Text: "[[" + ziel.titel + "]]"}},
					}
					s.verweisMerken(ctx, sp.id, ziel.id)
					continue
				}
			}
			bloecke[i].Children = bloeckeGehen(bloecke[i].Children)
		}
		return bloecke
	}

	sp.bloecke = bloeckeGehen(sp.bloecke)
	return anzahl
}

// zielPfad turns an address in the document into a path inside the archive.
//
// Web addresses and anchor jumps have no target inside the archive and return
// empty. The rest is percent-decoded: a link to "my%20image.png" means the file
// "my image.png", and without this step it would never be found.
func zielPfad(adresse, verzeichnis string) string {
	if adresse == "" || strings.HasPrefix(adresse, "#") {
		return ""
	}
	if i := strings.IndexAny(adresse, "#?"); i >= 0 {
		adresse = adresse[:i]
	}
	if adresse == "" {
		return ""
	}
	klein := strings.ToLower(adresse)
	for _, p := range []string{"http://", "https://", "mailto:", "data:", "ftp://", "//"} {
		if strings.HasPrefix(klein, p) {
			return ""
		}
	}
	if entpackt, err := url.PathUnescape(adresse); err == nil {
		adresse = entpackt
	}
	adresse = strings.ReplaceAll(adresse, "\\", "/")
	// An absolute address points at the archive root and is therefore not
	// resolved relative to the directory of the linking file.
	if strings.HasPrefix(adresse, "/") {
		return path.Clean(strings.TrimPrefix(adresse, "/"))
	}
	if verzeichnis != "" {
		adresse = path.Join(verzeichnis, adresse)
	}
	return path.Clean(adresse)
}

// verweisMerken additionally records the link in page_links.
//
// The text carries [[Title]] and is therefore readable; the row here carries
// the id and therefore survives the target page being renamed. Having both
// costs one row and saves the knowledge graph.
func (s *Server) verweisMerken(ctx context.Context, quelle, ziel string) {
	if quelle == ziel {
		return
	}
	if _, err := s.Pool.Exec(ctx,
		`INSERT INTO page_links (source_id, target_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		quelle, ziel); err != nil {
		log.Printf("Einfuhr: Verweis %s -> %s: %v", quelle, ziel, err)
	}
}

// inhaltSchreiben stores the finished content on the page.
func (s *Server) inhaltSchreiben(ctx context.Context, sp *einfuhrSeite) error {
	bloecke := sp.bloecke
	if len(bloecke) == 0 {
		bloecke = []einlesen.Block{{Type: "paragraph"}}
	}
	roh, err := json.Marshal(bloecke)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx,
		`UPDATE pages SET content = $2::jsonb, content_text = $3, updated_at = now() WHERE id = $1`,
		sp.id, string(roh), textAusInhalt(roh))
	return err
}

// tagsSetzen creates the tags from the front matter and attaches them. Tags
// belong to the account, not to the page, so an existing one is reused rather
// than duplicated.
func (s *Server) tagsSetzen(ctx context.Context, uid string, sp *einfuhrSeite) {
	for _, name := range sp.kopf.Tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var tagID string
		if err := s.Pool.QueryRow(ctx,
			`INSERT INTO tags (owner_id, name, color) VALUES ($1, $2, '#6b7280')
			 ON CONFLICT (owner_id, name) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`, uid, name).Scan(&tagID); err != nil {
			continue
		}
		s.Pool.Exec(ctx,
			`INSERT INTO page_tags (page_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			sp.id, tagID)
	}
}

// beilagenNachtragen attaches whatever nobody referred to.
//
// An archive often contains files no document refers to: old versions, images
// from a deleted paragraph, enclosures. Dropping them during the import would
// be a quiet loss; they end up on the page of their directory, where they sat
// in the archive too.
func (s *Server) beilagenNachtragen(ctx context.Context, uid string, seiten []*einfuhrSeite,
	beilagen map[string]einfuhrDatei, benutzt map[string]bool, warnungen *[]string) int {

	if len(beilagen) == 0 {
		return 0
	}
	if !lizenz.Frei(lizenz.Anhaenge) {
		return 0
	}
	verzSeite := map[string]*einfuhrSeite{}
	for _, sp := range seiten {
		if _, da := verzSeite[sp.verz]; !da || sp.pfad == "" {
			verzSeite[sp.verz] = sp
		}
	}

	offen := make([]string, 0, len(beilagen))
	for p := range beilagen {
		if !benutzt[p] {
			offen = append(offen, p)
		}
	}
	sort.Strings(offen)

	anzahl := 0
	for _, p := range offen {
		verz := path.Dir(p)
		if verz == "." {
			verz = ""
		}
		ziel := verzSeite[verz]
		for ziel == nil && verz != "" {
			verz = elternVerzeichnis(verz)
			ziel = verzSeite[verz]
		}
		if ziel == nil && len(seiten) > 0 {
			ziel = seiten[0]
		}
		if ziel == nil {
			continue
		}
		b := beilagen[p]
		if _, err := s.anhangAnlegen(ctx, ziel.id, uid, path.Base(p), b.inhalt); err != nil {
			grund := "Anhang nicht gespeichert"
			if errors.Is(err, errProgrammdatei) {
				grund = "übersprungen, ausführbares Programm"
			}
			*warnungen = append(*warnungen, p+": "+grund)
			continue
		}
		anzahl++
	}
	return anzahl
}

// anhangAnlegen speichert Bytes als Anhang einer Seite. Dieselbe Reihenfolge
// the same order as an upload through the browser: the row first, then the
// file, and on failure the row goes away again. An attachment you can click
// that leads nowhere is worse than none at all.
func (s *Server) anhangAnlegen(ctx context.Context, seiteID, uid, dateiname string, inhalt []byte) (string, error) {
	if dateiname == "" || dateiname == "." || dateiname == "/" {
		dateiname = "datei"
	}
	// Dieselbe Grenze wie beim Hochladen von Hand, siehe programmdatei.go. Ein
	// Archiv ist der bequemere Weg, ein Programm hereinzutragen, nicht der
	// erlaubtere.
	if istLinuxProgramm(inhalt) {
		return "", errProgrammdatei
	}
	typ := typAusAngabeUndName("", dateiname)

	var attID string
	if err := s.Pool.QueryRow(ctx,
		`INSERT INTO attachments (page_id, owner_id, filename, mime, size)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		seiteID, uid, dateiname, typ, len(inhalt)).Scan(&attID); err != nil {
		return "", err
	}
	if _, err := s.Ablage.Schreiben(ctx, attID, bytes.NewReader(inhalt), int64(len(inhalt)), typ); err != nil {
		s.Pool.Exec(ctx, `DELETE FROM attachments WHERE id=$1`, attID)
		return "", err
	}
	if lizenz.Frei(lizenz.Anhangsuche) {
		if txt := textAusAnhang(ctx, inhalt, typ, dateiname); txt != "" {
			s.Pool.Exec(ctx, `UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, attID, txt)
		}
	}
	return attID, nil
}

// darfInSpaceSchreiben checks the right to create new pages in a space. The
// same ladder as everywhere else: owner, administrator, a right of 'schreiben'
// or 'verwalten', or a space that stands open to everyone.
func (s *Server) darfInSpaceSchreiben(ctx context.Context, uid, spaceID string) bool {
	var erlaubt bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM spaces WHERE id = $1 AND owner_id = $2)
		    OR EXISTS (SELECT 1 FROM users  WHERE id = $2 AND role = 'admin')
		    OR EXISTS (SELECT 1 FROM spaces WHERE id = $1 AND oeffentlich = 'schreiben')
		    OR ($3 AND EXISTS (
		          SELECT 1 FROM space_rechte sr
		           WHERE sr.space_id = $1 AND sr.recht IN ('schreiben', 'verwalten')
		             AND (sr.user_id = $2
		                  OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
		                                      WHERE gm.user_id = $2))))`,
		spaceID, uid, lizenz.Frei(lizenz.Gruppen)).Scan(&erlaubt)
	return err == nil && erlaubt
}
