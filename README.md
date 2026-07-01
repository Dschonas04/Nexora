# Nexora

A minimal, self-hosted knowledge base — think Notion / Outline, but small and yours.

Nested pages, a Notion-style block editor, tags, favorites, full-text search and
public share links. **Go** backend, **React** frontend, **PostgreSQL** storage.

## Stack

| Layer     | Tech                                                        |
| --------- | ----------------------------------------------------------- |
| Backend   | Go 1.22 · chi router · pgx · JWT (httpOnly cookie) · bcrypt |
| Frontend  | React 18 · Vite · TypeScript · BlockNote editor             |
| Database  | PostgreSQL 16                                               |
| Delivery  | Docker Compose (nginx serves the SPA and proxies `/api`)    |

## Features

- **Nested pages** — infinite hierarchy in a collapsible sidebar
- **Block editor** — Notion-style editing (slash menu, markdown shortcuts) via BlockNote
- **Multi-user** — register / login, JWT session cookies, bcrypt-hashed passwords
- **Tags** — colored, per-user, attach to any page
- **Favorites** — quick access section in the sidebar
- **Search** — full-text over titles and content
- **Public links** — share a single page read-only via a random token
- **Autosave** — title, icon and content save automatically

## Quick Start

```bash
cp .env.example .env
# edit .env: set POSTGRES_PASSWORD and a long random JWT_SECRET
docker compose up -d --build
```

Open **http://localhost:3000** (or whatever `PORT` you set) and create the first account.

## Configuration

| Variable            | Purpose                            | Default         |
| ------------------- | ---------------------------------- | --------------- |
| `PORT`              | Host port for the web UI           | `3000`          |
| `POSTGRES_PASSWORD` | Database password (db + backend)   | `nexora`        |
| `JWT_SECRET`        | Secret used to sign session tokens | `change-me-...` |

Generate a strong secret with `openssl rand -hex 32`.

## Project Structure

```
backend/                Go API
  main.go               router + server bootstrap
  internal/db           connection pool + schema migration
  internal/auth         JWT + password hashing
  internal/middleware   cookie auth middleware
  internal/handlers     auth, pages, tags, favorites, search, sharing
frontend/               React SPA (Vite + TypeScript)
  src/api               typed API client
  src/components        Sidebar, PageTree, Editor
  src/pages             Login, Register, Workspace, PageView, PublicPage
docker-compose.yml      db + backend + frontend
```

## API (overview)

All endpoints live under `/api`. Auth uses an httpOnly cookie set on login/register.

```
POST   /auth/register            create account
POST   /auth/login               sign in
POST   /auth/logout              sign out
GET    /auth/me                  current user

GET    /pages                    flat list (sidebar tree)
POST   /pages                    create (optional parentId)
GET    /pages/:id                full page (content, tags, favorite)
PUT    /pages/:id                update title / content / icon / parent
DELETE /pages/:id                delete (cascades to subpages)
POST   /pages/:id/favorite       add favorite (DELETE to remove)
POST   /pages/:id/share          make public (returns token); DELETE to revoke
POST   /pages/:id/tags           attach tag; DELETE /pages/:id/tags/:tagId

GET    /favorites                favorited pages
GET    /tags                     list tags; POST to create, DELETE /tags/:id
GET    /search?q=                search titles + content
GET    /public/:token            read-only public page (no auth)
```

## Local Development

```bash
# backend (needs a local Postgres, or point DATABASE_URL at the compose db)
cd backend && go run .

# frontend (proxies /api to localhost:8080)
cd frontend && npm install && npm run dev
```

## License

Apache License 2.0 — see [LICENSE](LICENSE). Copyright 2026 Jonas Groll.
