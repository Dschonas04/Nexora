# Licensing

Nexora is **open core**:

* **The core is free software under the [GNU AGPL 3.0](LICENSE).** Run it,
  change it, sell services around it, in production, in companies, without
  asking or paying anybody.
* **The paid extras are under the [Nexora Enterprise License](LICENSE-ENTERPRISE).**
  Their source is in the same repository and can be read, built and tested;
  running them in production needs a license key.

Which license a file is under is written in its header: Enterprise files carry
`SPDX-License-Identifier: LicenseRef-Nexora-Enterprise`, together with
everything in `backend/premium/`. Everything else is AGPL 3.0.

## Prices

| | Standard | Pro | Business |
|---|---|---|---|
| Price | **free** | **200 € per installation and year** | **on request** |
| Editor, nested pages, spaces, tags, trash | ✓ | ✓ | ✓ |
| Full text search, backlinks, knowledge graph | ✓ | ✓ | ✓ |
| Accounts, roles, two-factor sign-in | ✓ | ✓ | ✓ |
| Version history, attachments, comments, conflict detection | ✓ | ✓ | ✓ |
| Sharing with accounts and public links | | ✓ | ✓ |
| Editing a page together in real time | | ✓ | ✓ |
| PDF and Word export, space export | | ✓ | ✓ |
| Full text search inside attachments | | ✓ | ✓ |
| Groups and space permissions | | | ✓ |
| Audit trail | | | ✓ |
| Single sign-on via OIDC | | | ✓ |
| LDAP and Active Directory | | | ✓ |

**To buy a key**, write to **[info@jonasgroll.de](mailto:info@jonasgroll.de)**
with the name the key should be issued to and the tier. You get an invoice and,
once paid, the key.

Keys are entered under *Settings → License* and are checked offline; the
installation never calls home.

## What the tiers mean in the code

| Add-on | Tier | Name in the key | Without a key |
|---|---|---|---|
| Sharing and public links | Pro | `freigeben` | pages stay with their owner |
| Editing together | Pro | `echtzeit` | one person writes, the others see it on reload |
| Export as PDF and Word, space export | Pro | `export` | per-page export as Markdown |
| Full text in attachments | Pro | `anhangsuche` | search over pages, not over files |
| Groups and space permissions | Business | `gruppen` | per-page permissions as before |
| Audit trail | Business | `pruefspur` | recording continues, reading is locked |
| SSO via OIDC | Business | `sso` | sign-in with a password |
| LDAP and Active Directory | Business | `ldap` | sign-in with a password |

Version history, attachments, comments and conflict detection
(`versionen`, `anhaenge`, `kommentare`, `konflikte`) needed a key up to 2.0 and
are part of the Standard scope since 2.1. Keys that name them, or the old
`advanced` tier, stay valid; they simply no longer unlock anything the
installation would not have anyway.

The **audit trail** keeps recording even without a licence. Otherwise a gap would
open up after enabling it, right over the unlicensed period, and an audit trail
with a hole in it is not one.

The server rejects locked calls with `402 Payment Required`, and the interface
hides the matching controls. Hiding them is courtesy; the rejection is the
protection.

## The core alone

The key check lives in `backend/premium`, the gate itself in
`backend/internal/lizenz`. The gate knows nothing about signatures; it only
asks who has registered as a checker. That is why the core can also be built
without the premium directory:

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

Every Pro and Business add-on then answers `402`; the Standard scope, version
history and attachments included, runs unchanged.

## Earlier versions

Releases up to and including 2.2.1 were published under the Business Source
License 1.1 and stay under it; from 19.08.2030 they become Apache 2.0 as that
license promised. Everything after 2.2.1 is AGPL 3.0 plus the Enterprise
License as described here.

## A key cannot be revoked

Keys are checked offline against a built-in Ed25519 key. No licence server, no
phoning home, works in networks without internet access.

The price for that: an issued key stays valid. An expiry date is the only
lever, which is why keys for paying customers carry one.

Whoever reads `backend/premium/lizenz/pruefer.go` finds the line that checks the
signature and can remove it. That is true of every piece of software that runs
on other people's machines. What protects is not the technology but the
licence: the Enterprise License forbids exactly that.

## Third-party components

Nexora is not built from its own source alone.
What ships with it and under which licence is listed in full in
[THIRD-PARTY.md](THIRD-PARTY.md), together with the commands that recount the
state at any time.

Two points from it concern everybody who passes Nexora on or sells it.

**The editor is licensed under MPL-2.0.** BlockNote is file-level copyleft.
Nexora's own source is unaffected and the product may be sold; what is required
is that the licence notice accompanies the distributed form and that changes to
the MPL files themselves stay under the MPL. Nexora uses them unchanged. Every
generated bundle carries the notice in its header, set in
`frontend/vite.config.ts`: the minifier would otherwise throw away the
packages' licence headers, and MPL source would then go out without any notice.
Because this obligation breaks silently, CI checks during the build that the
header really arrives in every bundle.

**The runtime image contains GPL-2.0 programs.** `poppler-utils` supplies
`pdftotext` for the full text of PDF attachments, and busybox comes along from
the Alpine base. Nexora calls them as separate processes over a pipe and links
none of them; that is an independent invocation and not a derived work, and the
Go program keeps its own license.

Whoever passes on the **image**, however, distributes these programs along with
it and owes the corresponding source for them. This offer hereby applies:

> The source of all GPL- and LGPL-licensed components of the Nexora images can
> be obtained from Alpine Linux at
> <https://gitlab.alpinelinux.org/alpine/aports>, in the versions named by the
> labels of the respective image. Whoever would rather receive it directly from
> the licensor may contact Jonas Groll; it will be provided at the cost of the
> storage medium.

Both images carry this information as labels, so that it still travels along
when only the image is passed on:

```bash
docker inspect --format '{{json .Config.Labels}}' nexora-backend | python3 -m json.tool
```

Whoever wants to avoid the GPL components can remove `poppler-utils` from
`backend/Dockerfile`. The full text of PDF attachments is then lost; a pure Go
solution fails on font encodings, columns and embedded images in real PDF
files.

## Obtaining a licence

License keys and other arrangements: [info@jonasgroll.de](mailto:info@jonasgroll.de).
