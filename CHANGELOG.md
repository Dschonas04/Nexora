# Changelog

Notable changes per release, newest first, after
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow
[semantic versioning](https://semver.org/lang/en/): the major number moves when
an upgrade needs a hand, the minor when something is added, the patch when
something is repaired.

Every release ships as images for amd64 and arm64, to two registries with the
same tags: `ghcr.io/dschonas04/nexora-{backend,frontend,pki}` and, since 2.0.1,
`dschohnas/nexora-{backend,frontend,pki}` on Docker Hub. See
[Where the images are](README.md#where-the-images-are).

## [Unreleased]

### Removed

- **Tasks**: the Tasks view, `/api/tasks`, inbox messages for tasks, the
  highlighting of `@username` and `📅` dates and the "/" menu entries for them.
  Checklists stay as ordinary checklists.
- **Gantt charts and timelines** are no longer drawn; such a diagram shows a short
  note instead. Every other Mermaid diagram stays.
- The built-in **project page** template with its timeline: Nexora is a wiki, not a
  project tool. Meeting notes, how-to and saved templates stay.

## [3.1.2]: 2026-10-22

### Fixed

- **Imported tasks show up under Tasks.** Pages from the importer or written
  through the API had blocks without ids, and a checklist item without an id
  never reached "My tasks". Both now give every block an id on the way in, and
  at startup Nexora fills in the ids of checklist items imported before.

## [3.1.1]: 2026-10-22

### Fixed

- **Import keeps code blocks.** A fenced block in Markdown and a `<pre>` in an
  HTML export now arrive as one code block with their language, instead of a
  fixed-width paragraph per line. A ```mermaid fence from Obsidian or a Git
  wiki is therefore drawn as a diagram, and the Markdown export writes it back
  the same way.

## [3.1.0]: 2026-10-22

Templates, tasks with due dates, diagrams, version comparison, API keys and
webhooks, an installable app, and install folders for Docker and Kubernetes.
One thing needs a hand when upgrading a Compose installation: the compose
files moved into `docker/`, see below.

### Added

- **Templates.** An empty page offers to start from a template: three built in
  (meeting notes, how-to, project page, in the reader's language) and every
  template one saved. "Save as template" in a page's ⋯ menu turns its content
  into one; an admin can offer it to the whole instance. Templates are a table
  of their own this time and appear nowhere else -- not in the tree, the search,
  the graph or the export. Applying one copies it; changing a template later
  leaves the pages made from it alone.
- **Comparing versions.** Every entry in the version history has a "Compare"
  button: what changed between that version and the current state, or the next
  version. Line by line as a reader sees the page, and word by word inside a
  changed line, unchanged stretches folded away.
- **Table of contents.** ⋯ → Contents shows the page's headings beside it,
  current while typing; a click jumps to the heading.
- **Watching a page.** "Watch" beside "Favourite": when somebody else changes the
  page, an inbox message (and an e-mail, where switched on). One message until
  it is read, not one per autosave.
- **Diagrams.** A code block in the language `mermaid` gets its picture drawn
  below it -- flowcharts, sequences, Gantt and everything else Mermaid knows.
  `/Diagram` inserts one. Still a code block underneath, so Markdown export and
  import carry it as ```mermaid. Drawn as an image: no script from the source
  text runs in the page.
- **Tasks with due dates.** A checklist item with `@username` is a task for that
  person, `📅 2026-10-30` (or `📅 30.10.2026`) gives it a due date; both are
  highlighted, the date coloured by how close it is. The new **Tasks** view
  gathers them from every readable page: "For me" or "On my pages", ticked off
  right there. Being named in a task lands in the inbox. In the editor "@" now
  offers people as well as pages, and "/" has "Task with due date" and "Due
  date".
- **API keys.** My account → API keys: personal keys for scripts and n8n,
  `Authorization: Bearer nxr_…`, read-only or read and write, with an expiry.
  Only a fingerprint is stored. A key cannot change the password or the second
  factor, end sessions or make another key.
- **Webhooks.** Administration → Integrations: addresses called on
  `page.created`, `page.updated`, `page.deleted`, `page.restored` and
  `comment.created`, signed with HMAC-SHA256 in `X-Nexora-Signature`.
  `page.updated` goes out once a page has been quiet for 30 seconds. A test
  button shows what the receiver answered.
- **App mode and offline reading.** Nexora installs as an app (PWA) on phone and
  desktop. A service worker keeps the pages one opened readable without a
  connection; a stripe says when one is offline and that nothing is saved then.
  Signing out empties that cache.

### Changed

- **Install folders `docker/` and `kubernetes/`.** All compose files, the
  `.env.example` and `postgres-18.sh` moved from the top of the repository into
  `docker/`; `deploy/kubernetes/` is now `kubernetes/`. Each folder has an
  `install.sh` that writes random passwords, starts Nexora from the published
  images and waits until it answers -- `./docker/install.sh` (also `single` or
  `redis`) and `./kubernetes/install.sh wiki.example.org`. Running it again is
  the update. The Kubernetes manifests now bring a PostgreSQL of their own
  (`postgres.yaml`; remove it for an existing database).
  **Upgrading a Compose installation:** move `.env` and `config.conf` into
  `docker/` and run compose from there. The project name stays `nexora`, so the
  containers and volumes are the same ones. Links to the raw stack file change
  to `…/main/docker/docker-compose.stack.yml`.
- **The single container speaks HTTPS only.** Port 80 serves nothing any more
  and redirects every request to HTTPS with a 308, so a POST stays a POST and
  the login form never crosses the network in the clear. Open
  `https://localhost:3443`; `docker-compose.single.yml` now publishes 443 as
  3443 and keeps 3000 only for the redirect. Whoever publishes 443 under another
  port sets `NEXORA_HTTPS_PORT`, and `NEXORA_HTTPS_ONLY=0` brings plain HTTP
  back. The stack is unchanged; it can opt in with the same two variables.

## [3.0.0]: 2026-10-17

The major number moves because scripts talking to the API have to be adjusted:
the JSON field names are English now. Nothing to do for whoever only uses the
interface.

### Changed

- **The API speaks English.** Paths, query parameters, form fields and JSON
  field names are English now: `/api/pruefspur?tage=7` is
  `/api/audit-log?days=7`, `{"inhaber": …}` is `{"holder": …}`, and so on;
  [docs/api.md](docs/api.md) lists all of them. The old German **paths and
  query parameters** keep working, they are translated before routing. The
  **JSON fields** have no fallback: a script that reads or sends German field
  names has to be adjusted. Untouched: the values (`lesen`, `schreiben`, the
  feature names in licence keys), the settings keys and the details stored in
  the audit trail.
- **OIDC callback:** the redirect URI is now `/api/auth/oidc/callback`. The
  old `/api/auth/oidc/zurueck` still arrives, but the provider has to allow the
  new address, since Nexora now sends that one.

## [2.3.0]: 2026-10-17

### Changed

- **PostgreSQL 18.** The stack, the single container and the CasaOS and Umbrel
  templates now ship PostgreSQL 18.6 instead of 16. The single container
  carries existing data over by itself on its first start; a compose stack
  runs `deploy/postgres-18.sh` once, see
  [Upgrading](docs/operations.md#from-22-to-23-postgresql-16-to-18). `pg_dump`
  in the backend image is 18 as well and still backs up older servers.
- **New license: open core.** The core is now free software under the
  **GNU AGPL 3.0**; the paid extras (sharing, real-time editing, export,
  attachment search, groups, audit trail, SSO, LDAP) are under the new
  **Nexora Enterprise License** and are marked in their file headers. Prices
  are in [LICENSING.md](LICENSING.md#prices): Pro 200 € per installation and
  year, Business on request. Releases up to 2.2.1 stay under BUSL 1.1.

## [2.2.1]: 2026-10-09

### Security

- **Single container: the database asks for a password.** 2.2.0 let every
  local connection into PostgreSQL without one, the superuser included, so a
  hole in the service would have opened the whole database. The service now
  signs in with a password of its own, generated on the first start and kept in
  `/data/db_password`; the superuser is only reachable as the system user
  `postgres`. An installation from 2.2.0 is switched over on its next start,
  nothing to do by hand.

## [2.2.0]: 2026-10-09

> The `nexora-single` image of this version has been withdrawn from both
> registries: its database accepted local connections without a password. Use
> 2.2.1 or later. The other three images are unaffected.

### Added

- **Single container.** `nexora-single` holds PostgreSQL, the service and nginx
  in one image with one volume: `docker run -p 3000:80 -v nexora_data:/data
  ghcr.io/dschonas04/nexora-single`. It runs the free Standard scope only,
  generates its session secret and database password on the first start and is meant for a first
  try or a small team, not for load. `docker-compose.single.yml` does the same
  with Compose.
- **Health checks.** The backend and frontend containers report their state:
  the backend asks its own `/healthz`, which pings the database, the frontend
  asks its nginx. `docker compose ps`, Portainer and Coolify show "healthy"
  instead of "running", and the frontend starts once the backend is healthy.
  The backend binary has a `nexora healthcheck` subcommand for that.
- **French.** The interface speaks French as a third language, picked in the
  account settings or taken from the browser. The editor's own menus follow.
  Notification mails for French accounts go out in English. The text probe in
  CI fails on any pair without a French entry.
- **Kubernetes manifests.** `deploy/kubernetes/` brings Nexora up with
  `kubectl apply -k` against an existing PostgreSQL: deployments, services,
  ingress, a volume for the attachments and a secret. See the operations
  document.
- **Unraid templates.** `templates/unraid/` holds container templates for the
  backend and the frontend, for an Unraid server with its own PostgreSQL.
- **Release notes from this file.** Every tag's release on GitHub carries its
  section of this changelog.

### Changed

- The remaining German names of files and environment variables are English
  now (`NEXORA_ATTACHMENT_DIR`, `NEXORA_SERVICE_SCHEME`, `NEXORA_SERVICE_PORT`,
  `PKI_DIR`, `PKI_DAYS`, `PKI_SERVICES`). The old names are still read, so an
  existing `.env` keeps working.

### Fixed

- The frontend started without the stack's certificate authority only in
  theory: with `NEXORA_SERVICE_SCHEME=http` and no `/pki/ca.crt`, nginx
  refused to start over the missing file. It now drops the certificate check
  when there is no certificate to check.
- The frontend's nginx asked Docker's DNS at 127.0.0.11 for the service, fixed
  in the configuration. It now takes the container's own DNS server from
  `/etc/resolv.conf`, and the service's host name from `NEXORA_SERVICE_HOST`
  (default `backend`), so it also runs on Kubernetes.

## [2.1.0]: 2026-09-18

### Changed

- **Standard needs no key, and holds more.** Version history, attachments,
  comments and conflict detection run on every installation now, without a
  licence key, what a wiki needs so that nobody loses text or keeps files
  somewhere else. Keys are only for the two tiers above:
  - **Pro**, sharing and public links, writing on a page together, PDF and Word
    export and space export, search inside attachments.
  - **Business**, groups and space permissions, audit trail, OIDC, LDAP.
- The licence's Additional Use Grant says so: those four features moved from the
  list of paid ones to the list of free ones. The `advanced` tier, which sold
  exactly them, stays readable so that issued keys remain valid, and adds
  nothing any more.
- The interface asks the server what is unlocked rather than assuming that
  "no valid licence" means "nothing unlocked", and names the scope "Standard".

### Fixed

- The documentation claimed in several places that PDF and Word export are
  free. They are not and were not: Markdown export is free, the typeset
  exports belong to Pro.

## [2.0.1]: 2026-09-18

No change to Nexora itself: the images carry the same code as 2.0.0. What is
new is where to get them and how to put them somewhere.

### Added

- **Docker Hub.** The images are now published to
  `docker.io/dschohnas/nexora-backend`, `-frontend` and `-pki` as well as to
  the GitHub registry, with the same tags.
- **[Project page](https://dschonas04.github.io/Nexora/)** with a half-minute
  walkthrough.
- **Templates** for CasaOS and Umbrel, and the way in for Unraid through the
  Compose Manager, in [`templates/`](templates/README.md).
- A walkthrough GIF and English screenshots in the README; the demo carries two
  English spaces next to the German ones.

## [2.0.0]: 2026-09-17

First published release. Nexora ran in production before this, but installing
it meant building it; from here on there are images and a tag.

### Added

- **Published images** for amd64 and arm64, tagged `2.0.0`, `2.0` and `latest`,
  no account needed to pull them. `docker-compose.image.yml` pulls instead of
  building, `docker-compose.stack.yml` carries Nexora and its database in one
  file for Portainer, Coolify, Dokploy or a bare machine.
- **[Comparison](docs/comparison.md)** against Notion, Outline, Docmost,
  Wiki.js, BookStack and AppFlowy, including the cases where one of those is
  the better answer.
- **Screenshots and a live demo** in the README,
  [nexora.jonasgroll.de](https://nexora.jonasgroll.de), reset every night.
- Issue templates, and a handful of small tasks marked as a good first one.

### The scope at this release

Nested pages in a block editor, spaces with per-user roles, `[[wiki links]]`
with automatic backlinks, a knowledge graph, version history, comments,
attachments on local disk or in any S3-compatible bucket, a self-emptying
trash, PostgreSQL full text search (optionally inside attachments), public
share links, inbox notifications by e-mail, two-factor authentication, import
from Obsidian, Notion and Confluence exports, and export as Markdown, PDF and
Word.

Measured on a 12-core host against 2,000 pages: about 4,600 operations per
second, p95 under 40 ms at 100 concurrent requests, no errors up to 3,200
concurrent. Method and caveats are in the [README](README.md#capacity).

### Licence

The core is BUSL 1.1, run it in production, commercially, without paying
anyone. Twelve extras (audit trail, groups, SSO, LDAP and more) needed a key at
this release; see 2.1.0 for what became free since. Markdown import and export
are free regardless, because the way out of a system must never sit behind a
key. On 2030-08-19 the whole thing becomes Apache 2.0.

[Unreleased]: https://github.com/Dschonas04/Nexora/compare/v3.1.2...main
[3.1.2]: https://github.com/Dschonas04/Nexora/releases/tag/v3.1.2
[3.1.1]: https://github.com/Dschonas04/Nexora/releases/tag/v3.1.1
[3.1.0]: https://github.com/Dschonas04/Nexora/releases/tag/v3.1.0
[3.0.0]: https://github.com/Dschonas04/Nexora/releases/tag/v3.0.0
[2.3.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.3.0
[2.2.1]: https://github.com/Dschonas04/Nexora/releases/tag/v2.2.1
[2.2.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.2.0
[2.1.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.1.0
[2.0.1]: https://github.com/Dschonas04/Nexora/releases/tag/v2.0.1
[2.0.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.0.0
