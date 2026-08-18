// Access control lives here. Both helpers hit the database on every call rather
// than caching, so a share revoked in one tab takes effect in the next request
// made from another.
package handlers

import "context"

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
//	canRead — owner, admin, or shared (read/edit)
//	canEdit — owner, admin, or shared with permission 'edit'
//	isOwner — the user owns the page
//	ok      — the page exists, is not in the trash, and the user may read it
func (s *Server) pagePerm(ctx context.Context, uid, pageID string) (canRead, canEdit, isOwner, ok bool) {
	var ownerID string
	if err := s.Pool.QueryRow(ctx,
		`SELECT owner_id FROM pages WHERE id=$1 AND deleted_at IS NULL`, pageID).Scan(&ownerID); err != nil {
		return false, false, false, false
	}
	isOwner = ownerID == uid
	if isOwner || s.isAdmin(ctx, uid) {
		return true, true, isOwner, true
	}
	var perm string
	if err := s.Pool.QueryRow(ctx,
		`SELECT permission FROM page_shares WHERE page_id=$1 AND user_id=$2`, pageID, uid).Scan(&perm); err == nil {
		return true, perm == "edit", false, true
	}
	return false, false, false, false
}
