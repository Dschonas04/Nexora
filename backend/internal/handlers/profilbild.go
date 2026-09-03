// Das Bild am eigenen Konto.
//
// In der Datenbank und nicht in der Ablage, wo die Anhänge liegen. Zwei Gründe:
// Anhänge sind ein kostenpflichtiger Zusatz, und ein Gesicht am eigenen Konto
// darf nicht davon abhängen, ob jemand einen Schlüssel gekauft hat. Und die
// Größe passt: die Oberfläche rechnet vor dem Hochladen auf 256 Pixel herunter,
// was wenige Zehn-Kilobyte ergibt. Für zweihundert Konten sind das ein paar
// Megabyte in einer Datenbank, die ohnehin jede Seite enthält -- eine zweite
// Ablage dafür wäre mehr Verwaltung als Nutzen.
//
// Zugeschnitten wird im Browser und nicht hier. Der Dienst prüft nur, was
// ankommt: dass es wirklich ein Bild ist (nicht, dass es so heißt), und dass es
// klein bleibt.
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

// hoechstensBild ist reichlich für ein auf 256 Pixel gerechnetes Bild und knapp
// genug, dass niemand die Datenbank als Fotoalbum benutzt.
const hoechstensBild = 512 * 1024

// erlaubteBildarten. GIF ist dabei, weil ein alter Bestand welche enthält; WebP
// nicht, denn Go erkennt es ohne fremdes Paket nicht, und ein Format, das der
// Dienst nicht prüfen kann, soll er auch nicht annehmen.
var erlaubteBildarten = map[string]string{
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
}

// ProfilbildSetzen nimmt die rohen Bytes entgegen.
func (s *Server) ProfilbildSetzen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	r.Body = http.MaxBytesReader(w, r.Body, hoechstensBild)
	roh, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("das Bild ist zu groß, höchstens %d KB", hoechstensBild/1024))
		return
	}
	// Nur die leere Anfrage bekommt hier einen eigenen Satz. Eine Untergrenze in
	// Bytes wäre geraten: ein gültiges PNG von einem Pixel wiegt 69 Byte, und
	// jede Zahl darüber wiese ein echtes Bild ab. Ob es eines ist, entscheidet
	// gleich darunter der Leser und nicht die Länge.
	if len(roh) == 0 {
		writeErr(w, http.StatusBadRequest, "da kam kein Bild an")
		return
	}

	// Wirklich ein Bild, und was für eines: entschieden wird nach dem Inhalt und
	// nicht nach dem, was der Browser als Typ behauptet. Sonst legte ein
	// umbenanntes Skript sich als "image/png" in die Zeile und käme später mit
	// genau diesem Typ wieder heraus.
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

// ProfilbildWeg nimmt das Bild wieder fort. Danach zeigt die Oberfläche wieder
// die Anfangsbuchstaben.
func (s *Server) ProfilbildWeg(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE users SET bild=NULL, bild_mime='', bild_stand=NULL WHERE id=$1`, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "das Bild ließ sich nicht entfernen")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Profilbild gibt das Bild eines Kontos heraus.
//
// Jedes angemeldete Konto darf jedes sehen, und das ist Absicht: ein Gesicht
// neben einem Kommentar nützt nur, wenn es auch dann erscheint, wenn der
// Kommentar von jemandem stammt, mit dem man keine Ablage teilt. Mehr als Name
// und Bild gibt dieser Weg nicht heraus, und beides steht ohnehin an jedem
// Kommentar und in jeder Freigabeliste.
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
	// Lange Frist, aber nur für diesen Browser: das Bild wird über die Adresse
	// mit dem Stand darin geholt, und ein neues Bild bringt eine neue Adresse
	// mit. private, weil ein Zwischenspeicher unterwegs sonst Gesichter einer
	// Instanz an eine andere Sitzung ausliefern könnte.
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
