// Importing Markdown -- a single file, or a whole archive with its structure.
//
// Export answers "what happens if we stop using this". Import answers the
// question that comes first and is asked more often: what happens to the notes
// that already exist. A wiki nobody can move into is a wiki nobody starts with,
// and the two hundred files somebody already has are the reason they are
// looking at all.
//
// The archive keeps its shape. A folder becomes a page, the files inside it
// become its subpages, and links between files are rewritten so they still
// point somewhere after the move -- that is the part a copy-and-paste migration
// never gets right.
package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
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

// einfuhrDatei ist eine Datei aus der Einfuhr, mit dem Pfad, unter dem sie im
// Archiv stand -- der Pfad ist das, woran Verweise sie später wiederfinden.
type einfuhrDatei struct {
	pfad   string
	inhalt []byte
}

// einfuhrSeite ist eine Seite, während sie entsteht: erst angelegt, dann mit
// aufgelösten Verweisen gefüllt.
type einfuhrSeite struct {
	id      string
	titel   string
	pfad    string // Quellpfad im Archiv, "" bei einem Sammelknoten
	verz    string // Verzeichnis, in dem die Quelle lag
	kopf    einlesen.Kopf
	bloecke []einlesen.Block
	eltern  *einfuhrSeite // nil heißt: hängt am Ziel der Einfuhr
}

// einfuhrVorschau ist derselbe Plan, nur ohne Folgen.
type einfuhrVorschau struct {
	Seiten    int          `json:"seiten"`
	Beilagen  int          `json:"beilagen"`
	Baum      []einfuhrAst `json:"baum"`
	Warnungen []string     `json:"warnungen"`
	// Gesetzt, wenn die Einfuhr eine eigene Ablage anlegen wird. In der
	// Vorschau steht hier nur der Name -- angelegt ist sie da noch nicht.
	Ablage string `json:"ablage,omitempty"`
}

// einfuhrAst ist ein Knoten der Vorschau. Quelle steht daneben, weil ein Titel
// allein nicht verrät, aus welcher Datei er stammt -- und genau das ist die
// Frage, wenn eine Seite an einer unerwarteten Stelle auftaucht.
type einfuhrAst struct {
	Titel  string       `json:"titel"`
	Quelle string       `json:"quelle"`
	Kinder []einfuhrAst `json:"kinder,omitempty"`
}

// baumAusPlan formt die flache Planliste in die Gestalt, die die Vorschau
// anzeigt.
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
	// Die angelegte Ablage, falls die Einfuhr eine mitgebracht hat. Die
	// Oberfläche springt danach hinein -- eine Einfuhr, die man erst suchen
	// muss, ist halb verloren.
	Ablage *einfuhrAblage `json:"ablage,omitempty"`
}

type einfuhrAblage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Namen, unter denen ein Verzeichnis seine eigene Seite ablegt. Der Reihe nach
// probiert: so wird aus einem Ordner mit index.md eine Seite mit Inhalt statt
// einer leeren Hülle mit einer Unterseite namens "index".
var indexNamen = []string{
	"index.md", "readme.md", "inhalt.md", "index.markdown",
	// Dasselbe für HTML: eine Confluence-Ausfuhr legt ihr Deckblatt als
	// index.html ab, und ohne diese Zeilen hinge es als gewöhnliche Seite
	// neben dem Ordner statt darüber.
	"index.html", "index.htm", "readme.html",
}

// Import nimmt Markdown entgegen: einzelne Dateien oder ein ZIP-Archiv.
//
// Der Ablauf hat zwei Durchgänge, und das ist kein Umweg. Erst entstehen alle
// Seiten, dann werden die Verweise aufgelöst -- ein Verweis auf eine Datei, die
// später an die Reihe käme, hätte im ersten Durchgang kein Ziel. Wer in einem
// Durchgang arbeitet, kann Querverweise nur nach vorn auflösen, und ein
// Wiki verweist in beide Richtungen.
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
	// Eine ganze Ablage einführen: das Archiv bringt seine eigene mit, statt
	// sich in eine vorhandene zu mischen. Genau der Weg zurück aus der
	// Ausfuhr -- ein Space, den man exportiert hat, kommt so wieder herein,
	// ohne dass man vorher von Hand eine Ablage anlegt.
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

	// Wer hierhin schreiben darf, entscheidet dieselbe Regel wie beim
	// Anlegen einer einzelnen Seite. Eine Einfuhr ist nichts anderes als
	// viele Seiten auf einmal, und sie darf kein Schlupfloch daneben sein.
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

	// Nicht nil: die Antwort trägt sonst null statt einer leeren Liste, und
	// der Aufrufer müsste beides unterscheiden können.
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
			// Eine einzeln hochgeladene Datei, die kein Markdown ist, hat
			// keine Seite, an die sie gehört. Sie hier stillschweigend zu
			// verwerfen wäre der unangenehmere Ausgang.
			warnungen = append(warnungen, name+": weder Markdown noch HTML, übergangen")
		}
	}

	if len(mdDateien) == 0 {
		writeErr(w, http.StatusBadRequest, "keine Markdown- oder HTML-Datei in der Einfuhr")
		return
	}

	// Beim Einlesen einer ganzen Ablage fällt das eigene Inhaltsverzeichnis
	// weg -- siehe istAusfuhrVerzeichnis.
	if neueAblage != "" {
		behalten := mdDateien[:0]
		for _, d := range mdDateien {
			if strings.EqualFold(path.Base(d.pfad), "INHALT.md") && istAusfuhrVerzeichnis(d.inhalt) {
				warnungen = append(warnungen,
					"INHALT.md der Ausfuhr übergangen -- die Ablage selbst ist das Verzeichnis")
				continue
			}
			behalten = append(behalten, d)
		}
		mdDateien = behalten
	}

	plan := planen(mdDateien)

	// Vorschau: derselbe Plan, aber nichts wird angelegt. Wer zweihundert
	// Dateien einführt, will vorher sehen, was daraus wird -- rückgängig
	// machen hieße sonst, zweihundert Seiten einzeln in den Papierkorb zu
	// schieben.
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

	// Erst jetzt anlegen, nicht schon bei der Vorschau: sonst bliebe nach
	// jedem Blick in ein Archiv eine leere Ablage stehen.
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

	// Zweiter Durchgang: Verweise auflösen, Beilagen anhängen, Inhalt
	// schreiben.
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

	// Was im Archiv lag und auf das keine Seite verweist, hängt an der Seite
	// seines Verzeichnisses. Sonst verschwände es beim Import -- und niemand
	// würde bemerken, dass etwas fehlt.
	anhaenge += s.beilagenNachtragen(r.Context(), uid, seiten, beilagen, benutzt, &warnungen)

	// Bei einer eigenen Ablage steht sie im Eintrag, sonst die Zielseite: die
	// Frage hinterher lautet "wohin ging das", und darauf muss der Eintrag
	// antworten können.
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

// einfuhrName beschreibt die Einfuhr für die Prüfspur.
func einfuhrName(dateien []einfuhrDatei) string {
	if len(dateien) == 1 {
		return dateien[0].pfad
	}
	return fmt.Sprintf("%d Dateien", len(dateien))
}

// archivLesen packt ein ZIP aus und trennt Markdown von allem anderen.
//
// Einträge mit ".." im Pfad werden verworfen. Sie könnten hier zwar nichts
// anrichten -- geschrieben wird nichts auf die Platte, die Pfade dienen nur dem
// Wiederfinden von Verweisen --, aber ein Archiv, das so etwas enthält, hat es
// nicht gut gemeint, und der Rest davon verdient dasselbe Misstrauen.
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
		// Beiwerk der Packprogramme. Es als Anhang zu übernehmen wäre für
		// niemanden ein Gewinn.
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
	// In der Reihenfolge des Archivs anzulegen hieße, sich auf die Laune des
	// Packprogramms zu verlassen. Nach Pfad sortiert steht ein Verzeichnis vor
	// seinem Inhalt, und der Baum entsteht von oben nach unten.
	sort.Slice(md, func(i, j int) bool { return md[i].pfad < md[j].pfad })
	return md, beilagen, warnungen
}

// notionMuster erkennt die Kennung, die Notion an jeden Datei- und Ordnernamen
// hängt: 32 Stellen hexadezimal, mit einem Leerzeichen davor.
var notionMuster = regexp.MustCompile(`^(.*[^ -])[ -]+([0-9a-f]{32})$`)

// sauberterTitel macht aus einem Datei- oder Ordnernamen einen Titel.
//
// Notion-Ausfuhren tragen ihre innere Kennung im Namen -- "Wochenplan
// 8f3a...c1" --, und wer das nicht abschneidet, bekommt hundert Seiten mit
// Kauderwelsch im Titel. Am Pfad ändert das nichts: Verweise werden über den
// Pfad aufgelöst, nicht über den Titel.
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

// istAusfuhrVerzeichnis erkennt das Inhaltsverzeichnis, das die eigene Ausfuhr
// jedem Archiv beilegt.
//
// Beim Einlesen einer ganzen Ablage ist es überflüssig: die Liste der Seiten
// ist die Ablage selbst, und als Seite eingeführt stünde sie doppelt da --
// einmal als Verzeichnis, einmal als Wirklichkeit. Beim Einlesen in etwas
// Vorhandenes bleibt es dagegen stehen; dort ist es der einzige Hinweis, was
// zusammengehörte.
//
// Erkannt wird die Form, die export.go schreibt, und nur sie: eine Überschrift,
// eine Zeile "N Seiten, ausgegeben am ...", danach ausschließlich Verweise.
// Ein selbst geschriebenes INHALT.md sieht anders aus und bleibt unangetastet.
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

// planen baut den Seitenbaum aus den Pfaden im Archiv -- ohne etwas anzulegen.
//
// Ein Verzeichnis wird zu einer Seite. Enthält es index.md (oder README.md,
// oder INHALT.md, wie der eigene Export sie schreibt), ist dieses Dokument der
// Inhalt dieser Seite; sonst entsteht eine leere Seite, die nur die Unterseiten
// zusammenhält. Beides ist besser als der flache Haufen, zu dem ein Import
// sonst führt.
//
// Getrennt vom Anlegen, damit dieselbe Rechnung zweimal gebraucht werden kann:
// einmal für die Vorschau, die nichts anfasst, und einmal für die Einfuhr. Wer
// die Vorschau aus einem zweiten Stück Code baut, zeigt eines Tages etwas
// anderes an, als er anlegt.
func planen(dateien []einfuhrDatei) []*einfuhrSeite {
	var plan []*einfuhrSeite

	// Alle vorkommenden Verzeichnisse einsammeln, samt der Zwischenstufen: ein
	// Archiv, das nur "a/b/c.md" enthält, hat trotzdem die Ebenen a und a/b.
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
	// Nach Tiefe, dann nach Namen: ein Elternteil entsteht vor seinem Kind.
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

	// Welche Datei die Seite ihres Verzeichnisses ist.
	indexVon := map[string]string{}
	vorhanden := map[string]bool{}
	for _, d := range dateien {
		vorhanden[strings.ToLower(d.pfad)] = true
	}
	// Welche Pfade dadurch verbraucht sind. Ohne diese Menge entstünde die
	// Ordnernotiz zweimal: einmal als Inhalt des Ordners und einmal als
	// gewöhnliche Datei ihres eigenen Verzeichnisses.
	istIndex := map[string]bool{}
	for _, v := range sortiert {
		var kandidaten []string
		for _, n := range indexNamen {
			kandidaten = append(kandidaten, path.Join(v, n))
		}
		if v != "" {
			// Obsidian legt die Notiz zum Ordner IN den Ordner, gleichnamig.
			kandidaten = append(kandidaten, path.Join(v, path.Base(v)+".md"), path.Join(v, path.Base(v)+".html"))
			// Notion legt sie DANEBEN -- gleicher Name wie der Ordner, eine
			// Ebene höher. Beide Sitten sind verbreitet, und beide meinen
			// dasselbe: dieser Text gehört zu diesem Ordner.
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

	// Für jedes Verzeichnis die Seite, unter der sein Inhalt hängt.
	verzSeite := map[string]*einfuhrSeite{}

	// Erst die Verzeichnisseiten, von oben nach unten.
	for _, v := range sortiert {
		idx, hatIndex := indexVon[v]
		if v == "" && !hatIndex {
			// Kein Deckblatt im Archiv: die obersten Dateien hängen direkt an
			// der Zielseite. Eine zusätzliche Hülle "Import" wäre eine Ebene,
			// die niemand angelegt hat.
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

	// Dann alle übrigen Dateien.
	for i := range dateien {
		d := dateien[i]
		v := path.Dir(d.pfad)
		if v == "." {
			v = ""
		}
		if istIndex[strings.ToLower(d.pfad)] {
			continue // steckt schon in einer Verzeichnisseite
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

// dateiLesen wählt den Leser nach der Endung. Markdown und HTML kommen beide
// als Blöcke heraus; alles Weitere unterscheidet der Rest des Ablaufs nicht.
func dateiLesen(d einfuhrDatei) (string, einlesen.Kopf, []einlesen.Block) {
	if istHTML(d.pfad) {
		titel, bloecke := einlesen.LiesHTML(string(d.inhalt))
		return titel, einlesen.Kopf{}, bloecke
	}
	return einlesen.Lies(string(d.inhalt))
}

// anlegen schreibt den Plan in die Datenbank und trägt die Kennungen nach.
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

// verweiseAufloesen schreibt die Verweise einer Seite um und liefert die Zahl
// der dabei angelegten Anhänge.
//
// Drei Fälle. Ein Verweis auf eine andere eingeführte Datei wird zu
// [[Titel]] -- der Verweisform, die Nexora selbst schreibt und die Rückverweise
// und Wissensnetz speist. Ein Verweis auf eine Beilage wird zum Anhang dieser
// Seite. Alles andere bleibt, wie es war: eine Adresse ins Netz ist nach dem
// Import so gültig wie davor.
func (s *Server) verweiseAufloesen(ctx context.Context, uid string, sp *einfuhrSeite,
	nachPfad map[string]*einfuhrSeite, beilagen map[string]einfuhrDatei,
	benutzt map[string]bool, warnungen *[]string) int {

	anzahl := 0
	// Dieselbe Beilage kann in einer Seite mehrfach vorkommen; sie soll
	// trotzdem nur einmal hochgeladen werden.
	angehaengt := map[string]string{}
	// Ein Hinweis, der bei jeder Datei erschiene, wäre kein Hinweis mehr.
	gemeldet := map[string]bool{}

	// seiteZu liefert die eingeführte Seite, auf die eine Adresse zeigt.
	seiteZu := func(adresse string) *einfuhrSeite {
		ziel := zielPfad(adresse, sp.verz)
		if ziel == "" {
			return nil
		}
		if z, ok := nachPfad[ziel]; ok {
			return z
		}
		// Obsidian verlinkt gern ohne Endung.
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

	// anhaengen legt die Beilage als Anhang dieser Seite an und gibt die
	// Adresse zurück, unter der sie abrufbar ist.
	anhaengen := func(b *einfuhrDatei) string {
		if adresse, da := angehaengt[b.pfad]; da {
			return adresse
		}
		// Anhänge sind ein Zusatz. Sie über die Einfuhr doch anzulegen wäre
		// ein Weg um die Schranke herum -- und einer, der ins Leere führt: die
		// Datei ließe sich danach weder auflisten noch herunterladen.
		if !lizenz.Frei(lizenz.Anhaenge) {
			if !gemeldet["anhaenge"] {
				gemeldet["anhaenge"] = true
				*warnungen = append(*warnungen, "Anhänge sind ein Zusatz -- die Dateien aus dem Archiv wurden übergangen")
			}
			return ""
		}
		attID, err := s.anhangAnlegen(ctx, sp.id, uid, path.Base(b.pfad), b.inhalt)
		if err != nil {
			*warnungen = append(*warnungen, b.pfad+": Anhang nicht gespeichert")
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
				// Die Beschriftung geht dabei verloren, wenn sie vom Titel
				// abweicht -- ein Wiki-Verweis trägt den Titel der Zielseite.
				// Dafür überlebt der Verweis das Umbenennen von Dateien, das
				// beim Import ohnehin stattfindet.
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

			// Bilder tragen ihre Adresse in den Eigenschaften, nicht im Text.
			if adresse, ok := bloecke[i].Props["url"].(string); ok && adresse != "" {
				if b := beilageZu(adresse); b != nil {
					if neu := anhaengen(b); neu != "" {
						bloecke[i].Props["url"] = neu
					}
				} else if ziel := seiteZu(adresse); ziel != nil {
					// Ein Bild, das auf ein Dokument zeigt, ist kein Bild.
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

// zielPfad macht aus einer Adresse im Dokument einen Pfad im Archiv.
//
// Adressen ins Netz und Ankersprünge haben kein Ziel im Archiv und liefern
// leer. Der Rest wird entprozentet -- ein Verweis auf "mein%20bild.png" meint
// die Datei "mein bild.png", und ohne diesen Schritt fände er sie nie.
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
	// Eine absolute Adresse zeigt in einem Archiv auf dessen Wurzel und wird
	// deshalb nicht am Verzeichnis der verweisenden Datei ausgerichtet.
	if strings.HasPrefix(adresse, "/") {
		return path.Clean(strings.TrimPrefix(adresse, "/"))
	}
	if verzeichnis != "" {
		adresse = path.Join(verzeichnis, adresse)
	}
	return path.Clean(adresse)
}

// verweisMerken hält den Verweis zusätzlich in page_links fest.
//
// Der Text trägt [[Titel]] und ist damit lesbar; die Zeile hier trägt die
// Kennung und überlebt deshalb das Umbenennen der Zielseite. Beides zu haben
// kostet eine Zeile und rettet das Wissensnetz.
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

// inhaltSchreiben legt den fertigen Inhalt in der Seite ab.
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

// tagsSetzen legt die Schlagworte aus dem Vorspann an und hängt sie an.
// Schlagworte gehören dem Konto, nicht der Seite -- ein vorhandenes wird
// wiederverwendet statt verdoppelt.
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

// beilagenNachtragen hängt an, worauf niemand verwiesen hat.
//
// Ein Archiv enthält oft Dateien, die in keinem Dokument vorkommen -- alte
// Fassungen, Bilder aus einem gelöschten Absatz, Anlagen. Sie beim Import
// fallen zu lassen wäre stiller Verlust; sie landen an der Seite ihres
// Verzeichnisses, wo sie auch im Archiv lagen.
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
			*warnungen = append(*warnungen, p+": Anhang nicht gespeichert")
			continue
		}
		anzahl++
	}
	return anzahl
}

// anhangAnlegen speichert Bytes als Anhang einer Seite. Dieselbe Reihenfolge
// wie beim Hochladen über den Browser: erst die Zeile, dann die Datei, und bei
// einem Fehler die Zeile wieder weg -- ein Anhang, den man anklicken kann und
// der ins Leere führt, ist schlimmer als keiner.
func (s *Server) anhangAnlegen(ctx context.Context, seiteID, uid, dateiname string, inhalt []byte) (string, error) {
	if dateiname == "" || dateiname == "." || dateiname == "/" {
		dateiname = "datei"
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

// darfInSpaceSchreiben prüft das Recht, in einer Ablage neue Seiten anzulegen.
// Dieselbe Stufenleiter wie überall: Eigentümer, Administrator, ein Recht
// 'schreiben' oder 'verwalten' -- oder eine Ablage, die für alle offen steht.
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
