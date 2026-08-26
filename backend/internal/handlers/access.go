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
	var spaceRecht *string // 'lesen' | 'schreiben' | 'verwalten', best right on the space

	err := s.Pool.QueryRow(ctx, `
		SELECT
			p.owner_id::text,
			(SELECT role = 'admin' FROM users WHERE id = $2),
			(SELECT sh.permission FROM page_shares sh
			  WHERE sh.page_id = p.id AND sh.user_id = $2),
			-- Best right on the space: granted directly, through a group, or
			-- from the space being public. All three ways land in the same
			-- UNION and are then sorted by level so that the strongest wins.
			-- The order in the ORDER BY is the ladder of levels, not the
			-- alphabet, otherwise 'lesen' would come before 'schreiben'.
			(SELECT x.recht FROM (
			   SELECT sr.recht FROM space_rechte sr
			    WHERE $3
			      AND sr.space_id = p.space_id
			      AND (sr.user_id = $2
			           OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
			                               WHERE gm.user_id = $2))
			   UNION ALL
			   -- A public space applies to every logged in account and depends
			   -- on no paid extra: it is part of the base equipment.
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

// spaceZugriffSQL returns the SQL condition under which an account has access
// to the space of a page. Two ways lead in:
//
//	a granted right, directly or through a group, only with the paid extra
//	a public space, for every logged in account of the instance
//
// It stands here as one string instead of being written out four times in the
// queries that need it. That is precisely the point: phrasing visibility in four
// places separately means that one day three of them say the same and the fourth
// says something else, and a diverging permission check is the way pages end up
// with the wrong people.
//
// The placeholders are passed in because they carry different numbers in each
// query: spaceID the column with the space id, nutzer the placeholder of the
// account, gruppenAn the one of the licence switch.
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
