// Moving and ordering: where a page hangs in the tree, and in which order
// pages and spaces stand in the sidebar.
//
// Moving alone was already possible through UpdatePage; what was missing is the
// order among siblings. Both live in one call here, because a drag in the
// sidebar is one gesture and would otherwise need two requests that can half
// fail.
//
// The order is not computed from fractions between neighbours but written out
// completely: the sibling list is read, the moved entry is taken out and put
// back in at its place, and every row gets its index. That costs one UPDATE per
// sibling instead of one in total, which at the size of a level in a wiki is
// nothing, and in return there is no drift to renormalise later.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexora/internal/lizenz"
	"nexora/internal/middleware"
)

type seiteVerschiebenReq struct {
	// Absent means unchanged, null means the top level or no space. The same
	// convention as UpdatePage, so a caller that knows one knows the other.
	ElternID json.RawMessage `json:"elternId"`
	SpaceID  json.RawMessage `json:"spaceId"`
	// VorID places the page directly in front of this sibling. Empty or absent
	// means: at the end.
	VorID *string `json:"vorId"`
}

// SeiteVerschieben hangs a page somewhere else and puts it at a place among its
// new siblings.
func (s *Server) SeiteVerschieben(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id := chi.URLParam(r, "id")

	_, _, isOwner, ok := s.pagePerm(r.Context(), uid, id)
	if !ok {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}
	// Same rule as in UpdatePage: rearranging the tree belongs to whoever owns
	// it. Write access to the content is not write access to the structure --
	// otherwise a page could be pulled out of the tree its owner sees.
	if !isOwner && !s.isAdmin(r.Context(), uid) {
		writeErr(w, http.StatusForbidden, "nur der Eigentümer darf umhängen")
		return
	}

	cur, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "page not found")
		return
	}

	var req seiteVerschiebenReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	eltern := cur.ParentID
	if len(req.ElternID) > 0 {
		var pid *string
		if err := json.Unmarshal(req.ElternID, &pid); err != nil {
			writeErr(w, http.StatusBadRequest, "elternId unlesbar")
			return
		}
		eltern = pid
	}
	space := cur.SpaceID
	if len(req.SpaceID) > 0 {
		var sid *string
		if err := json.Unmarshal(req.SpaceID, &sid); err != nil {
			writeErr(w, http.StatusBadRequest, "spaceId unlesbar")
			return
		}
		space = sid
	}

	if eltern != nil {
		if *eltern == id {
			writeErr(w, http.StatusBadRequest, "eine Seite kann nicht unter sich selbst hängen")
			return
		}
		// A page dropped into its own subtree would take that branch out of the
		// tree: it would hang below itself and be reachable from nowhere. The
		// interface already refuses it, but the interface is not the guard.
		unter, err := s.istNachfahre(r.Context(), *eltern, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "Baum nicht lesbar")
			return
		}
		if unter {
			writeErr(w, http.StatusBadRequest, "eine Seite kann nicht unter eine ihrer Unterseiten")
			return
		}
		// A subpage lives in the space of its parent. Letting the two drift apart
		// would show the page in one space and its parent in another.
		if len(req.SpaceID) == 0 {
			if err := s.Pool.QueryRow(r.Context(),
				`SELECT space_id FROM pages WHERE id=$1`, *eltern).Scan(&space); err != nil {
				writeErr(w, http.StatusBadRequest, "Zielseite nicht gefunden")
				return
			}
		}
	}

	if err := s.seiteEinsortieren(r.Context(), cur.OwnerID, id, eltern, space, req.VorID); err != nil {
		writeErr(w, http.StatusInternalServerError, "verschieben fehlgeschlagen")
		return
	}

	s.spurAusRequest(r, AktSeiteGeaendert, "seite", id, cur.Title,
		map[string]any{"verschoben": true})

	page, err := s.loadPage(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// seiteEinsortieren writes parent, space and the order of the whole target
// level in one transaction. Halfway through would mean a page that hangs in the
// new place but stands in the old order.
func (s *Server) seiteEinsortieren(ctx context.Context, besitzer, id string,
	eltern, space, vor *string) error {

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE pages SET parent_id=$2, space_id=$3, updated_at=now() WHERE id=$1`,
		id, eltern, space); err != nil {
		return err
	}

	// The whole subtree moves along into the space. A subpage belongs where its
	// parent belongs; if only the top row were rewritten, the children would
	// still count as being in the old space -- invisible in the sidebar, which
	// draws the tree from parent_id alone, but plain to see in the search filter,
	// in the space export and in the colours of the graph.
	if _, err := tx.Exec(ctx,
		`WITH RECURSIVE unten AS (
		     SELECT id FROM pages WHERE parent_id = $1
		     UNION ALL
		     SELECT p.id FROM pages p JOIN unten u ON p.parent_id = u.id
		 )
		 UPDATE pages SET space_id=$2 WHERE id IN (SELECT id FROM unten)`,
		id, space); err != nil {
		return err
	}

	// The siblings of the new place. At the top level the space decides who
	// stands beside whom; below it the parent alone does, because a subpage
	// always shares its parent's space.
	//
	// Only the owner's own pages: the order is written to their rows, and a
	// foreign page in the same space is not this user's to renumber.
	var rows pgx.Rows
	if eltern == nil {
		rows, err = tx.Query(ctx,
			`SELECT id FROM pages
			  WHERE owner_id=$1 AND deleted_at IS NULL AND parent_id IS NULL
			    AND space_id IS NOT DISTINCT FROM $2
			  ORDER BY sort_order, created_at`, besitzer, space)
	} else {
		rows, err = tx.Query(ctx,
			`SELECT id FROM pages
			  WHERE owner_id=$1 AND deleted_at IS NULL AND parent_id=$2
			  ORDER BY sort_order, created_at`, besitzer, *eltern)
	}
	if err != nil {
		return err
	}
	var geschwister []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err == nil {
			geschwister = append(geschwister, sid)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	geordnet := einsortieren(geschwister, id, vor)
	for platz, sid := range geordnet {
		if _, err := tx.Exec(ctx,
			`UPDATE pages SET sort_order=$2 WHERE id=$1`, sid, float64(platz)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// einsortieren takes id out of the list and puts it back in front of vor. An
// unknown vor, and an absent one, both mean the end -- a drop onto empty space
// below the last entry should append rather than fail.
func einsortieren(liste []string, id string, vor *string) []string {
	// Dropped in front of itself: that is the gap the entry already stands in,
	// so nothing changes. Without this the entry would be taken out, looked for
	// in vain and land at the end -- a gesture that means "leave it" would move
	// the page.
	if vor != nil && *vor == id {
		return append([]string(nil), liste...)
	}
	ohne := make([]string, 0, len(liste)+1)
	for _, e := range liste {
		if e != id {
			ohne = append(ohne, e)
		}
	}
	stelle := len(ohne)
	if vor != nil && *vor != "" {
		for i, e := range ohne {
			if e == *vor {
				stelle = i
				break
			}
		}
	}
	neu := make([]string, 0, len(ohne)+1)
	neu = append(neu, ohne[:stelle]...)
	neu = append(neu, id)
	neu = append(neu, ohne[stelle:]...)
	return neu
}

// istNachfahre reports whether ziel sits below wurzel in the page tree. Walked
// upwards from ziel, because that is the shorter way: one row per level instead
// of a whole subtree.
func (s *Server) istNachfahre(ctx context.Context, ziel, wurzel string) (bool, error) {
	akt := ziel
	// A guard against a cycle that should not exist but would otherwise turn
	// this into an endless loop.
	for i := 0; i < 200; i++ {
		var eltern *string
		if err := s.Pool.QueryRow(ctx, `SELECT parent_id FROM pages WHERE id=$1`, akt).Scan(&eltern); err != nil {
			return false, err
		}
		if eltern == nil {
			return false, nil
		}
		if *eltern == wurzel {
			return true, nil
		}
		akt = *eltern
	}
	return false, nil
}

type spacesOrdnenReq struct {
	// The ids in the wanted order. Ids the caller cannot see are ignored rather
	// than refused: a list from a second tab may be a moment old, and that is no
	// reason to drop the whole gesture.
	IDs []string `json:"ids"`
}

// SpacesOrdnen writes the order of the sidebar for the calling account.
func (s *Server) SpacesOrdnen(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	var req spacesOrdnenReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "keine Reihenfolge angegeben")
		return
	}

	sichtbar, err := s.sichtbareSpaces(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}

	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer tx.Rollback(r.Context())

	platz := 0
	for _, id := range req.IDs {
		if !sichtbar[id] {
			continue
		}
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO space_reihenfolge (user_id, space_id, platz) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, space_id) DO UPDATE SET platz = EXCLUDED.platz`,
			uid, id, platz); err != nil {
			writeErr(w, http.StatusInternalServerError, "speichern fehlgeschlagen")
			return
		}
		platz++
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "speichern fehlgeschlagen")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sichtbareSpaces is the set of spaces the account may see -- the same reach as
// ListSpaces, only as a set for checking.
func (s *Server) sichtbareSpaces(ctx context.Context, uid string) (map[string]bool, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT sp.id FROM spaces sp
		  WHERE sp.owner_id = $1
		     OR sp.oeffentlich <> 'nein'
		     OR ($2 AND EXISTS (
		           SELECT 1 FROM space_rechte sr
		            WHERE sr.space_id = sp.id
		              AND (sr.user_id = $1
		                   OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
		                                       WHERE gm.user_id = $1))))`,
		uid, lizenz.Frei(lizenz.Gruppen))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	menge := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			menge[id] = true
		}
	}
	return menge, rows.Err()
}
