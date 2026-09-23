// Who can be addressed with an @ in a comment.
//
// Until now one had to hit an account's name letter for letter, or the mention
// went nowhere -- and silently at that: the comment stood there, the
// notification never arrived, and nobody found out. Whoever cannot recite
// their colleagues' names from memory could not use the feature.
//
// Hence a list to pick from. It holds exactly the accounts allowed to read
// this page: the others would get no notification anyway, and offering them
// would mean revealing in a dropdown who else has an account here.
package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nexora/internal/middleware"
)

// Person is an account as it appears in the dropdown: the name and nothing
// else. The interface needs no id, because a mention is the name in the text,
// and the address would be nobody's business.
type Person struct {
	Name string `json:"name"`
}

// lesendeKonten collects the accounts allowed to read a page.
//
// Against the instance's list of names rather than with a query over the
// permissions: the permissions live in four places (ownership, share, group,
// open space), and pagePerm is the only place that knows all four. For an
// instance of this size the loop is cheap; for ten thousand accounts it would
// be the wrong way round -- the same caveat as in erwaehnte().
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

// ErwaehnbarePersonen answers the question the comment column asks: whom can I
// address here? Whoever may not read the page itself does not get the list
// either.
func (s *Server) ErwaehnbarePersonen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")
	if canRead, _, _, ok := s.pagePerm(r.Context(), uid, id); !ok || !canRead {
		writeErr(w, http.StatusNotFound, "Seite nicht gefunden")
		return
	}
	writeJSON(w, http.StatusOK, s.lesendeKonten(r.Context(), id))
}
