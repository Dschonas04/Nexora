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

## Documentation

The [`docs/`](docs/README.md) directory carries the long form: the
[architecture](docs/architecture.md) after arc42 with C4 diagrams, plus
reference documents for the [API](docs/api.md), the
[configuration](docs/configuration.md), the [data model](docs/data-model.md),
[operations](docs/operations.md) and [development](docs/development.md).
[Capacity](#capacity) below carries measured throughput and latency figures.

Licensing is in [LICENSING.md](LICENSING.md); what Nexora ships from other
people, and under which terms, is listed in
[THIRD-PARTY.md](THIRD-PARTY.md), which also carries the commands to verify the
inventory yourself.

## Stack

| Layer    | Tech                                                     |
| -------- | -------------------------------------------------------- |
| Backend  | Go 1.25+, chi router, pgx, JWT (httpOnly cookie), bcrypt  |
| Frontend | React 18, Vite, TypeScript, BlockNote editor             |
| Database | PostgreSQL 16                                            |
| Delivery | Docker Compose (nginx serves the SPA and proxies `/api`) |

## Capacity

Measured on one 12-core host, backend and PostgreSQL 16 on the same machine,
against a throwaway database seeded with 40 accounts and 2,000 pages.

The unit is an **operation**, not a request: opening a page is three calls (the
tree, the page, its backlinks), so an operation averages 2.3 HTTP requests. The
mix is 50 % open a page, 18 % search, 14 % load the tree, 8 % browse tags and
favourites, and **10 % save** — a save being the expensive one, since it
snapshots the previous version, recomputes the search column and writes an audit
row. Every account works on its own pages, so the permission check runs its full
path rather than the shortcut an admin takes.

| Concurrent in flight | Operations/s | p50 | p95 | Errors |
| --- | --- | --- | --- | --- |
| 1 | 213 | 5.2 ms | 8.8 ms | 0 |
| 20 | 4,322 | 5.2 ms | 8.9 ms | 0 |
| 100 | 4,293 | 31 ms | 39 ms | 0 |
| 400 | 4,712 | 119 ms | 138 ms | 0 |
| 1,600 | 4,635 | 496 ms | 554 ms | 0 |
| 3,200 | 4,499 | 1.01 s | 1.14 s | 0 |

Throughput is saturated at roughly 20 concurrent requests and holds at about
4,600 operations per second from there on; beyond that, added concurrency only
queues, and latency rises linearly with it. **No errors at any level**, up to
3,200 concurrent — it gets slower under load, it does not fall over.

Translated through a think time of 20 seconds between actions, that is tens of
thousands of simultaneously active users on paper. Take the paper part
seriously: this was measured over loopback without TLS, with a 2,000-page
database that fits entirely in cache, and with the load generator competing for
the same cores. What the figures do support is the modest claim: several hundred
people working at the same time are unremarkable, and it takes more than 800
concurrent requests before p95 passes 300 ms.

**One tuning knob is worth knowing.** `DATABASE_URL` carries no
`pool_max_conns`, so pgx defaults to one connection per core. Raising it to 50
gives 5,895 operations/s at 100 concurrent instead of 4,293, with p95 dropping
from 39 ms to 32 ms — 37 % more throughput for one query parameter. PostgreSQL
allows 100 connections by default, so there is room. RAM is *not* the knob:
`shared_buffers` sits at the 128 MB default and a real instance measured here
held a 10 MB database at a 99.98 % cache hit ratio. Give PostgreSQL more memory
when the database approaches a gigabyte, not before.

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
  attached to its folder's page rather than dropped. An archive can bring its
  own space instead of merging into an existing one, which is what makes an
  exported space importable again as a whole. Front matter supplies title,
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
- **Sessions you can see and end**: every sign-in is a row in the database, so a
  lost device can be locked out without logging everyone else out. Sessions
  renew themselves while they are in use and expire when they are not; logging
  out revokes the token rather than only clearing the browser
- **Sign in with a username**: an account has a login name next to its address
  and either of the two gets you in — the `@` decides which one was typed. It is
  handed out on registration, derived from the address when nobody picks one, so
  the second way is there without anybody having to switch it on
- **Sign in through Keycloak or a directory**: OIDC against any provider that
  publishes a discovery document, or a direct bind against LDAP / Active
  Directory. Both link by verified email address and never take over an account
  that has its own password
- **Word files**: a `.docx` attachment opens in the viewer and, with write
  access, can be edited and written back. Text, headings, lists and tables
  survive; headers, styles, comments and images do not — and the interface says
  so before you start
- **Marking up a PDF**: a PDF attachment can be highlighted in colour and given
  notes right in the viewer. The marks are drawn into the file itself, so they
  survive every other reader and every printout, and notes become real PDF
  annotations carrying their author. The marked-up file **replaces** the old
  one under the same id — every link to it keeps working, and the unmarked
  version is gone afterwards; the interface says so before you save
- **Your own profile**: a display name and a picture, set by the account itself
  rather than by an administrator. The picture is cropped and scaled to 256 × 256
  in the browser before it goes up, and the server accepts it only after
  decoding it as an actual image — the claimed content type decides nothing
- **Audit trail**: who did what, when — sign-ins including the failed ones,
  accounts, pages, trash, permanent deletion, shares and public links. Entries
  survive the deletion of the page or account they refer to, because deleting is
  exactly what an auditor comes looking for

## Quick Start

```bash
cp .env.example .env
cp config.beispiel.conf config.conf
# edit .env: set POSTGRES_PASSWORD and a long random JWT_SECRET
docker compose up -d --build
```

`config.conf` is deliberately not tracked: it holds credentials and is edited
from the maintenance page, so a tracked copy would be overwritten on every
deployment.

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

# plus Redis as a cache in front of the database
COMPOSE_FILE=docker-compose.yml:docker-compose.db.yml:docker-compose.redis.yml
```

An existing MinIO or S3 is wired up through the `NEXORA_S3_*` settings instead
of the third file. An object store that has been configured and does not answer
at startup stops the boot; `s3_rueckfall = ja` allows the disk as a stopgap.

### Where the attachments lie

The bytes of an attachment are the one part of Nexora that does not live in the
database. By default they lie in a volume Docker manages, `nexora_files`. Two
settings move them:

```bash
# a directory of the host, a disk of its own, a mount of the file server
NEXORA_ANHANG_ORT=/srv/nexora/anhaenge     # .env: what gets mounted
anhang_verzeichnis = /data/attachments     # config.conf: the path inside

# or not on any disk at all: into a bucket
s3_aktiv = ja
```

The directory has to belong to uid/gid `10001`, the account the service runs
under in the container. Changing the setting does not move the files that are
already there — carry them over first, then restart.

The same instance also answers over TLS on **https://localhost:3443** with a
certificate the container issues on first start (825 days, stored in a volume so
a rebuild does not change it). Browsers will warn about the issuer — nobody
issues a trusted certificate for an address in a private network. Drop a real
certificate into the `nexora_tls` volume as `zertifikat.pem` / `schluessel.pem`
and nothing is generated. The session cookie is marked `Secure` whenever the
request arrived over TLS.

Open **http://localhost:3000** (or whichever `PORT` you set) and create the first
account, which becomes the workspace admin.

## Licensing tiers

Four tiers, each containing the smaller ones:

| Tier | adds |
|---|---|
| `free` | pages, search, trash, Markdown import and export |
| `advanced` | version history, attachments, comments |
| `pro` | sharing and public links, writing on a page together, conflict detection, PDF/Word export, search inside attachments |
| `business` | groups, audit trail, OIDC, LDAP |

Keys are Ed25519-signed and verified offline, which is why they carry an expiry
of at most a year: an issued key cannot be revoked, so the date is the only
lever there is. A key can name a tier, a list of individual extras, or both.

Importing a key works from the maintenance page of any installation — it is
checked, stored in the database and takes effect at once. *Issuing* a key needs
the private signing key in `NEXORA_SIGNIERSCHLUESSEL`; without it the endpoint
answers 501 and the section does not appear. That asymmetry is the whole point:
verification is offline, so the private key is the only thing separating a
customer from a self-issued licence.

## Configuration

Everything is read from **`config.conf`** (copied from
`config.beispiel.conf`), which documents itself: 245 lines,
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
| Server | `port`, `daten_verzeichnis`, `anhang_verzeichnis`, `oeffentliche_url` |
| Database | `datenbank_url` |
| Sessions | `jwt_geheimnis`, `sitzung_stunden` |
| License | `lizenz` |
| Registration | `registrierung_offen`, `erlaubte_domaenen` |
| Search | `such_woerterbuch` |
| Attachments | `max_anhang_mb` |
| Trash | `papierkorb_tage` |
| Object storage | `s3_aktiv` and eight more |
| LDAP / AD | `ldap_aktiv` and ten more |
| OIDC | `oidc_aktiv` and eight more |

### Warnings on start

Dangerous defaults are named on every boot without preventing it — a homelab
install with the default secret should still run, it should just be impossible
to miss that it did:

```
ACHTUNG: jwt_geheimnis steht auf der Vorgabe, jede Sitzung ist fälschbar
ACHTUNG: LDAP ohne TLS, Zugangsdaten gehen im Klartext über das Netz
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
users        id, email, name, benutzername, password_hash, role, created_at,
             bild, bild_mime, bild_stand
spaces       id, owner_id, name, created_at
pages        id, owner_id, parent_id, space_id, title, content (jsonb),
             content_text, such_tsv (generated), icon, is_public,
             public_token, sort_order, deleted_at, created_at, updated_at
page_versions  id, page_id, title, content, icon, author_id, created_at
attachments    id, page_id, owner_id, filename, mime, size, created_at
page_shares    page_id, user_id, permission ('read' | 'edit'), created_at
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
or register. Unauthenticated: the sign-in routes themselves, `GET /api/public/{token}`
with its file route, and `/healthz`. The complete reference, including the routes this
overview leaves out, is in [docs/api.md](docs/api.md).

Paid endpoints answer `402 Payment Required` without a license key; they are
marked below.

### Auth and license

```
POST   /auth/register                     create account (first one becomes admin)
POST   /auth/login                        sign in ({kennung, password}: email or username)
POST   /auth/logout                       sign out
GET    /auth/me                           current user
POST   /auth/passwort                     change your own password ({alt, neu})
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
POST   /pages/{id}/attachments            upload (multipart field "file", max 25 MiB;
                                          Linux executables are refused, by their
                                          first bytes and not by their name)
GET    /pages/{id}/attachments/{attId}    download
DELETE /pages/{id}/attachments/{attId}    delete
```

### Sharing

```
POST   /pages/{id}/share                  publish read-only, returns a token
DELETE /pages/{id}/share                  revoke the public link
GET    /pages/{id}/shares                 who this page is shared with
POST   /pages/{id}/shares                 share with a user ('read' or 'edit')
DELETE /pages/{id}/shares/{userId}        revoke a user's access
GET    /public/{token}                    read-only public page (no auth)
```

### Writing together

```
GET    /echtzeit/{id}                     WebSocket: the session for one page
GET    /pages/{id}/mitschreibende         how many are sitting at this page
GET    /system/mitschrift                 which pages are open, and who is in them
```

Whoever may **edit** a page shared with them writes on it at the same time as
everyone else: the changes appear as they are typed, each with the name and the
caret of the person making them. The service passes the packets on and keeps no
document of its own; the browsers merge the text between them, so a restart
costs nothing and two people in the same sentence both keep what they wrote.
Saving to the database is done by exactly one of them, the one with the lowest
client id in the room — and when that browser leaves, the next takes over.

A proxy in front must pass WebSocket upgrades through to `/api/echtzeit/`.

### Watching machines  ·  admin only

```
GET    /system                            the services around this one: reachable, version, latency
GET    /system/rechner                    machines you keep an eye on, freshly probed
POST   /system/rechner                    add one ({name, ziel}: host:port or http(s)://)
PUT    /system/rechner/{id}               change one
DELETE /system/rechner/{id}               remove one
```

Two lists in the settings under **System**. The first is what Nexora needs to
run and therefore knows by itself — database, cache, object store, sign-in
provider — each with its state, version and response time. The second is what you
enter: the machines around it.

Nexora only knocks on those, and everything in the table is what it saw while
doing so: whether something answers, how long that took, the version the service
names itself (an SSH greeting, a `Server` header) and, on TLS targets, how many
days its certificate still has. No monitoring system to set up, no agent on the
far side, no key to your machines.

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
PUT    /spaces/{id}/farbe                 the colour the space wears in sidebar and graph
POST   /spaces                            create
PUT    /spaces/{id}                       rename
DELETE /spaces/{id}                       delete (pages keep existing, space_id nulled)
PUT    /spaces/{id}/oeffentlich           open to the whole instance: nein|lesen|schreiben
GET    /spaces/{id}/export[?format=]      ZIP of Markdown, or format=pdf|word as one document

GET    /search?q=&space=&tag=&tage=&wer=  full text search, optionally narrowed

GET    /tags                              list tags
POST   /tags                              create
DELETE /tags/{id}                         delete

GET    /pages/{id}/attachments/{attId}/word    a .docx as editor blocks
PUT    /pages/{id}/attachments/{attId}/word    write the blocks back as .docx
PUT    /pages/{id}/attachments/{attId}/pdf     replace a PDF with a marked-up one

PUT    /profil                            your own display name
PUT    /profil/bild                       your own profile picture (raw bytes)
DELETE /profil/bild                       remove it
GET    /users/{id}/bild                   an account's picture, 404 when none
GET    /auth/sso                         which sign-in methods this instance offers
GET    /auth/oidc/start                  begin an OIDC sign-in (redirects to the provider)
GET    /auth/oidc/zurueck                the provider's callback
POST   /auth/ldap                        sign in against LDAP / Active Directory
GET    /sitzungen                        the signed-in account's stored sessions
DELETE /sitzungen                        end every session but the current one
DELETE /sitzungen/{id}                   end one session, effective immediately
PUT    /system/lizenz                    import a license key, effective at once (admin)
POST   /system/lizenz/ausstellen        issue a key — only where a signing key is present
POST   /import                           Markdown/HTML files or a ZIP, multipart; parentId/spaceId
                                         optional, or neueAblage=<name> to create a space for it
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
PUT    /users/{id}/benutzername           set the login name (the account itself or an admin)
PUT    /users/{id}/passwort              set a password for a forgotten one (admin only)

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
docs/                          architecture (arc42 + C4), API, configuration,
                               data model, operations, development

backend/                       Go API
  main.go                      router, route table, server bootstrap
  premium/                     the license check; see premium/README.md
    README.md                  how keys are issued
    lizenz/pruefer.go          verifies a key against the built-in public key
    cmd/schluessel/            issues keys; the only place the private key is used
  internal/config              config.conf plus environment, with defaults
  internal/db                  connection pool, schema creation and migration
  internal/auth                JWT issuing and password hashing
  internal/vertrauen           whom the service believes on the way out: the
                               stack's own authority, added to the public ones
  internal/lizenz              the gate: asks whoever registered as verifier
  internal/middleware          cookie auth
  internal/einlesen            Markdown and HTML to editor blocks: the import side
  internal/dok                 typesetting: PDF and .docx written by hand
  internal/ablage              where attachments are stored: disk or S3
  internal/handlers
    auth.go                    register, login, logout, me
    benutzername.go            the login name: rules, derivation, changing it
    passwort.go                changing your own password, and an admin resetting one
    pages.go                   CRUD, tree, trash, restore, purge, conflicts
    versions.go                snapshots and rollback
    attachments.go             upload, download, delete
    einfuhr.go                 import: archive, page tree, link rewriting
    sitzungen.go               stored sessions: list, revoke, renew, sweep
    sso.go / ldap.go           sign-in through OIDC or a directory
    word.go                    .docx attachments: read as blocks, write back
    redis.go                   optional shared cache in front of the database
    lizenzverwaltung.go        import and issue license keys
    postfach.go                the inbox and what fills it
    papierkorb.go              the trash and its expiry sweep
    kommentare.go              comment threads
    pruefspur.go               audit trail: writing and reading
    volltext.go                plain text extraction for the search index
    lizenz.go                  the 402 gate and the license status endpoint
    sharing.go                 public links
    access.go                  per-user shares and permission checks
    links.go, backlinks.go     explicit page links
    erwaehnungen.go            who may be named with @ in a comment
    graph.go                   graph nodes and edges
    export.go, exportdateien.go  a whole space as ZIP, PDF or Word
    oeffentlich.go             what a public link is allowed to show
    dateiausgabe.go            the headers an attachment is handed out with
    verbund.go                 the state of database, cache and file store
    rechner.go                 the machine watch list: knock, and read what comes back
    spaces.go, tags.go         organisation and full text search
    users.go                   admin account management
frontend/                      React SPA (Vite + TypeScript)
  src/api                      typed API client
  src/lizenz.tsx               which extras are unlocked, asked once
  src/design.tsx               the three colour schemes
  src/klappen.ts               closing a menu on a click outside it
  src/components               Sidebar, PageTree, Editor, Attachments,
                               QuickView, Kommentare, VersionPanel, ShareDialog,
                               Grafbild, LocalGraph, SpaceRechte, Einfuhr,
                               Rueckfrage, PasswortDialog, Fehlergrenze
  src/pages                    Login, Register, Workspace, PageView, PostfachView,
                               PublicPage, TrashView, TagView, GraphView, AdminView,
                               GruppenView, EinstellungenView,
                               PruefspurView (shown as "Protokoll")
pki/                           the stack's own certificate authority
  erzeuge.sh                   runs once at start-up, issues one certificate per
                               service, and leaves alone what is already there
docker-compose.yml             pki + backend + frontend
docker-compose.db.yml          optional: bundled PostgreSQL
docker-compose.minio.yml       optional: bundled MinIO for attachments
docker-compose.redis.yml       optional: Redis as a cache (never the source of truth)
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
attachment search, space export, comments, conflict detection, writing on a
page together.

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
