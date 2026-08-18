// Package models holds the shapes the API sends over the wire. They are not the
// database rows: fields such as password_hash never leave the backend, and a few
// fields (CanEdit, IsFavorite) are computed per request and per user.
package models

import (
	"encoding/json"
	"time"
)

// User is the public view of an account. The password hash is deliberately
// absent so it cannot leak through a handler that serialises this struct.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Space is a top-level container grouping pages beyond simple nesting.
type Space struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Tag is a colored label. Tags belong to one user, so two people can both have
// a tag named "Project" without seeing each other's.
type Tag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// PageMeta is the lightweight shape used for the sidebar tree and lists. It
// leaves out Content, which keeps the tree request small even for a workspace
// with hundreds of pages.
type PageMeta struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parentId"`
	SpaceID   *string   `json:"spaceId"`
	Title     string    `json:"title"`
	Icon      string    `json:"icon"`
	Shared    bool      `json:"shared"` // true when this page is shared to me, not owned
	UpdatedAt time.Time `json:"updatedAt"`
}

// Page is the full page returned when a single page is opened.
type Page struct {
	ID          string          `json:"id"`
	OwnerID     string          `json:"ownerId"`
	ParentID    *string         `json:"parentId"`
	SpaceID     *string         `json:"spaceId"`
	Title       string          `json:"title"`
	Content     json.RawMessage `json:"content"`
	Icon        string          `json:"icon"`
	IsPublic    bool            `json:"isPublic"`
	PublicToken *string         `json:"publicToken"`
	Tags        []Tag           `json:"tags"`
	IsFavorite  bool            `json:"isFavorite"`
	CanEdit     bool            `json:"canEdit"` // false for read-only shares
	IsOwner     bool            `json:"isOwner"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// PageVersion is an immutable snapshot in a page's history.
type PageVersion struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Content    json.RawMessage `json:"content,omitempty"`
	Icon       string          `json:"icon"`
	AuthorName string          `json:"authorName"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// Attachment is a file uploaded to a page.
type Attachment struct {
	ID        string    `json:"id"`
	PageID    string    `json:"pageId"`
	Filename  string    `json:"filename"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

// ShareEntry is one user a page is shared with, plus its permission.
type ShareEntry struct {
	UserID     string `json:"userId"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Permission string `json:"permission"` // "read" | "edit"
}

// GraphNode / GraphEdge power the knowledge graph view.
type GraphNode struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	SpaceID *string `json:"spaceId"`
	Space   string  `json:"space"` // space name, "" for pages not in a space
}

// GraphEdge connects two nodes. Kind separates the hierarchy from explicit
// links so the view can draw them differently.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"` // "parent" | "link"
}

// Graph is the whole workspace as the graph view consumes it.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}
