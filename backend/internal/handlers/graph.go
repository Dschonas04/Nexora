// The knowledge graph, plus the wiki-link parsing that backlinks.go also uses.
package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"nexora/internal/middleware"
	"nexora/internal/models"
)

// wikiLinkRe matches explicit [[Page title]] references inside page text.
var wikiLinkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// extractText walks decoded BlockNote content and collects every string value.
// [[links]] only occur in visible text; structural JSON (types, ids, styles)
// never contains "[[", so gathering all strings safely covers both the editor's
// inline-node format and the plain-string content shorthand.
func extractText(v interface{}, sb *strings.Builder) {
	switch t := v.(type) {
	case string:
		sb.WriteString(t)
		sb.WriteString(" ")
	case map[string]interface{}:
		for _, val := range t {
			extractText(val, sb)
		}
	case []interface{}:
		for _, item := range t {
			extractText(item, sb)
		}
	}
}

// wikiLinks returns the lowercased titles referenced via [[...]] in a page's
// JSON content.
func wikiLinks(content string) []string {
	var doc interface{}
	if json.Unmarshal([]byte(content), &doc) != nil {
		return nil
	}
	var sb strings.Builder
	extractText(doc, &sb)
	var out []string
	for _, m := range wikiLinkRe.FindAllStringSubmatch(sb.String(), -1) {
		if title := strings.ToLower(strings.TrimSpace(m[1])); title != "" {
			out = append(out, title)
		}
	}
	return out
}

// Graph builds the whole visible workspace as nodes and edges.
//
// Everything is assembled in memory: the pages are read once, then edges are
// derived from three sources. That keeps the permission rule in one place,
// because a page missing from the map simply cannot gain an edge, so no link
// can point at something the caller may not see.
//
// The response grows with the size of the workspace and is not paginated, which
// is the practical limit of this endpoint.
func (s *Server) Graph(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)

	var rows pgx.Rows
	var err error
	if s.isAdmin(r.Context(), uid) {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.title, p.parent_id, p.content::text, p.space_id, COALESCE(s.name, '')
			 FROM pages p LEFT JOIN spaces s ON s.id = p.space_id
			 WHERE p.deleted_at IS NULL`)
	} else {
		rows, err = s.Pool.Query(r.Context(),
			`SELECT p.id, p.title, p.parent_id, p.content::text, p.space_id, COALESCE(s.name, '')
			 FROM pages p LEFT JOIN spaces s ON s.id = p.space_id
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
		parent *string
		links  []string // lowercased [[titles]] referenced by this page
	}
	pages := map[string]node{}
	titles := map[string]string{} // lowercased title -> id
	g := models.Graph{Nodes: []models.GraphNode{}, Edges: []models.GraphEdge{}}

	for rows.Next() {
		var id, title, content, space string
		var parent, spaceID *string
		if err := rows.Scan(&id, &title, &parent, &content, &spaceID, &space); err != nil {
			continue
		}
		pages[id] = node{parent: parent, links: wikiLinks(content)}
		g.Nodes = append(g.Nodes, models.GraphNode{ID: id, Title: title, SpaceID: spaceID, Space: space})
		titles[strings.ToLower(strings.TrimSpace(title))] = id
	}

	// addEdge deduplicates by direction, so two pages linking to each other still
	// produce two distinct edges while a repeated link produces one.
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

	// Hierarchy first: because addEdge keeps whichever edge arrives first, a
	// parent-child pair that also links to each other stays drawn as nesting,
	// which is the stronger relationship.
	for id, n := range pages {
		if n.parent != nil {
			if _, ok := pages[*n.parent]; ok {
				addEdge(*n.parent, id, "parent")
			}
		}
	}
	// Wiki-links, resolved through the title map. A [[link]] to a title that does
	// not exist, or to a page outside the caller's view, is dropped. If two pages
	// share a title, the last one read wins.
	for id, n := range pages {
		for _, title := range n.links {
			if otherID, ok := titles[title]; ok {
				addEdge(id, otherID, "link")
			}
		}
	}
	// Explicit links. The table is read whole, without a permission clause, and
	// filtered against the pages map instead: an edge is only kept when both ends
	// are already visible nodes.
	if lrows, lerr := s.Pool.Query(r.Context(), `SELECT source_id, target_id FROM page_links`); lerr == nil {
		defer lrows.Close()
		for lrows.Next() {
			var src, dst string
			if lrows.Scan(&src, &dst) != nil {
				continue
			}
			if _, okS := pages[src]; okS {
				if _, okD := pages[dst]; okD {
					addEdge(src, dst, "link")
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, g)
}
