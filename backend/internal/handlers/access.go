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
//	canRead — owner, admin, page share, or a right on the space it sits in
//	canEdit — owner, admin, share with 'edit', or 'schreiben'/'verwalten' on the space
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
// rows may still exist -- they are not deleted when a licence lapses -- but
// they grant nothing, and access falls back to owner, admin and page shares.
func (s *Server) pagePerm(ctx context.Context, uid, pageID string) (canRead, canEdit, isOwner, ok bool) {
	gruppenAn := lizenz.Frei(lizenz.Gruppen)

	var ownerID string
	var admin bool
	var freigabe *string   // 'read' | 'edit', wenn die Seite direkt geteilt ist
	var spaceRecht *string // 'lesen' | 'schreiben' | 'verwalten', bestes Recht am Space

	err := s.Pool.QueryRow(ctx, `
		SELECT
			p.owner_id::text,
			(SELECT role = 'admin' FROM users WHERE id = $2),
			(SELECT sh.permission FROM page_shares sh
			  WHERE sh.page_id = p.id AND sh.user_id = $2),
			-- Bestes Recht am Space: direkt vergeben oder über eine Gruppe.
			-- Die Reihenfolge im ORDER BY ist die Stufenleiter, nicht das
			-- Alphabet -- 'lesen' käme sonst vor 'schreiben'.
			(SELECT sr.recht FROM space_rechte sr
			  WHERE $3
			    AND sr.space_id = p.space_id
			    AND (sr.user_id = $2
			         OR sr.gruppe_id IN (SELECT gm.gruppe_id FROM gruppen_mitglieder gm
			                             WHERE gm.user_id = $2))
			  ORDER BY CASE sr.recht
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
