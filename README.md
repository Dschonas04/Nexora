# Nexora

A minimal, self-hosted knowledge base: think Notion or Outline, but small and yours.

Nested pages in a block editor, spaces, per-user sharing with roles, version
history, attachments, comments, a trash can, backlinks, a knowledge graph and a
real full text search. **Go** backend, **React** frontend, **PostgreSQL**
storage.

Licensed under the [Business Source License 1.1](LICENSING.md). The source is
open and the core may be run in production, commercially, without paying
anyone. Twelve extras — audit trail, groups, SSO, LDAP and more — need a
license key. On 2030-08-19 the whole thing becomes Apache 2.0.

## Stack

| Layer    | Tech                                                     |
| -------- | -------------------------------------------------------- |
| Backend  | Go 1.22+, chi router, pgx, JWT (httpOnly cookie), bcrypt  |
| Frontend | React 18, Vite, TypeScript, BlockNote editor             |
| Database | PostgreSQL 16                                            |
| Delivery | Docker Compose (nginx serves the SPA and proxies `/api`) |

## Features

### Writing

- **Nested pages**: infinite hierarchy in a collapsible sidebar, drag to reorder
- **Block editor**: slash menu and markdown shortcuts via BlockNote
- **Autosave**: title and content persist as you type
- **Markdown export**: download the current page as `.md` — always free, because
  getting your own content out must never sit behind a licence
- **Markdown and HTML import**: drop in single `.md`/`.html` files or a whole
  `.zip` — an Obsidian vault, a Notion export (the id in every filename is
  stripped), a Confluence HTML export, a git wiki, a folder of notes. A preview
  shows the page tree that would result before anything is created. The archive
  keeps its shape: a folder becomes a page, the files inside become its subpages, and an
  `index.md`, `README.md` or `INHALT.md` becomes the folder's own content. Links
  between imported files are rewritten to `[[Page title]]`, so they still lead
  somewhere and feed backlinks and the graph; images and other files become
  attachments of the page that uses them, and anything nothing referenced is
  attached to its folder's page rather than dropped. Front matter supplies title,
  tags and icon. Free, like the export — the way in must not sit behind a licence
  either
- **PDF and Word export**: a typeset document for a page, or a whole space as one
  file. Written without a third-party library; the PDF uses the base fonts and
  WinAnsi encoding, so umlauts survive
- **Version history**: a snapshot is written before every content change; browse
  and restore any earlier revision
- **Attachments**: stored on local disk or in any S3-compatible bucket (MinIO,
  Garage, Ceph, AWS) — the handlers never learn which. Upload files per page,
  with a quick viewer for images, PDFs
  and plain text — browse between files with `←` `→`, zoom with `+` `−`, rotate
  with `R`, `0` resets, `Esc` closes
- **Comments**: threads under every page, one reply level deep, mark a thread as
  settled. Deleting empties the text and keeps the shell, so replies hanging off
  it do not lose their place
- **Conflict detection**: the editor sends the state it started from. If the page
  moved on in between, the save is refused instead of overwriting a colleague

### Organising

- **Spaces**: group pages into separate areas; a page may belong to one space. A
  space can be opened to every signed-in account of the instance, for reading or
  for writing — that means the instance, not the internet; anonymous access
  still runs solely through a page's share link
- **Collapsible sidebar sections**: every section folds away and stays folded
  across reloads; spaces and tags show the first four and reveal the rest on
  demand
- **Tags**: colored, per-user, attach any number to a page
- **Favorites**: quick access section in the sidebar
- **Trash**: deleting moves a page to the trash; restore it or purge permanently.
  It empties itself after `papierkorb_tage` (30 by default, 0 disables it), and
  every row says how long it has left. The hourly sweep removes the attachment
  bytes too, so an object store does not silently fill up with files no page
  points at any more
- **Search**: real full text search (PostgreSQL `tsvector`, GIN index) with
  relevance ranking and snippets. Narrowed by space, tag, age and authorship
  from dropdowns rather than a query syntax nobody remembers. Title matches outrank body matches. Results
  are limited to pages the caller may read, using the same rule that governs
  opening a single page: owner, admin, or an explicit share
- **Backlinks and links**: link pages explicitly or with `@` mentions in the text,
  and see what points back at the current page
- **Knowledge graph**: a global graph of all pages plus a local view around one page

### Access

- **Multi-user**: email and password, JWT session cookie, bcrypt-hashed passwords
- **Roles**: the very first account created becomes `admin`; every later account is
  `user`. Admins manage accounts under the admin view
- **Per-user sharing**: share a page with named accounts, each as `read` or `write`
- **Public links**: publish a single page read-only behind a random token
- **Inbox**: comments on your pages, replies to your comments, `@Name` mentions
  and pages somebody shared with you. Three kinds and no more — an inbox that
  carries noise is one people stop opening
- **Audit trail**: who did what, when — sign-ins including the failed ones,
  accounts, pages, trash, permanent deletion, shares and public links. Entries
  survive the deletion of the page or account they refer to, because deleting is
  exactly what an auditor comes looking for

## Quick Start

```bash
cp .env.example .env
# edit .env: set POSTGRES_PASSWORD and a long random JWT_SECRET
docker compose up -d --build
```

`.env.example` ships with `COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml`,
so the command above brings its own PostgreSQL. The database and the object
store are deliberately kept out of the main file — most installations already
run one, and tying Nexora to its own copy would mean operating them twice:

```bash
# Nexora only, against a database you already run (set DATABASE_URL)
COMPOSE_FILE=docker-compose.yml

# with the bundled PostgreSQL (default)
COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml

# plus a bundled MinIO for attachments
COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml:docker-compose.minio.yml
```

An existing MinIO or S3 is wired up through the `NEXORA_S3_*` settings instead
of the third file.

Open **http://localhost:3000** (or whichever `PORT` you set) and create the first
account, which becomes the workspace admin.

## Configuration

Everything is read from **`config.conf`**, which documents itself: 245 lines,
174 of them comment. Every setting says what it does, which environment
variable overrides it, and what happens when it is set wrong.

Precedence, highest first:

```
environment variable   →   config.conf   →   built-in default
```

The environment wins so a container can override a single value without a
rebuilt image, and so a secret never has to touch disk. The file is looked for
at `$NEXORA_CONFIG`, then `./config.conf`, then `/etc/nexora/config.conf`.

**A missing file is not an error.** Every setting has a default that produces a
working server, which is what lets the binary start with no configuration at
all.

The file can also be edited from **Settings → Wartung**, which checks the draft
before writing it and keeps a timestamped backup of the previous version.
Credentials are masked on the way to the browser and restored on the way back,
so saving never overwrites them with asterisks. Because the file is only read at
startup, that page also carries a restart button — the process ends and whatever
runs it brings it back.

The format is deliberately dull:

```ini
schluessel = wert          # one entry per line, # and ; start a comment
[Abschnitt]                # sections are read and discarded, they only group
"  wert mit Rand  "        # quotes preserve leading and trailing spaces
```

Broken lines do not bring anything down: an unreadable number keeps the
default, a line without `=` is reported with its line number and skipped. A
typo must not cause an outage.

### The settings

| Group | Keys |
| --- | --- |
| Server | `port`, `daten_verzeichnis`, `oeffentliche_url` |
| Database | `datenbank_url` |
| Sessions | `jwt_geheimnis`, `sitzung_tage` |
| License | `lizenz` |
| Registration | `registrierung_offen`, `erlaubte_domaenen` |
| Search | `such_woerterbuch` |
| Attachments | `max_anhang_mb` |
| Trash | `papierkorb_tage` |
| Object storage | `s3_aktiv` and seven more |
| LDAP / AD | `ldap_aktiv` and ten more |
| OIDC | `oidc_aktiv` and eight more |

### Warnings on start

Dangerous defaults are named on every boot without preventing it — a homelab
install with the default secret should still run, it should just be impossible
to miss that it did:

```
ACHTUNG: jwt_geheimnis steht auf der Vorgabe -- jede Sitzung ist fälschbar
ACHTUNG: LDAP ohne TLS -- Zugangsdaten gehen im Klartext über das Netz
```

### Two things to know before the first start

- **PostgreSQL fixes the password on first launch.** It is written into the data
  directory at initialisation. Changing it afterwards locks the backend out
  unless you also change it inside the running database
  (`ALTER USER nexora WITH PASSWORD ...`) or discard the volume.
- **The very first account created becomes the administrator.** Turning
  `registrierung_offen` off before that account exists locks everyone out.

### `.env`

Docker Compose still reads `.env` for the values it needs before the backend
starts, and for anything you would rather not put in a file that lives in the
repository:

| Variable | Purpose |
| --- | --- |
| `PORT` | Host port for the web UI |
| `POSTGRES_PASSWORD` | Database password, used by both db and backend |
| `JWT_SECRET` | Session signing key — overrides `jwt_geheimnis` |
| `NEXORA_LIZENZ` | License key — overrides `lizenz` |

## Data Model

```
users        id, email, name, password_hash, role, created_at
spaces       id, owner_id, name, created_at
pages        id, owner_id, parent_id, space_id, title, content (jsonb),
             content_text, such_tsv (generated), icon, is_public,
             public_token, sort_order, deleted_at, created_at, updated_at
page_versions  id, page_id, title, content, icon, author_id, created_at
attachments    id, page_id, owner_id, filename, mime, size, created_at
page_shares    page_id, user_id, permission ('read' | 'write'), created_at
page_links     source_id, target_id, created_at
tags           per-user tags; page_tags joins them to pages
favorites      per-user favorites
kommentare     id, page_id, eltern_id, autor_id, autor_name, text, erledigt,
               erstellt_am, geaendert_am, geloescht_am
pruefspur      id, zeitpunkt, akteur_id, akteur_name, akteur_email, aktion,
               objekt_art, objekt_id, objekt_titel, details (jsonb), ip
postfach       id, empfaenger_id, art, page_id, kommentar_id, ausloeser_id,
               ausloeser_name, seiten_titel, text, gelesen_am, erstellt_am
```

`content_text` holds the prose pulled out of the BlockNote JSON on every save;
`such_tsv` is generated from it plus the title, with the title weighted higher,
and carries a GIN index. That is what makes the search a search rather than a
`LIKE '%word%'` over raw JSON.

`pruefspur` deliberately has **no** foreign key with a cascade. Names and titles
are frozen copies, so an entry stays readable after the page or account it
refers to is gone — deleting is exactly the event an auditor looks for.

Deleting a page sets `deleted_at` (trash). Purging removes the row, which cascades
to its versions, attachments, shares, links and subpages.

## API

Everything lives under `/api`. Authentication uses an httpOnly cookie set on login
or register. `GET /api/public/{token}` and `/api/healthz` are the only unauthenticated
endpoints.

Paid endpoints answer `402 Payment Required` without a license key; they are
marked below.

### Auth and license

```
POST   /auth/register                     create account (first one becomes admin)
POST   /auth/login                        sign in
POST   /auth/logout                       sign out
GET    /auth/me                           current user
GET    /lizenz                            which extras are unlocked
```

### Pages

```
GET    /pages                             flat list for the sidebar tree
GET    /pages/shared                      pages other users shared with you
GET    /pages/trash                       deleted pages
POST   /pages                             create (optional parentId, spaceId)
GET    /pages/{id}                        full page incl. tags, favorite, permission
PUT    /pages/{id}                        update; send basis to detect conflicts (409)
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
PUT    /spaces/{id}/oeffentlich           open to the whole instance: nein|lesen|schreiben
GET    /spaces/{id}/export[?format=]      ZIP of Markdown, or format=pdf|word as one document

GET    /search?q=&space=&tag=&tage=&wer=  full text search, optionally narrowed

GET    /tags                              list tags
POST   /tags                              create
DELETE /tags/{id}                         delete

POST   /import                           Markdown/HTML files or a ZIP, multipart; parentId/spaceId optional
POST   /import  (vorschau=1)              the same, but only reports the tree it would create

GET    /postfach[?ungelesen=1]            inbox entries, newest first
GET    /postfach/anzahl                   unread count, for the sidebar badge
POST   /postfach/gelesen                  mark all read
POST   /postfach/{id}/gelesen             mark one read
DELETE /postfach                          drop the read ones

GET    /pages/{id}/markdown               the page as Markdown
GET    /pages/{id}/pdf                    the page as PDF (extra)
GET    /pages/{id}/word                   the page as .docx (extra)

GET    /system/konfig                     config.conf, credentials masked (admin only)
PUT    /system/konfig                     check or write it (admin only)
POST   /system/neustart                   end the process so its supervisor restarts it
POST   /system/papierkorb                 purge the whole instance's trash (admin only)

GET    /users                             list accounts (admin only)
POST   /users                             create an account (admin only)
DELETE /users/{id}                        delete an account (admin only, not yourself)
PUT    /users/{id}/role                   change role (admin only)

GET    /favorites                         favorited pages
GET    /search?q=                         search titles and content
GET    /healthz                           liveness probe
```

### Comments  ·  paid: `kommentare`

```
GET    /pages/{id}/kommentare             the whole thread, oldest first
POST   /pages/{id}/kommentare             comment, or reply via elternId
PUT    /kommentare/{id}                   edit own comment
DELETE /kommentare/{id}                   empty the text, keep the shell
POST   /kommentare/{id}/erledigt          mark thread settled, or reopen it
```

### Audit trail  ·  paid: `pruefspur`  ·  admin only

```
GET    /pruefspur                         newest first; filter by aktion, akteur, objekt
GET    /pruefspur/aktionen                which action names actually occur
```

Recording runs on every installation regardless of the license — only reading
is paid. A trail with a hole over the unlicensed period would not be one.

## Project Structure

```
config.conf                    every setting, documented in place
LICENSING.md                   what is free and what needs a key

backend/                       Go API
  main.go                      router, route table, server bootstrap
  premium/                     the license check; see premium/README.md
    README.md                  how keys are issued
    lizenz/pruefer.go          verifies a key against the built-in public key
    cmd/schluessel/            issues keys; the only place the private key is used
  internal/config              config.conf plus environment, with defaults
  internal/db                  connection pool, schema creation and migration
  internal/auth                JWT issuing and password hashing
  internal/lizenz              the gate: asks whoever registered as verifier
  internal/middleware          cookie auth
  internal/einlesen            Markdown and HTML to editor blocks: the import side
  internal/handlers
    auth.go                    register, login, logout, me
    pages.go                   CRUD, tree, trash, restore, purge, conflicts
    versions.go                snapshots and rollback
    attachments.go             upload, download, delete
    einfuhr.go                 import: archive, page tree, link rewriting
    postfach.go                the inbox and what fills it
    papierkorb.go              the trash and its expiry sweep
    kommentare.go              comment threads
    pruefspur.go               audit trail: writing and reading
    volltext.go                plain text extraction for the search index
    lizenz.go                  the 402 gate and the license status endpoint
    sharing.go                 public links
    access.go                  per-user shares and permission checks
    links.go, backlinks.go     explicit page links
    graph.go                   graph nodes and edges
    spaces.go, tags.go         organisation and full text search
    users.go                   admin account management
frontend/                      React SPA (Vite + TypeScript)
  src/api                      typed API client
  src/lizenz.tsx               which extras are unlocked, asked once
  src/components               Sidebar, PageTree, Editor, Attachments,
                               QuickView, Kommentare, VersionPanel,
                               ShareDialog, LocalGraph, SpaceRechte, Einfuhr
  src/pages                    Login, Register, Workspace, PageView, PostfachView,
                               PublicPage, TrashView, GraphView, AdminView,
                               PruefspurView (shown as "Protokoll")
docker-compose.yml             backend + frontend
docker-compose.db.yml          optional: bundled PostgreSQL
docker-compose.minio.yml       optional: bundled MinIO for attachments
```


## Local Development

```bash
# backend: needs a local PostgreSQL, or point DATABASE_URL at the compose db
cd backend && go run .

# frontend: proxies /api to localhost:8080
cd frontend && npm install && npm run dev
```

## Licensing

**Business Source License 1.1** — see [LICENSE](LICENSE), explained in
[LICENSING.md](LICENSING.md).

Not an OSI open-source license, but not a closed one either: the source is
open, and the Additional Use Grant explicitly permits production and commercial
use of the core. On **2030-08-19** the restriction lapses and Apache 2.0 takes
over.

**Free to run:** editor, nested pages, spaces, tags, favourites, trash, full
text search over pages, backlinks, knowledge graph, accounts and roles.

**Needs a key:** version history, attachments, sharing and public links, audit
trail, groups and space permissions, SSO via OIDC, LDAP/Active Directory,
attachment search, space export, templates, comments, conflict detection.

Locked endpoints answer `402 Payment Required` and the browser hides the
corresponding controls. Hiding is a courtesy to the reader — the refusal is
what enforces it.

Build without the paid half:

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

Apply a key:

```
NEXORA_LIZENZ='<key>'      # or lizenz = <key> in config.conf
```

A missing or invalid key is never fatal: the server logs why and runs on the
free feature set. Keys are issued with
[`backend/premium/cmd/schluessel`](backend/premium/README.md).

Copyright 2026 Jonas Groll.
