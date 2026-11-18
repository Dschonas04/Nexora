// Save an edited text attachment in place of the original.
//
// Scripts, Markdown, configuration files and logs open in the viewer's text
// editor. Saving sends the whole text back; it replaces the file under the
// same attachment ID, exactly like an annotated PDF does, so links from the
// page, comments and bookmarks keep pointing at it.
package handlers

import (
	"bytes"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

// maxTextBytes caps what the editor may write. Five megabytes of text is a
// very long log; the browser's text field gets sluggish well before that.
const maxTextBytes = 5 << 20

// textEndungen are the extensions the editor opens, whatever type the browser
// claimed on upload (a .sh arrives as application/x-sh, a .bat as
// application/x-msdos-program, many as application/octet-stream).
var textEndungen = map[string]bool{
	"txt": true, "md": true, "markdown": true, "log": true, "csv": true, "tsv": true,
	"json": true, "jsonc": true, "xml": true, "yml": true, "yaml": true, "toml": true,
	"ini": true, "cfg": true, "conf": true, "config": true, "env": true, "properties": true,
	"sh": true, "bash": true, "zsh": true, "fish": true, "ps1": true, "psm1": true, "psd1": true,
	"bat": true, "cmd": true, "vbs": true,
	"py": true, "rb": true, "pl": true, "php": true, "lua": true, "r": true,
	"go": true, "rs": true, "c": true, "h": true, "cpp": true, "hpp": true, "cc": true, "cs": true,
	"java": true, "kt": true, "kts": true, "swift": true, "scala": true, "dart": true,
	"js": true, "mjs": true, "cjs": true, "ts": true, "tsx": true, "jsx": true,
	"css": true, "scss": true, "less": true, "html": true, "htm": true, "vue": true, "svelte": true,
	"sql": true, "graphql": true, "gql": true, "proto": true,
	"dockerfile": true, "makefile": true, "gitignore": true, "gitattributes": true, "editorconfig": true,
	"tf": true, "tfvars": true, "hcl": true, "nix": true, "gradle": true,
	"srt": true, "vtt": true, "tex": true, "bib": true, "rst": true, "adoc": true, "org": true,
	"pem": true, "crt": true, "csr": true, "pub": true,
}

// istTextDatei decides by type and file name whether an attachment is text.
func istTextDatei(mime, name string) bool {
	m := strings.ToLower(mime)
	if strings.HasPrefix(m, "text/") || m == "application/json" || m == "application/xml" ||
		m == "application/x-yaml" || m == "application/x-sh" || m == "application/javascript" {
		return true
	}
	basis := strings.ToLower(path.Base(strings.ReplaceAll(name, "\\", "/")))
	if textEndungen[basis] || textEndungen[strings.TrimPrefix(basis, ".")] {
		return true // Dockerfile, Makefile, .gitignore
	}
	if i := strings.LastIndex(basis, "."); i >= 0 {
		return textEndungen[basis[i+1:]]
	}
	return false
}

// TextErsetzen accepts the edited text.
func (s *Server) TextErsetzen(w http.ResponseWriter, r *http.Request) {
	if !lizenz.Frei(lizenz.Anhaenge) {
		writeErr(w, http.StatusPaymentRequired, "Anhänge sind ein Zusatz")
		return
	}
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	_, darf, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	if !darf {
		writeErr(w, http.StatusForbidden, "read-only access")
		return
	}
	attID := chi.URLParam(r, "attId")

	var name, mime string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT filename, mime FROM attachments WHERE id=$1 AND page_id=$2`, attID, id).
		Scan(&name, &mime); err != nil {
		writeErr(w, http.StatusNotFound, "attachment not found")
		return
	}
	if !istTextDatei(mime, name) {
		writeErr(w, http.StatusBadRequest, "das ist keine Textdatei")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTextBytes)
	roh, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "der Text ist zu lang")
		return
	}
	// Text in, text stored: anything that is not UTF-8 or carries NUL bytes is
	// a binary file in disguise and must not overwrite the original.
	if !utf8.Valid(roh) || bytes.IndexByte(roh, 0) >= 0 {
		writeErr(w, http.StatusBadRequest, "das ist kein gültiger Text")
		return
	}

	// The stored type stays, except the meaningless octet-stream: the next
	// download should name the file what it is.
	neuerTyp := mime
	if neuerTyp == "" || neuerTyp == "application/octet-stream" || neuerTyp == "binary/octet-stream" {
		neuerTyp = "text/plain; charset=utf-8"
	}
	if _, err := s.Ablage.Schreiben(r.Context(), attID, bytes.NewReader(roh),
		int64(len(roh)), neuerTyp); err != nil {
		writeErr(w, http.StatusInternalServerError, "Ablage nicht erreichbar")
		return
	}
	s.Pool.Exec(r.Context(), `UPDATE attachments SET size=$2, mime=$3 WHERE id=$1`, attID, len(roh), neuerTyp)

	// The search must find the new wording, not the old one.
	if lizenz.Frei(lizenz.Anhangsuche) {
		s.Pool.Exec(r.Context(), `UPDATE attachments SET inhalt_text=$2 WHERE id=$1`, attID, kuerzen(string(roh)))
	}

	s.spurAusRequest(r, AktAnhangBearbeitet, "anhang", attID, name,
		map[string]any{"bytes": len(roh), "art": "text"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(roh)})
}
