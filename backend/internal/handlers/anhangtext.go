// Pulling readable text out of an attachment, for the search index.
//
// Only what can be read cheaply and reliably: plain text straight from the
// stream, PDFs through pdftotext. Everything else contributes its filename and
// nothing more, which is honest, a search that pretends to look inside a ZIP
// and finds nothing is worse than one that never claimed to.
//
// Extraction never fails an upload. A file that cannot be read is still a file;
// it simply will not be found by its contents.
package handlers

import (
	"bytes"
	"context"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// maxAnhangText begrenzt, was in den Index geht. Ein tsvector fasst rund ein
// Megabyte; ein Handbuch mit tausend Seiten würde die Grenze sprengen und das
// INSERT scheitern lassen.
const maxAnhangText = 400_000

// textAusAnhang liefert den lesbaren Inhalt. roh ist der Dateistrom, mime der
// gemeldete Typ.
func textAusAnhang(ctx context.Context, roh []byte, mime, dateiname string) string {
	switch {
	case strings.HasPrefix(mime, "text/"),
		mime == "application/json",
		mime == "application/xml",
		mime == "application/x-yaml":
		return kuerzen(string(roh))

	case mime == "application/pdf" || strings.HasSuffix(strings.ToLower(dateiname), ".pdf"):
		return kuerzen(ausPDF(ctx, roh))
	}
	return ""
}

// ausPDF ruft pdftotext auf.
//
// Ein eigener Auslesecode in Go wäre ohne fremde Abhängigkeit ausgekommen,
// scheitert aber an vielen echten PDFs, Schriftkodierungen, Spalten,
// eingebettete Bilder. pdftotext ist der Maßstab, an dem sich solche Werkzeuge
// messen, und liegt als poppler-utils im Abbild.
//
// Ein gescanntes PDF ohne Textebene liefert nichts. Das ist richtig so: dafür
// bräuchte es Texterkennung, und die gehört nicht in den Weg eines Uploads.
func ausPDF(ctx context.Context, roh []byte) string {
	// Eigene Frist. Ein beschädigtes PDF kann pdftotext beliebig lange
	// beschäftigen, und ein hängender Upload ist schlimmer als ein Anhang
	// ohne Volltext.
	ctx, abbrechen := context.WithTimeout(ctx, 20*time.Second)
	defer abbrechen()

	// "-" für Eingabe und Ausgabe: nichts wird auf die Platte geschrieben, was
	// sonst aufgeräumt werden müsste.
	cmd := exec.CommandContext(ctx, "pdftotext", "-q", "-enc", "UTF-8", "-", "-")
	cmd.Stdin = bytes.NewReader(roh)
	var aus, fehler bytes.Buffer
	cmd.Stdout = &aus
	cmd.Stderr = &fehler

	if err := cmd.Run(); err != nil {
		// Only note it, do not pass it on: the upload succeeded long ago.
		log.Printf("PDF-Text: %v (%s)", err, strings.TrimSpace(fehler.String()))
		return aus.String() // whatever arrived until then beats nothing
	}
	return aus.String()
}

// kuerzen normalises whitespace and cuts to the index limit.
func kuerzen(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxAnhangText {
		s = s[:maxAnhangText]
		// After the cut a multi-byte character may be split in half.
		for len(s) > 0 && !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// mitschnitt liest den Strom und gibt ihn zugleich weiter.
//
// Der Anhang muss ohnehin durch den Server laufen, um in der Ablage zu landen.
// Ihn danach zum Auslesen ein zweites Mal zu holen wäre eine vermeidbare Runde
// gerade beim Objektspeicher, wo das über das Netz ginge.
//
// Gepuffert wird nur bis zur Indexgrenze; alles darüber fließt durch, ohne
// gemerkt zu werden. Sonst läge eine 200-MB-Datei komplett im Arbeitsspeicher.
type mitschnitt struct {
	quelle io.Reader
	puffer bytes.Buffer
}

func (m *mitschnitt) Read(p []byte) (int, error) {
	n, err := m.quelle.Read(p)
	if n > 0 && m.puffer.Len() < maxAnhangText {
		rest := maxAnhangText - m.puffer.Len()
		if n < rest {
			rest = n
		}
		m.puffer.Write(p[:rest])
	}
	return n, err
}

func (m *mitschnitt) Bytes() []byte { return m.puffer.Bytes() }
