// Die Dateien, die zu einem Export gehoeren.
//
// Ein Ausgang aus dem System, der die Bilder zuruecklaesst, ist keiner. Bisher
// enthielt das Archiv nur Markdown, und darin standen Adressen der Form
// /api/pages/<id>/attachments/<id>: Verweise, die ausserhalb dieser Instanz auf
// nichts zeigen. Wer sein Wiki verlassen wollte, nahm den Text mit und liess
// die Bilder da.
//
// Also wandern die Dateien in einen Ordner des Archivs, und die Adressen im
// Text zeigen dorthin. Das Archiv ist damit fuer sich lesbar, in jedem
// Markdown-Betrachter und auch in zehn Jahren noch.
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

// dateiOrdner ist der Ordner im Archiv, in dem die Anhaenge liegen.
const dateiOrdner = "dateien"

// exportDatei ist ein Anhang, so wie er im Archiv landet.
type exportDatei struct {
	ID     string
	Seite  string
	Name   string // der Name, den die Datei beim Hochladen hatte
	Pfad   string // wo sie im Archiv liegt, samt Ordner
	Groess int64
}

// anhaengeSammeln liest die Anhaenge der ausgegebenen Seiten und legt fuer jeden
// einen eindeutigen Platz im Archiv fest.
//
// Zwei Seiten duerfen dieselbe Datei "Plan.png" tragen; im Archiv koennen sie
// nicht denselben Namen haben. Der Zaehler haengt eine Zahl an, statt die zweite
// still zu ueberschreiben -- dieselbe Regel wie bei den Seiten.
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

// eindeutigerDateiname haengt bei einem schon vergebenen Namen eine Zahl an,
// und zwar VOR der Endung: "Plan-2.png" und nicht "Plan.png-2", sonst weiss
// kein Programm mehr, was fuer eine Datei das ist.
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

// adressenAufDateien schreibt die Anhangadressen im Markdown auf die Pfade im
// Archiv um.
//
// Nur die Anhaenge der ausgegebenen Seiten: was auf eine Seite zeigt, die nicht
// mitkommt, bleibt stehen wie es ist. Ein Verweis, der ins Leere zeigt, ist
// ehrlicher als einer, der auf eine Datei zeigt, die im Archiv fehlt.
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

// anhangListe haengt an eine Seite die Dateien an, die im Text nicht vorkommen.
//
// Ein Bild steht im Text und ist damit erwaehnt; ein Anhang aus der Spalte
// daneben steht nirgends. Ohne diese Liste laege er im Archiv, ohne dass ihn
// jemals jemand fände.
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

// dateienSchreiben legt die Anhaenge ins Archiv.
//
// Der Strom geht von der Ablage direkt in das ZIP, ohne Zwischenspeicher: eine
// Ablage mit ein paar hundert Bildern laege sonst zweimal im Arbeitsspeicher.
// Eine Datei, die nicht mehr da ist, wird uebersprungen und nicht zum Fehler --
// das Archiv ist dann unvollstaendig, aber es entsteht, und die fehlende Datei
// war ohnehin schon weg.
func (s *Server) dateienSchreiben(ctx context.Context, zw *zip.Writer, dateien []exportDatei) {
	for _, d := range dateien {
		f, err := s.Ablage.Lesen(ctx, d.ID)
		if err != nil {
			continue
		}
		// Deflate bringt bei einem JPEG nichts und kostet Zeit; gespeichert wird
		// es darum nur bei den Formaten, die noch etwas hergeben.
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
