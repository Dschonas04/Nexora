// Access control lives here. Both helpers hit the database on every call rather
// than caching, so a share revoked in one tab takes effect in the next request
// made from another.
package handlers

import (
	"context"

	"nexora/internal/lizenz"
)

// isAdmin reports whether the user has the admin role. Admins can read and
// edit every page in the workspace.
func (s *Server) isAdmin(ctx context.Context, uid string) bool {
	var role string
	if err := s.Pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, uid).Scan(&role); err != nil {
		return false
	}
	return role == "admin"
}

// pagePerm resolves a user's access to a non-deleted page.
//
//	canRead — owner, admin, page share, a right on the space it sits in, or a
//	          space that is open to the whole instance
//	canEdit — owner, admin, share with 'edit', 'schreiben'/'verwalten' on the
//	          space, or a space that is open for writing
//	isOwner — the user owns the page
//	ok      — the page exists, is not in the trash, and the user may read it
//
// Everything is resolved in ONE query. That matters more than it looks: this
// function runs on every request that touches a page, and it is the single
// place where "who may see what" is decided. Splitting it across several
// queries would invite the parts to drift, and a drifting permission check is
// how pages end up visible to the wrong people.
//
// Space rights only count while the Gruppen extra is licensed. Without it the
// rows may still exist, they are not deleted when a licence lapses, but
// they grant nothing, and access falls back to owner, admin and page shares.
func (s *Server) pagePerm(ctx context.Context, uid, pageID string) (canRead, canEdit, isOwner, ok bool) {
	gruppenAn := lizenz.Frei(lizenz.Gruppen)

	var ownerID string
	var admin bool
	var freigabe *string   // 'read' | 'edit' when the page is shared directly
	var spaceRecht *string // 'lesen' | 'schreiben' | 'verwalten', bestes Recht am Space

	err := s.Pool.QueryRow(ctx, `
		SELECT
			p.owner_id::text,
			(SELECT role = 'admin' FROM users WHERE id = $2),
			(SELECT sh.permission FROM page_shares sh
			  WHERE sh.page_id = p.id AND sh.user_id = $2),
			-- Bestes Recht am Space: einzeln vergeben, über eine Gruppe,
			-- oder daher, dass die Ablage öffentlich ist. Alle drei Wege
			-- landen im selben UNION und werden danach nach Stufe sortiert,
			-- damit der stärkste gewinnt. Die Reihenfolge im ORDER BY ist die
			-- Stufenleiter, nicht das Alphabet, 'lesen' käme sonst vor
			-- 'schreiben'.
			(SELECT x.recht FROM (
			   SELECT sr.recht FROM space_rechte sr
			    WHERE $3
			      AND sr.space_id = p.space_id
			      AND (sr.user_id = $2
			           OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
			                               WHERE gm.user_id = $2))
			   UNION ALL
			   -- Eine öffentliche Ablage gilt für jedes angemeldete Konto und
			   -- hängt an keinem Zusatzumfang: sie ist Grundausstattung.
			   SELECT CASE so.oeffentlich WHEN 'schreiben' THEN 'schreiben' ELSE 'lesen' END
			     FROM spaces so
			    WHERE so.id = p.space_id AND so.oeffentlich <> 'nein'
			 ) x
			  ORDER BY CASE x.recht
			             WHEN 'verwalten' THEN 3
			             WHEN 'schreiben' THEN 2
			             ELSE 1 END DESC
			  LIMIT 1)
		FROM pages p
		WHERE p.id = $1 AND p.deleted_at IS NULL`,
		pageID, uid, gruppenAn).Scan(&ownerID, &admin, &freigabe, &spaceRecht)
	if err != nil {
		return false, false, false, false
	}

	isOwner = ownerID == uid
	if isOwner || admin {
		return true, true, isOwner, true
	}
	if freigabe != nil {
		return true, *freigabe == "edit", false, true
	}
	if spaceRecht != nil {
		return true, *spaceRecht == "schreiben" || *spaceRecht == "verwalten", false, true
	}
	return false, false, false, false
}

// darfSpaceVerwalten reports whether the user may hand out rights on a space:
// its owner, an admin, or somebody who was given 'verwalten' on it.
//
// This is the space manager the role list never had. Deliberately a right on
// one space rather than a global role between user and admin: "may manage
// Marketing" is a sentence an organisation can check, "is half an admin" is not.
func (s *Server) darfSpaceVerwalten(ctx context.Context, uid, spaceID string) bool {
	var erlaubt bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM spaces WHERE id = $1 AND owner_id = $2)
		    OR EXISTS (SELECT 1 FROM users  WHERE id = $2 AND role = 'admin')
		    OR ($3 AND EXISTS (
		          SELECT 1 FROM space_rechte sr
		           WHERE sr.space_id = $1 AND sr.recht = 'verwalten'
		             AND (sr.user_id = $2
		                  OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
		                                      WHERE gm.user_id = $2))))`,
		spaceID, uid, lizenz.Frei(lizenz.Gruppen)).Scan(&erlaubt)
	return err == nil && erlaubt
}

// spaceZugriffSQL liefert die SQL-Bedingung, unter der ein Konto Zugriff auf
// den Space einer Seite hat. Zwei Wege führen hinein:
//
//	ein vergebenes Recht, einzeln oder über eine Gruppe, nur mit Zusatzumfang
//	eine öffentliche Ablage, für jedes angemeldete Konto der Instanz
//
// Das steht hier als eine Zeichenkette und nicht viermal ausgeschrieben in den
// Abfragen, die sie brauchen. Genau das ist der Punkt: Sichtbarkeit an vier
// Stellen getrennt zu formulieren heißt, dass eines Tages drei davon dasselbe
// sagen und die vierte etwas anderes, und eine abweichende Rechteprüfung ist
// der Weg, auf dem Seiten bei den Falschen landen.
//
// Die Platzhalter werden übergeben, weil sie je Abfrage andere Nummern haben:
// spaceID die Spalte mit der Space-Kennung, nutzer den Platzhalter des Kontos,
// gruppenAn den des Lizenzschalters.
func spaceZugriffSQL(spaceID, nutzer, gruppenAn string) string {
	return `(` + spaceID + ` IS NOT NULL AND (
	        EXISTS (SELECT 1 FROM spaces so
	                 WHERE so.id = ` + spaceID + ` AND so.oeffentlich <> 'nein')
	     OR (` + gruppenAn + ` AND EXISTS (
	           SELECT 1 FROM space_rechte sr
	            WHERE sr.space_id = ` + spaceID + `
	              AND (sr.user_id = ` + nutzer + `
	                   OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
	                                        WHERE gm.user_id = ` + nutzer + `))))))`
}
