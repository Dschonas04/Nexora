// How an uploaded file is delivered.

// The file's type is the claim the uploader made: it comes from the
// Content-Type of the form part and has since been stored in the column. If
// it were returned unchanged and marked "inline" then a page could contain an
// HTML or SVG file that the browser executes on the origin of this instance —
// including access to the viewer's session. A public reference would be
// sufficient for an outsider to trigger that.

// Therefore we restrict what can be shown inline: images, audio, video, PDF
// and plain text. Everything else is sent as a download with a type no browser
// will render. That is the tough side of the choice — a file you cannot view
// immediately is an annoyance; a file that executes code in the name of the
// instance is a breach.
package handlers

import (
	"net/http"
	"net/url"
	"strings"
)

// inlineTypen lists the mime types a browser may render inline.
//
// Note: image/svg+xml is intentionally excluded: an SVG is a document and may
// carry scripts, which makes it equivalent to an HTML file. Allowing it here
// would close the hole in one place and leave it open in another.
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

// reinerTyp extracts the base mime type from a header value: lowercase and
// without additions like "; charset=utf-8".
func reinerTyp(mime string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	return m
}

// darfInsFenster reports whether a mime type may be displayed inline. Audio
// and video are not listed explicitly but allowed by their top-level type:
// there are too many variants and none of them is a document.
func darfInsFenster(mime string) bool {
	m := reinerTyp(mime)
	if strings.HasPrefix(m, "audio/") || strings.HasPrefix(m, "video/") {
		return true
	}
	return inlineTypen[m]
}

// anhangKopf sets headers for a delivered file.
//
// X-Content-Type-Options: nosniff is not a replacement but a complement: it
// prevents the browser from overriding a harmless type, but does not help if
// the mime type itself is text/html. The allowed list above prevents that.
func anhangKopf(w http.ResponseWriter, mime, dateiname string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if darfInsFenster(mime) {
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", "inline"+namensteil(dateiname))
		return
	}

	// An SVG is both an image and a document. In an <img> it cannot execute
	// scripts, but requesting its URL directly could — and then on the origin
	// of this instance. We therefore serve it as an image but with a policy
	// that disables this case: `sandbox` removes scripts. The policy has no
	// effect when the browser embeds it as an <img>, because browsers do not
	// apply the policy in that context.
	if reinerTyp(mime) == "image/svg+xml" {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Disposition", "inline"+namensteil(dateiname))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		return
	}

	// A type no browser will render, together with an instruction to save the
	// file, and a policy that forbids everything in case any of it were
	// executed.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment"+namensteil(dateiname))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
}

// `namensteil` builds the filename part for the header twice: once for
// programs that only read ASCII, and once using RFC 5987 encoding for other
// clients — this ensures a filename like "Overview.pdf" remains readable
// when saved.
//
// Anything that does not belong in a header is removed first: quotes would
// terminate the value and line breaks would create extra header lines the
// caller did not intend.
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
