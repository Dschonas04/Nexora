// Wie breit der Text einer Seite steht.
//
// Der Satzspiegel war fest: 720 Pixel, gleich ob auf der Seite ein Merkzettel
// steht oder eine Tabelle mit zwoelf Spalten. Auf einem breiten Bildschirm
// blieb links und rechts eine Handbreit Papier leer, und die Tabelle brach
// trotzdem um.
//
// Der Wert haengt an der Seite und nicht am Konto: die Breite gehoert zum Satz
// des Textes wie eine Ueberschrift, und wer eine Tabellenseite oeffnet, soll
// sie so sehen, wie ihr Verfasser sie gesetzt hat.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// breiten sind die erlaubten Werte. Eine feste Liste und keine Zahl: hinter den
// Namen stehen Werte im Stilblatt, und eine freie Pixelangabe aus dem Browser
// waere eine Zahl, die niemand mehr prueft.
//
// Der leere Wert gehoert dazu: er heisst "wie die Instanz es vorgibt" und ist
// der Ausgangszustand jeder Seite. Ohne ihn waere jede Seite fuer immer auf das
// festgelegt, was beim Anlegen gerade Vorgabe war.
var breiten = map[string]bool{"": true, "normal": true, "breit": true, "voll": true}

type breiteReq struct {
	Breite string `json:"breite"`
}

// SetzeBreite aendert den Satzspiegel einer Seite. Wer schreiben darf, darf das
// auch: es ist eine Eigenschaft des Textes, keine der Freigabe.
func (s *Server) SetzeBreite(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	_, canEdit, _, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "Seite nicht gefunden")
		return
	}
	if !canEdit {
		writeErr(w, http.StatusForbidden, "nur mit Schreibrecht")
		return
	}

	var req breiteReq
	if err := decode(r, &req); err != nil || !breiten[req.Breite] {
		writeErr(w, http.StatusBadRequest, "erwartet normal, breit oder voll")
		return
	}

	// Ohne updated_at zu ruehren: die Breite ist keine Aenderung am Inhalt, und
	// eine neue Fassung im Verlauf waere fuer einen Handgriff am Satzspiegel zu
	// viel. Der offene Editor wuerde ausserdem seine Basis verlieren und beim
	// naechsten Speichern einen Konflikt melden, den es nicht gibt.
	if _, err := s.Pool.Exec(r.Context(),
		`UPDATE pages SET breite=$2 WHERE id=$1 AND deleted_at IS NULL`, id, req.Breite); err != nil {
		writeErr(w, http.StatusInternalServerError, "konnte nicht gespeichert werden")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"breite": req.Breite})
}
