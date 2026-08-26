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

// maxAnhangText limits what goes into the index. A tsvector holds roughly one
// megabyte; a manual of a thousand pages would burst the limit and make the
// INSERT fail.
const maxAnhangText = 400_000

// textAusAnhang returns the readable content. roh is the file stream, mime the
// reported type.
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

// ausPDF calls pdftotext.
//
// Extraction code of our own in Go would have managed without a foreign
// dependency, but fails on many real PDFs: font encodings, columns, embedded
// images. pdftotext is the yardstick such tools are measured against, and it
// sits in the image as poppler-utils.
//
// A scanned PDF without a text layer yields nothing. That is as it should be:
// that would need OCR, and OCR does not belong in the path of an upload.
func ausPDF(ctx context.Context, roh []byte) string {
	// A deadline of its own. A damaged PDF can keep pdftotext busy for an
	// arbitrarily long time, and a hanging upload is worse than an attachment
	// without full text.
	ctx, abbrechen := context.WithTimeout(ctx, 20*time.Second)
	defer abbrechen()

	// "-" for input and output: nothing is written to disk that would have to be
	// cleaned up afterwards.
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

// mitschnitt reads the stream and passes it on at the same time.
//
// The attachment has to run through the server anyway to land in the storage.
// Fetching it a second time afterwards to read it would be an avoidable round
// trip, especially with an object store, where it would go across the network.
//
// Buffered only up to the index limit; everything beyond flows through without
// being remembered. Otherwise a 200 MB file would sit in memory in full.
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
