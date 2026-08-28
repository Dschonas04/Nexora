package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Ein Bild darf im Fenster stehen -- daran haengt die Vorschau und das Bild im
// Text.
func TestBildBleibtInline(t *testing.T) {
	w := httptest.NewRecorder()
	anhangKopf(w, "image/png", "Plan.png")
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Typ: %q", got)
	}
	if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline") {
		t.Fatalf("Anordnung: %q", got)
	}
}

// Eine HTML-Datei darf NICHT als Dokument ausgeliefert werden. Der Typ ist die
// Behauptung dessen, der sie hochgeladen hat; unveraendert und inline waere sie
// Programmcode auf dem Ursprung dieser Instanz, mit Zugriff auf die Sitzung
// dessen, der sie anschaut.
func TestHTMLWirdNichtAngezeigt(t *testing.T) {
	for _, typ := range []string{"text/html", "text/html; charset=utf-8", "TEXT/HTML", "application/xhtml+xml"} {
		w := httptest.NewRecorder()
		anhangKopf(w, typ, "boese.html")
		if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
			t.Fatalf("%s: Typ %q", typ, got)
		}
		if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
			t.Fatalf("%s: Anordnung %q", typ, got)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s: kein Regelwerk gesetzt", typ)
		}
	}
}

// nosniff steht immer da: sonst überstimmt der Browser einen harmlosen Typ
// anhand des Inhalts.
func TestNosniffImmer(t *testing.T) {
	w := httptest.NewRecorder()
	anhangKopf(w, "image/png", "Plan.png")
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("nosniff fehlt")
	}
}

// Ton, Video und PDF bleiben sichtbar: sie sind keine Dokumente, und an ihnen
// hängt die Schnellansicht.
func TestMedienBleibenSichtbar(t *testing.T) {
	for _, typ := range []string{"application/pdf", "audio/mpeg", "video/mp4", "text/plain"} {
		w := httptest.NewRecorder()
		anhangKopf(w, typ, "datei")
		if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline") {
			t.Fatalf("%s: %q", typ, got)
		}
	}
}

// Was in einem Kopf nichts zu suchen hat, kommt nicht hinein: ein
// Anführungszeichen beendet die Angabe, ein Zeilenumbruch wäre eine Zeile mehr,
// als der Aufrufer im Sinn hatte.
func TestNameWirdEntschaerft(t *testing.T) {
	w := httptest.NewRecorder()
	anhangKopf(w, "image/png", "a\"; x=1\r\nSet-Cookie: b=c\n.png")
	got := w.Header().Get("Content-Disposition")
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("Umbruch im Kopf: %q", got)
	}
	if strings.Count(got, `"`) != 2 {
		t.Fatalf("Anführungszeichen im Namen: %q", got)
	}
}

// Ein Name aus lauter Steuerzeichen lässt gar keinen Namen übrig -- dann steht
// nur die Anordnung da, statt eines leeren Namens.
func TestLeererNameFaelltWeg(t *testing.T) {
	w := httptest.NewRecorder()
	anhangKopf(w, "application/zip", "\x01\x02")
	if got := w.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Anordnung: %q", got)
	}
}

// Ein SVG bleibt ein Bild -- sonst zeigte die Vorschau ein Loch --, aber mit
// einem Regelwerk, das ihm die Skripte nimmt, falls jemand seine Adresse
// unmittelbar aufruft.
func TestSVGBleibtBildOhneSkripte(t *testing.T) {
	w := httptest.NewRecorder()
	anhangKopf(w, "image/svg+xml", "zeichnung.svg")
	if got := w.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("Typ: %q", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Fatalf("Regelwerk: %q", got)
	}
}
