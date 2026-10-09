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

[Unreleased]: https://github.com/Dschonas04/Nexora/compare/v2.2.1...main
[2.2.1]: https://github.com/Dschonas04/Nexora/releases/tag/v2.2.1
[2.2.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.2.0
[2.1.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.1.0
[2.0.1]: https://github.com/Dschonas04/Nexora/releases/tag/v2.0.1
[2.0.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.0.0
