# Changelog

Notable changes per release, newest first, after
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow
[semantic versioning](https://semver.org/lang/en/): the major number moves when
an upgrade needs a hand, the minor when something is added, the patch when
something is repaired.

Every release ships as images — `ghcr.io/dschonas04/nexora-backend`, `-frontend`
and `-pki`, for amd64 and arm64.

## [2.0.0] — 2026-09-17

First published release. Nexora ran in production before this, but installing
it meant building it; from here on there are images and a tag.

### Added

- **Published images** for amd64 and arm64, tagged `2.0.0`, `2.0` and `latest`,
  no account needed to pull them. `docker-compose.abbild.yml` pulls instead of
  building, `docker-compose.stack.yml` carries Nexora and its database in one
  file for Portainer, Coolify, Dokploy or a bare machine.
- **[Comparison](docs/comparison.md)** against Notion, Outline, Docmost,
  Wiki.js, BookStack and AppFlowy, including the cases where one of those is
  the better answer.
- **Screenshots and a live demo** in the README —
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

The core is BUSL 1.1 — run it in production, commercially, without paying
anyone. Twelve extras (audit trail, groups, SSO, LDAP and more) need a key.
Markdown, PDF and Word export are free regardless, because the way out of a
system must never sit behind one. On 2030-08-19 the whole thing becomes
Apache 2.0.

[2.0.0]: https://github.com/Dschonas04/Nexora/releases/tag/v2.0.0
