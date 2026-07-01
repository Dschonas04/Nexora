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
	for i := 0; i < 15; i++ {
		if err = pool.Ping(ctx); err == nil {
			return pool, nil
		}
		time.Sleep(2 * time.Second)
	}
	pool.Close()
	return nil, err
}

const schema = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
	id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	email         text UNIQUE NOT NULL,
	name          text NOT NULL,
	password_hash text NOT NULL,
	created_at    timestamptz NOT NULL DEFAULT now()
);

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
`

// Migrate creates the schema if it does not yet exist (idempotent).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schema)
	return err
}
