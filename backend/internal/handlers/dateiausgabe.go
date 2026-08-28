// Wie eine hochgeladene Datei ausgeliefert wird.
//
// Der Typ einer Datei ist das, was der Hochladende behauptet hat: er kommt aus
// dem Content-Type des Formularteils und steht seitdem in der Spalte. Wurde er
// unveraendert wieder ausgeliefert, und dazu als "inline", dann liess sich eine
// Seite mit einer HTML- oder SVG-Datei bestuecken, die der Browser auf dem
// Ursprung dieser Instanz als Dokument ausfuehrt -- samt Zugriff auf die
// Sitzung dessen, der sie anschaut. Ueber einen oeffentlichen Verweis genuegte
// dafuer ein Fremder mit dem Link.
//
// Darum eine Liste dessen, was im Fenster stehen darf: Bilder, Ton, Video, PDF
// und schlichter Text. Alles andere geht als Download hinaus, mit einem Typ,
// den kein Browser auslegt. Das ist die haerte Seite der Wahl -- eine Datei,
// die man nicht sofort sieht, ist ein Aergernis; eine Datei, die im Namen der
// Instanz Programmcode ausfuehrt, ist ein Einbruch.
package handlers

import (
	"net/http"
	"net/url"
	"strings"
)

// inlineTypen sind die Typen, die der Browser anzeigen darf.
//
// image/svg+xml steht mit Absicht NICHT darin: ein SVG ist ein Dokument, es
// darf Skripte tragen und ist damit dasselbe wie eine HTML-Datei. Es hier
// aufzunehmen hiesse, das Loch an einer Stelle zu schliessen und an der
// naechsten offen zu lassen.
var inlineTypen = map[string]bool{
	"image/png":                true,
	"image/jpeg":               true,
	"image/gif":                true,
	"image/webp":               true,
	"image/avif":               true,
	"image/bmp":                true,
	"image/tiff":               true,
	"image/x-icon":             true,
	"image/vnd.microsoft.icon": true,
	"application/pdf":          true,
	"text/plain":               true,
}

// reinerTyp schaelt den Typ aus der Angabe: klein geschrieben und ohne
// Zusaetze wie "; charset=utf-8".
func reinerTyp(mime string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	return m
}

// darfInsFenster sagt, ob ein Typ angezeigt werden darf. Ton und Video stehen
// nicht in der Liste, sondern werden ueber ihre Gattung erlaubt: davon gibt es
// zu viele Schreibweisen, und keine von ihnen ist ein Dokument.
func darfInsFenster(mime string) bool {
	m := reinerTyp(mime)
	if strings.HasPrefix(m, "audio/") || strings.HasPrefix(m, "video/") {
		return true
	}
	return inlineTypen[m]
}

// anhangKopf setzt Typ und Anordnung fuer eine ausgelieferte Datei.
//
// nosniff ist dabei kein Ersatz, sondern eine Ergaenzung: es hindert den
// Browser daran, einen harmlosen Typ zu ueberstimmen, hilft aber nicht, wenn
// der Typ selbst schon text/html lautet. Das verhindert erst die Liste oben.
func anhangKopf(w http.ResponseWriter, mime, dateiname string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if darfInsFenster(mime) {
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", "inline"+namensteil(dateiname))
		return
	}

	// Ein SVG ist beides: ein Bild und ein Dokument. In einem <img> kann es
	// keine Skripte ausfuehren, ruft man seine Adresse aber unmittelbar auf,
	// schon -- und dann auf dem Ursprung dieser Instanz. Es geht darum als Bild
	// hinaus, aber mit einem Regelwerk, das genau diesen Fall stillegt: sandbox
	// nimmt dem Dokument die Skripte, und fuer die Einbindung als Bild ist das
	// Regelwerk ohne Belang, weil ein Browser es dort gar nicht erst anwendet.
	if reinerTyp(mime) == "image/svg+xml" {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Disposition", "inline"+namensteil(dateiname))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		return
	}

	// Ein Typ, den kein Browser auslegt, dazu die Anweisung zu speichern, und
	// ein Regelwerk, das dem Dokument alles verbietet, falls doch einmal etwas
	// davon ausgeführt würde.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment"+namensteil(dateiname))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
}

// namensteil baut den Dateinamen fuer den Kopf, zweimal: einmal fuer Programme,
// die nur ASCII lesen, und einmal nach RFC 5987 fuer alle anderen -- damit eine
// "Übersicht.pdf" auch beim Speichern eine Übersicht bleibt.
//
// Was in einem Kopf nichts zu suchen hat, faellt vorher heraus:
// Anfuehrungszeichen beenden die Angabe, Zeilenumbrueche waeren eine Zeile
// mehr, als der Aufrufer im Sinn hatte.
func namensteil(dateiname string) string {
	var b strings.Builder
	for _, r := range dateiname {
		if r < 32 || r == 127 || r == '"' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	name := strings.TrimSpace(b.String())
	if name == "" {
		return ""
	}
	return `; filename="` + nurASCII(name) + `"; filename*=UTF-8''` + url.PathEscape(name)
}
