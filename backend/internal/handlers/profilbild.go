// Avatar image for a user account.
//
// Stored in the database rather than in the attachment storage. Reasons:
// attachments are a paid extra and avatar visibility must not depend on a
// purchased feature; avatars are also small (client resizes to 256px before
// upload), so a few hundred avatars fit comfortably in the DB without a
// separate storage system.
//
// Images are cropped client-side; the server only validates incoming bytes —
// that they are an actual image and within the size limit.
package handlers

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // nur wegen der Erkennung
	_ "image/jpeg" // dito
	_ "image/png"  // dito
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// hoechstensBild is generous for a 256px image and small enough to avoid
// treating the database as a photo album.
const hoechstensBild = 512 * 1024

// Allowed image formats. GIF is permitted for legacy content; WebP is not
// accepted because Go does not recognise it without an external package, and
// the server should not accept formats it cannot validate.
var erlaubteBildarten = map[string]string{
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
}

// ProfilbildSetzen accepts the raw image bytes.
func (s *Server) ProfilbildSetzen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	r.Body = http.MaxBytesReader(w, r.Body, hoechstensBild)
	roh, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("das Bild ist zu groß, höchstens %d KB", hoechstensBild/1024))
		return
	}
	// Only an empty request gets a bespoke error here. A byte-size lower bound
	// would be guesswork (a one-pixel PNG can be very small); actual image
	// detection is performed below using the decoder.
	if len(roh) == 0 {
		writeErr(w, http.StatusBadRequest, "da kam kein Bild an")
		return
	}

	// Verify the data is actually an image and determine its type by content,
	// not by a claimed Content-Type header. Otherwise a renamed script claiming
	// to be "image/png" could be accepted.
	_, art, err := image.DecodeConfig(bytes.NewReader(roh))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "das ließ sich nicht als Bild lesen")
		return
	}
	mime, ok := erlaubteBildarten[art]
	if !ok {
		writeErr(w, http.StatusUnsupportedMediaType,
			"dieses Bildformat wird nicht angenommen: "+art)
		return
	}

	jetzt := time.Now()
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET bild=$2, bild_mime=$3, bild_stand=$4 WHERE id=$1`,
		uid, roh, mime, jetzt); err != nil {
		writeErr(w, http.StatusInternalServerError, "das Bild ließ sich nicht speichern")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bildStand": jetzt})
}

// ProfilbildWeg removes the profile image. The UI will revert to showing
// initials after this.
func (s *Server) ProfilbildWeg(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET bild=NULL, bild_mime='', bild_stand=NULL WHERE id=$1`, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "das Bild ließ sich nicht entfernen")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Profilbild serves the image for a given account.
//
// Any signed-in account may view any avatar. This is intentional: an avatar
// next to a comment is only useful if it appears even for commentators with
// whom the viewer does not share a space. This endpoint only exposes name and
// image, which are already present on comments and share lists.
func (s *Server) Profilbild(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var roh []byte
	var mime string
	var stand *time.Time
	if err := s.Pool.QueryRow(r.Context(),
		`SELECT bild, coalesce(bild_mime, ''), bild_stand FROM users WHERE id=$1`, id).
		Scan(&roh, &mime, &stand); err != nil || len(roh) == 0 {
		// 404 und kein Platzhalterbild: welches Zeichen für ein Konto ohne Bild
		// steht, entscheidet die Oberfläche, nicht der Dienst.
		writeErr(w, http.StatusNotFound, "kein Bild")
		return
	}
	if mime == "" {
		mime = "application/octet-stream"
	}

	w.Header().Set("Content-Type", mime)
	// Long cache lifetime but private to the browser: the image is fetched
	// using a URL that encodes the image's timestamp, so a new upload yields a
	// new URL. `private` prevents shared caches from serving avatars from one
	// session to another.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(roh)
}

// ProfilAendern setzt den angezeigten Namen des eigenen Kontos.
//
// Der Name und nicht die Adresse: die Adresse ist die Kennung, an der Freigaben
// und die Anmeldung hängen, und die zu ändern ist Sache einer Verwaltung. Wie
// jemand heißen möchte, ist dagegen seine eigene Angelegenheit.
func (s *Server) ProfilAendern(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	var eingabe struct {
		Name string `json:"name"`
	}
	if err := decode(r, &eingabe); err != nil {
		writeErr(w, http.StatusBadRequest, "unlesbarer Rumpf")
		return
	}
	name := strings.TrimSpace(eingabe.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "ein Name wird gebraucht")
		return
	}
	// Eine Grenze, damit eine Namensspalte eine Spalte bleibt. Gezählt werden
	// Zeichen und nicht Bytes, sonst reichte ein Name mit Umlauten weniger weit
	// als einer ohne.
	if len([]rune(name)) > 80 {
		writeErr(w, http.StatusBadRequest, "der Name ist zu lang, höchstens 80 Zeichen")
		return
	}

	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET name=$2 WHERE id=$1`, uid, name); err != nil {
		writeErr(w, http.StatusInternalServerError, "der Name ließ sich nicht speichern")
		return
	}
	s.spurAusRequest(r, AktProfilGeaendert, "konto", uid, name, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
}
