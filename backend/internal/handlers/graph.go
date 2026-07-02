package handlers

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// Graph returns the knowledge graph the user can see: one node per accessible
// page, plus edges for parent-child nesting and for pages that mention another
// page's title in their content.
func (s *Server) Graph(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	var rows pgx.Rows
	var err error
	if s.isAdmin(r.Context(), uid) {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT id, title, parent_id, content::text FROM pages WHERE deleted_at IS NULL`)
	} else {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.title, p.parent_id, p.content::text FROM pages p
			 WHERE p.deleted_at IS NULL AND (
			   p.owner_id=$1 OR p.id IN (SELECT page_id FROM page_shares WHERE user_id=$1)
			 )`, uid)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type node struct {
		title, content string
		parent         *string
	}
	pages := map[string]node{}
	titles := map[string]string{} // lowercased title -> id
	g := models.Graph{Nodes: []models.GraphNode{}, Edges: []models.GraphEdge{}}

	for rows.Next() {
		var id, title, content string
		var parent *string
		if err := rows.Scan(&id, &title, &parent, &content); err != nil {
			continue
		}
		pages[id] = node{title: title, content: strings.ToLower(content), parent: parent}
		g.Nodes = append(g.Nodes, models.GraphNode{ID: id, Title: title})
		if t := strings.ToLower(strings.TrimSpace(title)); len(t) >= 3 {
			titles[t] = id
		}
	}

	seen := map[string]bool{}
	addEdge := func(src, dst, kind string) {
		if src == dst {
			return
		}
		key := src + "->" + dst
		if seen[key] {
			return
		}
		seen[key] = true
		g.Edges = append(g.Edges, models.GraphEdge{Source: src, Target: dst, Kind: kind})
	}

	for id, n := range pages {
		// Hierarchy edges.
		if n.parent != nil {
			if _, ok := pages[*n.parent]; ok {
				addEdge(*n.parent, id, "parent")
			}
		}
		// Mention edges: this page's content references another page's title.
		for t, otherID := range titles {
			if otherID == id {
				continue
			}
			if n.parent != nil && *n.parent == otherID {
				continue // already a hierarchy edge
			}
			if strings.Contains(n.content, t) {
				addEdge(id, otherID, "link")
			}
		}
	}

	writeJSON(w, http.StatusOK, g)
}
