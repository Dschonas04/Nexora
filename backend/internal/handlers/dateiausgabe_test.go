package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// An image may be shown inline — this is needed for previews and inline
// images in text.
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

// An HTML file must NOT be served as a document. The content type is the
// uploader's claim; returned unchanged and inline it would be executable code
// on the origin of this instance with access to the viewer's session.
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

// `nosniff` is always present: otherwise the browser may override a harmless
// declared type based on content sniffing.
func TestNosniffImmer(t *testing.T) {
	w := httptest.NewRecorder()
	anhangKopf(w, "image/png", "Plan.png")
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("nosniff fehlt")
	}
}

// Audio, video and PDF remain viewable: they are not documents and are used
// by the quick view feature.
func TestMedienBleibenSichtbar(t *testing.T) {
	for _, typ := range []string{"application/pdf", "audio/mpeg", "video/mp4", "text/plain"} {
		w := httptest.NewRecorder()
		anhangKopf(w, typ, "datei")
		if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline") {
			t.Fatalf("%s: %q", typ, got)
		}
	}
}

// Anything that does not belong in a header is removed: a quote would end
// the value and a line break would create an extra header line the caller did
// not intend.
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

// A name consisting solely of control characters yields no filename — in
// that case only the disposition remains, instead of an empty filename.
func TestLeererNameFaelltWeg(t *testing.T) {
	w := httptest.NewRecorder()
	anhangKopf(w, "application/zip", "\x01\x02")
	if got := w.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Anordnung: %q", got)
	}
}

// An SVG remains an image — otherwise the preview would show a hole — but it
// is returned with a policy that removes scripts in case someone requests the
// file directly.
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
