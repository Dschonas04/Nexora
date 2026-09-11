# Licensing

Nexora is licensed entirely under the **Business Source License 1.1** — see
[LICENSE](LICENSE).

That is not an open source licence in the OSI sense, but it is not a closed one
either: the source is open, and the Additional Use Grant expressly permits
production use, commercial use included — as long as the paid add-ons are not
used without a key.

**On 19.08.2030 the restriction falls away.** From then on Apache 2.0 applies.
That is written into the licence and cannot be revoked.

## What is permitted without a key

The core may run in production, in companies too, commercially too, without
anybody having to ask or pay:

Editor, nested pages, spaces, tags, favourites, trash, full-text search over
pages, backlinks, knowledge graph, accounts and roles.

Reading, changing, building and testing is always permitted for the **entire**
work, commercial evaluation included.

## What needs a key

| Add-on | Name in the key | Without a key |
|---|---|---|
| Version history | `versionen` | snapshots keep being taken, viewing and restoring are locked |
| Attachments | `anhaenge` | no upload area |
| Sharing and public links | `freigeben` | pages stay with their owner |
| Audit trail | `pruefspur` | recording continues, reading is locked |
| Groups and space permissions | `gruppen` | per-page permissions as before |
| SSO via OIDC | `sso` | sign-in with a password |
| LDAP and Active Directory | `ldap` | sign-in with a password |
| Full text in attachments | `anhangsuche` | search over pages, not over files |
| Export as PDF and Word, space export | `export` | per-page export as Markdown |
| Comments | `kommentare` | no comments |
| Conflict detection | `konflikte` | the last write wins |
| Editing together | `echtzeit` | one person writes, the others see it on reload |

Two of them keep recording even without a licence: **version history** and the
**audit trail**. Otherwise a gap would open up after enabling them, right over
the unlicensed period — and an audit trail with a hole in it is not one.

The server rejects locked calls with `402 Payment Required`, and the interface
hides the matching controls. Hiding them is courtesy; the rejection is the
protection.

## Where the check sits

The key check lives in `backend/premium`, the gate itself in
`backend/internal/lizenz`. The gate knows nothing about signatures — it only
asks who has registered as a checker. That is why the core can also be built
without the premium directory:

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

Every add-on then answers `402`, everything else runs unchanged.

## A key cannot be revoked

Keys are checked offline against a built-in Ed25519 key. No licence server, no
phoning home, works in networks without internet access.

The price for that: an issued key stays valid. An expiry date is the only
lever, which is why keys for paying customers should carry one.

## And the source is open

Whoever reads `backend/premium/lizenz/pruefer.go` finds the line that checks the
signature and can remove it. That is true of every piece of software that runs
on other people's machines — pfSense, GitLab, Sentry and Elastic all work this
way.

What protects is not the technology but the licence: such an intervention is a
breach of the licence, not a trick. And whoever does it would not have paid
anyway.

## Third-party components

Nexora is licensed under BUSL-1.1 but is not built from its own source alone.
What ships with it and under which licence is listed in full in
[THIRD-PARTY.md](THIRD-PARTY.md), together with the commands that recount the
state at any time.

Two points from it concern everybody who passes Nexora on or sells it.

**The editor is licensed under MPL-2.0.** BlockNote is file-level copyleft.
Nexora's own source is unaffected and the product may be sold; what is required
is that the licence notice accompanies the distributed form and that changes to
the MPL files themselves stay under the MPL. Nexora uses them unchanged. Every
generated bundle carries the notice in its header, set in
`frontend/vite.config.ts` — the minifier would otherwise throw away the
packages' licence headers, and MPL source would then go out without any notice.
Because this obligation breaks silently, CI checks during the build that the
header really arrives in every bundle.

**The runtime image contains GPL-2.0 programs.** `poppler-utils` supplies
`pdftotext` for the full text of PDF attachments, and busybox comes along from
the Alpine base. Nexora calls them as separate processes over a pipe and links
none of them; that is an independent invocation and not a derived work, and the
Go program stays under BUSL-1.1.

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

For commercial licences and other arrangements, contact the licensor, Jonas
Groll.
