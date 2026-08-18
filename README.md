# Nexora

A minimal, self-hosted knowledge base: think Notion or Outline, but small and yours.

Nested pages in a block editor, spaces, per-user sharing with roles, version history,
attachments, a trash can, backlinks and a knowledge graph. **Go** backend,
**React** frontend, **PostgreSQL** storage.

## Stack

| Layer    | Tech                                                     |
| -------- | -------------------------------------------------------- |
| Backend  | Go 1.22, chi router, pgx, JWT (httpOnly cookie), bcrypt  |
| Frontend | React 18, Vite, TypeScript, BlockNote editor             |
| Database | PostgreSQL 16                                            |
| Delivery | Docker Compose (nginx serves the SPA and proxies `/api`) |

## Features

### Writing

- **Nested pages**: infinite hierarchy in a collapsible sidebar, drag to reorder
- **Block editor**: slash menu and markdown shortcuts via BlockNote
- **Autosave**: title and content persist as you type
- **Markdown export**: download the current page as `.md`
- **Version history**: a snapshot is written before every content change; browse
  and restore any earlier revision
- **Attachments**: upload files per page (25 MiB per file), with inline preview
  for images, PDFs and plain text

### Organising

- **Spaces**: group pages into separate areas; a page may belong to one space
- **Tags**: colored, per-user, attach any number to a page
- **Favorites**: quick access section in the sidebar
- **Trash**: deleting moves a page to the trash; restore it or purge permanently
- **Search**: full text over titles and content
- **Backlinks and links**: link pages explicitly or with `@` mentions in the text,
  and see what points back at the current page
- **Knowledge graph**: a global graph of all pages plus a local view around one page

### Access

- **Multi-user**: email and password, JWT session cookie, bcrypt-hashed passwords
- **Roles**: the very first account created becomes `admin`; every later account is
  `user`. Admins manage accounts under the admin view
- **Per-user sharing**: share a page with named accounts, each as `read` or `write`
- **Public links**: publish a single page read-only behind a random token

## Quick Start

```bash
cp .env.example .env
# edit .env: set POSTGRES_PASSWORD and a long random JWT_SECRET
docker compose up -d --build
```

Open **http://localhost:3000** (or whichever `PORT` you set) and create the first
account, which becomes the workspace admin.

## Configuration

Docker Compose reads three variables from `.env`:

| Variable            | Purpose                            | Default         |
| ------------------- | ---------------------------------- | --------------- |
| `PORT`              | Host port for the web UI           | `3000`          |
| `POSTGRES_PASSWORD` | Database password (db + backend)   | `nexora`        |
| `JWT_SECRET`        | Secret used to sign session tokens | `change-me-...` |

Generate a strong secret with `openssl rand -hex 32`.

Two notes worth knowing before the first start:

- **PostgreSQL fixes the password on first launch.** It is written into the data
  directory at initialisation. Changing `POSTGRES_PASSWORD` afterwards locks the
  backend out unless you also change it inside the running database
  (`ALTER USER nexora WITH PASSWORD ...`) or discard the volume.
- **A missing `.env` does not stop the stack.** Compose falls back to the defaults
  above, so you get a running instance with the password `nexora` and a publicly
  known signing secret, without any warning.

The backend itself reads four variables. Compose sets the other three for you, so
they do not belong in `.env`:

| Variable          | Purpose                          | Set by Compose to     |
| ----------------- | -------------------------------- | --------------------- |
| `DATABASE_URL`    | PostgreSQL connection string     | derived from the above |
| `JWT_SECRET`      | Session token signing key        | from `.env`           |
| `NEXORA_DATA_DIR` | Where attachments are stored     | `/data/attachments`   |
| `PORT`            | Port the Go server listens on    | `8080` (container)    |

## Data Model

```
users        id, email, name, password_hash, role, created_at
spaces       id, owner_id, name, created_at
pages        id, owner_id, parent_id, space_id, title, content (jsonb),
             icon, is_public, public_token, sort_order, deleted_at,
             created_at, updated_at
page_versions  id, page_id, title, content, icon, author_id, created_at
attachments    id, page_id, owner_id, filename, mime, size, created_at
page_shares    page_id, user_id, permission ('read' | 'write'), created_at
page_links     source_id, target_id, created_at
tags           per-user tags; page_tags joins them to pages
favorites      per-user favorites
```

Deleting a page sets `deleted_at` (trash). Purging removes the row, which cascades
to its versions, attachments, shares, links and subpages.

## API

Everything lives under `/api`. Authentication uses an httpOnly cookie set on login
or register. `GET /api/public/{token}` and `/api/healthz` are the only unauthenticated
endpoints.

### Auth

```
POST   /auth/register                     create account (first one becomes admin)
POST   /auth/login                        sign in
POST   /auth/logout                       sign out
GET    /auth/me                           current user
```

### Pages

```
GET    /pages                             flat list for the sidebar tree
GET    /pages/shared                      pages other users shared with you
GET    /pages/trash                       deleted pages
POST   /pages                             create (optional parentId, spaceId)
GET    /pages/{id}                        full page incl. tags, favorite, permission
PUT    /pages/{id}                        update title / content / icon / parent / space
DELETE /pages/{id}                        move to trash
POST   /pages/{id}/restore                restore from trash
DELETE /pages/{id}/purge                  delete permanently
POST   /pages/{id}/favorite               add favorite
DELETE /pages/{id}/favorite               remove favorite
POST   /pages/{id}/tags                   attach a tag
DELETE /pages/{id}/tags/{tagId}           detach a tag
```

### Versions and attachments

```
GET    /pages/{id}/versions               list snapshots
GET    /pages/{id}/versions/{versionId}   read one snapshot
POST   /pages/{id}/versions/{versionId}/restore   roll the page back

GET    /pages/{id}/attachments            list files
POST   /pages/{id}/attachments            upload (multipart field "file", max 25 MiB)
GET    /pages/{id}/attachments/{attId}    download
DELETE /pages/{id}/attachments/{attId}    delete
```

### Sharing

```
POST   /pages/{id}/share                  publish read-only, returns a token
DELETE /pages/{id}/share                  revoke the public link
GET    /pages/{id}/shares                 who this page is shared with
POST   /pages/{id}/shares                 share with a user ('read' or 'write')
DELETE /pages/{id}/shares/{userId}        revoke a user's access
GET    /public/{token}                    read-only public page (no auth)
```

### Links and graph

```
GET    /pages/{id}/links                  outgoing links
POST   /pages/{id}/links                  add a link
DELETE /pages/{id}/links/{targetId}       remove a link
GET    /pages/{id}/backlinks              pages linking here
GET    /graph                             nodes and edges of the whole workspace
```

### Spaces, tags, users, misc

```
GET    /spaces                            list spaces
POST   /spaces                            create
PUT    /spaces/{id}                       rename
DELETE /spaces/{id}                       delete (pages keep existing, space_id nulled)

GET    /tags                              list tags
POST   /tags                              create
DELETE /tags/{id}                         delete

GET    /users                             list accounts (admin only)
POST   /users                             create an account (admin only)
DELETE /users/{id}                        delete an account (admin only, not yourself)
PUT    /users/{id}/role                   change role (admin only)

GET    /favorites                         favorited pages
GET    /search?q=                         search titles and content
GET    /healthz                           liveness probe
```

## Project Structure

```
backend/                       Go API
  main.go                      router, route table, server bootstrap
  internal/db                  connection pool, schema creation and migration
  internal/auth                JWT issuing and password hashing
  internal/middleware          cookie auth
  internal/handlers
    auth.go                    register, login, logout, me
    pages.go                   CRUD, tree, trash, restore, purge
    versions.go                snapshots and rollback
    attachments.go             upload, download, delete
    sharing.go                 public links
    access.go                  per-user shares and permission checks
    links.go, backlinks.go     explicit page links
    graph.go                   graph nodes and edges
    spaces.go, tags.go         organisation
    users.go                   admin account management
frontend/                      React SPA (Vite + TypeScript)
  src/api                      typed API client
  src/components               Sidebar, PageTree, Editor, Attachments,
                               VersionPanel, ShareDialog, LocalGraph
  src/pages                    Login, Register, Workspace, PageView,
                               PublicPage, TrashView, GraphView, AdminView
docker-compose.yml             db + backend + frontend
```


## Local Development

```bash
# backend: needs a local PostgreSQL, or point DATABASE_URL at the compose db
cd backend && go run .

# frontend: proxies /api to localhost:8080
cd frontend && npm install && npm run dev
```

## License

Apache License 2.0, see [LICENSE](LICENSE). Copyright 2026 Jonas Groll.
