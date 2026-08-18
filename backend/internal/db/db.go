// Package db owns the connection pool and the schema. There is no migration
// tool: the whole schema lives in one idempotent script that runs on every boot.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx pool and waits for the database to become reachable
// (Postgres may still be starting up in a fresh Docker Compose stack).
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// pgxpool.NewWithConfig does not dial, so a bad address only surfaces here.
	// Retry for about 30 seconds because Postgres in a fresh Compose stack needs
	// a moment before it accepts connections.
	for i := 0; i < 15; i++ {
		if err = pool.Ping(ctx); err == nil {
			return pool, nil
		}
		time.Sleep(2 * time.Second)
	}
	pool.Close()
	return nil, err
}

// schema is written so it can run against an empty database and against one
// that is already up to date. New columns are added with ALTER TABLE ... IF NOT
// EXISTS rather than by editing the CREATE TABLE above, so existing installs
// pick them up on the next start.
const schema = `
-- pgcrypto supplies gen_random_uuid(), used as the default for every id.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
	id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	email         text UNIQUE NOT NULL,
	name          text NOT NULL,
	password_hash text NOT NULL,
	created_at    timestamptz NOT NULL DEFAULT now()
);

-- The page tree is self-referential: parent_id points at another page, and the
-- cascade means purging a page takes its whole subtree with it. content holds
-- the BlockNote document as JSON; the backend never interprets it.
CREATE TABLE IF NOT EXISTS pages (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	owner_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	parent_id    uuid REFERENCES pages(id) ON DELETE CASCADE,
	title        text NOT NULL DEFAULT 'Untitled',
	content      jsonb NOT NULL DEFAULT '[]'::jsonb,
	icon         text NOT NULL DEFAULT '',
	is_public    boolean NOT NULL DEFAULT false,
	public_token text UNIQUE,
	sort_order   double precision NOT NULL DEFAULT 0,
	created_at   timestamptz NOT NULL DEFAULT now(),
	updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pages_owner_idx  ON pages(owner_id);
CREATE INDEX IF NOT EXISTS pages_parent_idx ON pages(parent_id);

-- Tags are per user, not global, hence the UNIQUE over (owner_id, name).
CREATE TABLE IF NOT EXISTS tags (
	id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name     text NOT NULL,
	color    text NOT NULL DEFAULT '#6b7280',
	UNIQUE (owner_id, name)
);

CREATE TABLE IF NOT EXISTS page_tags (
	page_id uuid REFERENCES pages(id) ON DELETE CASCADE,
	tag_id  uuid REFERENCES tags(id)  ON DELETE CASCADE,
	PRIMARY KEY (page_id, tag_id)
);

CREATE TABLE IF NOT EXISTS favorites (
	user_id uuid REFERENCES users(id)  ON DELETE CASCADE,
	page_id uuid REFERENCES pages(id)  ON DELETE CASCADE,
	PRIMARY KEY (user_id, page_id)
);

-- Roles: 'admin' can see and edit every page; 'user' is the default.
ALTER TABLE users ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'user';

-- Spaces group pages into top-level workspaces (beyond page nesting).
CREATE TABLE IF NOT EXISTS spaces (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name       text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS spaces_owner_idx ON spaces(owner_id);

-- Deleting a space keeps its pages and only clears their space_id.
ALTER TABLE pages ADD COLUMN IF NOT EXISTS space_id   uuid REFERENCES spaces(id) ON DELETE SET NULL;
-- deleted_at is the trash: NULL means live, a timestamp means deleted but
-- recoverable. Every page query has to filter on it.
ALTER TABLE pages ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE INDEX IF NOT EXISTS pages_space_idx   ON pages(space_id);
CREATE INDEX IF NOT EXISTS pages_deleted_idx ON pages(deleted_at);

-- Immutable snapshots of a page, written on save (coalesced).
CREATE TABLE IF NOT EXISTS page_versions (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	page_id    uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	title      text NOT NULL,
	content    jsonb NOT NULL DEFAULT '[]'::jsonb,
	icon       text NOT NULL DEFAULT '',
	author_id  uuid REFERENCES users(id) ON DELETE SET NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS page_versions_idx ON page_versions(page_id, created_at DESC);

-- File attachments stored on disk (path = attachments/<id>), metadata here.
CREATE TABLE IF NOT EXISTS attachments (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	page_id    uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	filename   text NOT NULL,
	mime       text NOT NULL DEFAULT 'application/octet-stream',
	size       bigint NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS attachments_page_idx ON attachments(page_id);

-- Explicit per-user sharing with a read/edit permission.
CREATE TABLE IF NOT EXISTS page_shares (
	page_id    uuid REFERENCES pages(id)  ON DELETE CASCADE,
	user_id    uuid REFERENCES users(id)  ON DELETE CASCADE,
	permission text NOT NULL DEFAULT 'read',
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (page_id, user_id)
);
CREATE INDEX IF NOT EXISTS page_shares_user_idx ON page_shares(user_id);

-- Manual page-to-page links, edited via the UI (independent of [[wiki-links]]
-- written into the text). Feed the knowledge graph and backlinks like wiki-links.
CREATE TABLE IF NOT EXISTS page_links (
	source_id  uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	target_id  uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (source_id, target_id)
);
CREATE INDEX IF NOT EXISTS page_links_target_idx ON page_links(target_id);
`

// Migrate applies the schema. It is idempotent and safe to run on every start,
// which is exactly what main does.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schema)
	return err
}
