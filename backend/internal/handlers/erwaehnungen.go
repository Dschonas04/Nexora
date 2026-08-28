// Wen man in einem Kommentar mit @ ansprechen kann.
//
// Bisher musste man den Namen eines Kontos auf den Buchstaben genau treffen,
// sonst ging die Erwaehnung ins Leere -- und zwar still: der Kommentar stand
// da, die Benachrichtigung kam nie an, und niemand erfuhr davon. Wer die Namen
// der Kollegen nicht auswendig kann, konnte die Funktion nicht benutzen.
//
// Darum eine Liste zum Auswaehlen. Sie enthaelt genau die Konten, die diese
// Seite lesen duerfen: die anderen bekaemen ohnehin keine Nachricht, und sie
// anzubieten hiesse, in der Auswahlliste zu verraten, wer sonst noch ein Konto
// hat.
package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// Person ist ein Konto, so wie es in der Auswahlliste steht: nur der Name. Die
// Kennung braucht die Oberflaeche nicht, denn eine Erwaehnung ist der Name im
// Text, und die Adresse ginge niemanden etwas an.
type Person struct {
	Name string `json:"name"`
}

// lesendeKonten sammelt die Konten, die eine Seite lesen duerfen.
//
// Gegen die Namensliste der Instanz und nicht mit einer Abfrage ueber die
// Rechte: die Rechte stehen an vier Stellen (Eigentum, Freigabe, Gruppe,
// offene Ablage), und pagePerm ist die einzige Stelle, die sie alle vier
// kennt. Fuer eine Instanz dieser Groesse ist die Schleife billig; fuer
// zehntausend Konten waere das der falsche Weg -- derselbe Vorbehalt wie bei
// erwaehnte().
func (s *Server) lesendeKonten(ctx context.Context, pageID string) []Person {
	liste := []Person{}
	rows, err := s.Pool.Query(ctx, `SELECT id::text, name FROM users WHERE name <> '' ORDER BY name`)
	if err != nil {
		return liste
	}
	defer rows.Close()

	type konto struct{ id, name string }
	var alle []konto
	for rows.Next() {
		var k konto
		if rows.Scan(&k.id, &k.name) == nil {
			alle = append(alle, k)
		}
	}
	for _, k := range alle {
		if canRead, _, _, ok := s.pagePerm(ctx, k.id, pageID); ok && canRead {
			liste = append(liste, Person{Name: k.name})
		}
	}
	return liste
}

// ErwaehnbarePersonen beantwortet die Frage der Kommentarspalte: wen kann ich
// hier ansprechen? Wer die Seite selbst nicht lesen darf, bekommt auch die
// Liste nicht.
func (s *Server) ErwaehnbarePersonen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if canRead, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok || !canRead {
		writeErr(w, http.StatusNotFound, "Seite nicht gefunden")
		return
	}
	writeJSON(w, http.StatusOK, s.lesendeKonten(r.Context(), id))
}
