// What a visitor without an account can see.
//
// Previously a publicly shared page contained only its text. Images linked to
// /api/pages/<id>/attachments/<id>, and that path requires a session: the
// visitor received 401 and thus broken images. Sharing a page with images
// therefore shared its holes.
//
// This adds a dedicated path for files of a shared page and rewrites the
// addresses in the content accordingly. It also removes page IDs from the
// responses: previously the page ID appeared in every image URL.
package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// oeffentlicheDateien builds the URL under which files of a shared page are
// served. It is a function rather than a repeated string: the path appears
// both in the router and here and both must mean the same thing.
func oeffentlicheDateien(token string) string { return "/api/public/" + token + "/dateien/" }

// adressenOeffnen rewrites image and file addresses of a page to the public
// path.
//
// Intentionally performed as a raw JSON string replacement rather than by
// traversing the tree: the URL may appear in `props.url`, in a caption, in a
// link, or in data added by other block types. The searched text contains the
// page ID and is therefore specific enough to not match unrelated content.
//
// Only files of THIS page are rewritten: a block that references an attachment
// of another page remains untouched and locked. Exposing it would amount to
// sharing a second page implicitly.
func adressenOeffnen(inhalt json.RawMessage, seitenID, token string) json.RawMessage {
	alt := "/api/pages/" + seitenID + "/attachments/"
	if !strings.Contains(string(inhalt), alt) {
		return inhalt
	}
	return json.RawMessage(strings.ReplaceAll(string(inhalt), alt, oeffentlicheDateien(token)))
}

// OeffentlicheDatei serves an attachment of a shared page to anyone with the
// reference.
//
// The authorization is the same as for the page itself: token and `is_public`,
// and the attachment must belong to exactly that page. If the share is revoked
// the images become inaccessible immediately.
func (s *Server) OeffentlicheDatei(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	attID := chi.URLParam(r, "attId")

	var filename, mime string
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT a.filename, a.mime
		   FROM attachments a
		   JOIN pages p ON p.id = a.page_id
		  WHERE a.id = $1 AND p.public_token = $2 AND p.is_public = true AND p.deleted_at IS NULL`,
		attID, token).Scan(&filename, &mime); err != nil {
		writeErr(w, http.StatusNotFound, "nicht gefunden")
		return
	}

	f, err := s.Ablage.Lesen(r.Context(), attID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Datei fehlt")
		return
	}
	defer f.Close()

	// Ueber anhangKopf und nicht von Hand: der Typ ist die Behauptung dessen,
	// der die Datei hochgeladen hat. Unveraendert und "inline" ausgeliefert
	// waere eine HTML- oder SVG-Datei ein Dokument auf dem Ursprung dieser
	// Instanz -- und hier reichte dafuer ein Fremder mit dem Verweis.
	anhangKopf(w, mime, filename)
	// Kein Suchmaschinenfutter: eine geteilte Seite ist nicht veroeffentlicht,
	// sie ist weitergegeben.
	w.Header().Set("X-Robots-Tag", "noindex")
	io.Copy(w, f)
}
