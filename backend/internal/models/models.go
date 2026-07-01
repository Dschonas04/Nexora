package models

import (
	"encoding/json"
	"time"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Tag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// PageMeta is the lightweight shape used for the sidebar tree and lists.
type PageMeta struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parentId"`
	Title     string    `json:"title"`
	Icon      string    `json:"icon"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Page is the full page returned when a single page is opened.
type Page struct {
	ID          string          `json:"id"`
	OwnerID     string          `json:"ownerId"`
	ParentID    *string         `json:"parentId"`
	Title       string          `json:"title"`
	Content     json.RawMessage `json:"content"`
	Icon        string          `json:"icon"`
	IsPublic    bool            `json:"isPublic"`
	PublicToken *string         `json:"publicToken"`
	Tags        []Tag           `json:"tags"`
	IsFavorite  bool            `json:"isFavorite"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}
