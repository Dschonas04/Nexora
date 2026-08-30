# API Reference

Every endpoint, complete. Everything lives under `/api`; `/healthz` is the one
exception and sits outside on purpose.

## Conventions

**Authentication** is an httpOnly cookie `nexora_token`, set by register, login,
the OIDC callback or the LDAP sign-in. There is no header form and no API token.
Two stages run before any handler: the cookie's JWT must parse, and the session
it names must still be valid — signing out revokes it, so a copied token stops
working.

**Unauthenticated endpoints** are exactly these:

```
POST /api/auth/register · POST /api/auth/login
GET  /api/auth/sso · GET /api/auth/oidc/start · GET /api/auth/oidc/zurueck
POST /api/auth/ldap
GET  /api/public/{token} · GET /api/public/{token}/dateien/{attId}
GET  /healthz
```

**Status codes**

| Code | Means |
|---|---|
| 400 | Malformed body or missing field |
| 401 | No session, or an invalid or revoked token. Identical for all three, so nothing is learned from the difference |
| 402 | The feature is not unlocked. Body: `{error, funktion, grund}` |
| 403 | May read, may not write — or an admin-only route |
| 404 | Does not exist, **or** exists and may not be read. The two are not told apart |
| 409 | Concurrent edit. Body: `{error, konflikt: true, stand, geaendert}` |
| 501 | Issuing a licence key on an installation without a private signing key |

**Paid** endpoints are marked with the feature name that unlocks them. Which
tier contains which feature is in [architecture chapter 8.5](architecture.md#85-the-licence-gate).

---

## Authentication and session

| Method | Path | Notes |
|---|---|---|
| `POST` | `/auth/register` | `{email, name, password}`. The **first account ever created becomes admin**. Subject to `registrierung_offen` and `erlaubte_domaenen` |
| `POST` | `/auth/login` | `{kennung, password}` — `kennung` is an email address *or* a login name; the `@` decides which. `{email, password}` is still accepted. One error message for both failure cases |
| `POST` | `/auth/logout` | Revokes the session row, not just the cookie |
| `GET` | `/auth/me` | The signed-in account |
| `GET` | `/auth/sso` | Which sign-in methods this instance offers. Read before the login form is drawn |
| `GET` | `/auth/oidc/start` | 302 to the provider · paid: `sso` |
| `GET` | `/auth/oidc/zurueck` | The provider's callback · paid: `sso` |
| `POST` | `/auth/ldap` | `{kennung, password}`, bound against the directory · paid: `ldap` |
| `GET` | `/sitzungen` | Every stored session of this account, with IP, browser, ages, and `diese: true` for the current one |
| `DELETE` | `/sitzungen` | End every session except the current one |
| `DELETE` | `/sitzungen/{id}` | End one, effective on its next request |

---

## Pages

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pages` | Flat list for the sidebar tree |
| `GET` | `/pages/shared` | Pages other accounts shared with you · paid: `freigeben` |
| `GET` | `/pages/trash` | Deleted pages, each with how long it has left |
| `POST` | `/pages` | `{title?, parentId?, spaceId?}` |
| `GET` | `/pages/{id}` | The page including tags, favourite flag and the caller's permission |
| `PUT` | `/pages/{id}` | `{title?, content?, icon?, parentId?, spaceId?, basis?}`. `basis` is the `updatedAt` the editor last saw — send it and a concurrent edit answers 409 instead of being overwritten (paid: `konflikte`; without the licence the field is ignored). Only the **owner** may change `parentId` / `spaceId`: re-parenting is a structural change to the owner's workspace |
| `DELETE` | `/pages/{id}` | Move to the trash |
| `POST` | `/pages/{id}/restore` | Back out of the trash |
| `DELETE` | `/pages/{id}/purge` | Permanent. Cascades to versions, attachments, shares, links and **subpages** |
| `PUT` | `/pages/{id}/reihenfolge` | Move and order in one call — a drag in the sidebar is one gesture, and two requests could half-fail |
| `PUT` | `/pages/{id}/breite` | Page width. Free, like the page itself |
| `POST` · `DELETE` | `/pages/{id}/favorite` | |
| `POST` | `/pages/{id}/tags` | Attach a tag |
| `DELETE` | `/pages/{id}/tags/{tagId}` | Detach it |

### Versions · paid: `versionen`

Snapshots are **written** on every installation regardless of licence — otherwise
there would be a hole in the history right after unlocking. Only viewing and
restoring are gated.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pages/{id}/versions` | Newest first |
| `GET` | `/pages/{id}/versions/{versionId}` | One snapshot |
| `POST` | `/pages/{id}/versions/{versionId}/restore` | Roll the page back |

### Attachments · paid: `anhaenge`

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pages/{id}/attachments` | |
| `POST` | `/pages/{id}/attachments` | multipart, field `file`. Capped by `max_anhang_mb` (25 by default) and by nginx's `client_max_body_size` |
| `GET` | `/pages/{id}/attachments/{attId}` | The bytes. Headers are decided by the server, not by the MIME type the uploader claimed |
| `DELETE` | `/pages/{id}/attachments/{attId}` | Removes the row and the bytes |

### Word attachments

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pages/{id}/attachments/{attId}/word` | A `.docx` as editor blocks. Whoever may read the page may read the file |
| `PUT` | `/pages/{id}/attachments/{attId}/word` | Write the blocks back as `.docx`. Needs write access **and** the `anhaenge` licence. Text, headings, lists and tables survive; headers, styles, comments and images do not |

---

## Sharing · paid: `freigeben`

| Method | Path | Notes |
|---|---|---|
| `POST` | `/pages/{id}/share` | Publish read-only, returns a random token |
| `DELETE` | `/pages/{id}/share` | Revoke the public link |
| `GET` | `/pages/{id}/shares` | Who this page is shared with |
| `POST` | `/pages/{id}/shares` | `{userId, permission}` — `"read"` or `"edit"` |
| `DELETE` | `/pages/{id}/shares/{userId}` | |
| `GET` | `/public/{token}` | The page, no session needed |
| `GET` | `/public/{token}/dateien/{attId}` | Images and attachments of a shared page. Without this route a shared page with pictures would show a visitor nothing but broken images, because the ordinary attachment path demands a session |

A public link is the only anonymous access there is, and it is read-only. Opening
a **space** (`/spaces/{id}/oeffentlich`) opens it to signed-in accounts of the
instance, never to the internet.

---

## Links, backlinks and the graph

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pages/{id}/links` | Outgoing links |
| `POST` | `/pages/{id}/links` | |
| `DELETE` | `/pages/{id}/links/{targetId}` | |
| `GET` | `/pages/{id}/backlinks` | Pages pointing here, whether through `[[wiki links]]`, `@` mentions or manual links |
| `GET` | `/graph` | Nodes and edges of the whole workspace |

---

## Spaces

| Method | Path | Notes |
|---|---|---|
| `GET` | `/spaces` | |
| `POST` | `/spaces` | |
| `PUT` | `/spaces/reihenfolge` | Order in the sidebar. Registered before `/spaces/{id}`, or chi would read `reihenfolge` as a space id |
| `PUT` | `/spaces/{id}` | Rename |
| `DELETE` | `/spaces/{id}` | The pages keep existing; their `space_id` is nulled |
| `PUT` | `/spaces/{id}/oeffentlich` | `nein` \| `lesen` \| `schreiben` — for every signed-in account of the instance. An unrecognised value becomes `nein`, so a typo can open nothing |
| `GET` | `/spaces/{id}/rechte` | Space permissions · paid: `gruppen` |
| `PUT` | `/spaces/{id}/rechte` | Grant to an account or a group: `lesen` \| `schreiben` \| `verwalten` · paid: `gruppen` |
| `GET` | `/spaces/{id}/export` | ZIP of Markdown; `?format=pdf` or `?format=word` returns the whole space as one typeset document · paid: `export` |

## Groups · paid: `gruppen`

| Method | Path |
|---|---|
| `GET` · `POST` | `/gruppen` |
| `DELETE` | `/gruppen/{id}` |
| `GET` | `/gruppen/{id}/mitglieder` |
| `PUT` | `/gruppen/{id}/mitglieder` |

The extra gates **managing** them. Whether existing permissions still apply is
decided by the permission check itself: without a licence they do not take
effect, and neither are they deleted — everything comes back unchanged once the
extra is unlocked again.

---

## Search and tags

| Method | Path | Notes |
|---|---|---|
| `GET` | `/search` | Query parameters below |
| `GET` | `/tags` | |
| `POST` | `/tags` | |
| `GET` | `/tags/{id}/pages` | |
| `DELETE` | `/tags/{id}` | |
| `GET` | `/favorites` | |

### `/search` parameters

| Parameter | Values | Effect |
|---|---|---|
| `q` | text | The query. Full text over title (weight A) and prose (weight B), ranked, with snippets |
| `space` | a space id | Narrow to one space |
| `tag` | a tag id | Narrow to one tag |
| `tage` | a positive number | Only pages changed within that many days |
| `wer` | `ich` | Only pages you authored |

Results are limited to what the caller may read, by the same rule that governs
opening a single page. Attachments are searched too where `anhangsuche` is
licensed.

---

## Comments · paid: `kommentare`

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pages/{id}/kommentare` | The whole thread, oldest first |
| `POST` | `/pages/{id}/kommentare` | `{text, elternId?}`. Replies are one level deep only |
| `GET` | `/pages/{id}/erwaehnbare` | Who may be named with `@` on this page |
| `PUT` | `/kommentare/{id}` | Edit your own |
| `DELETE` | `/kommentare/{id}` | Empties the text and keeps the shell, so replies hanging off it keep their place |
| `POST` | `/kommentare/{id}/erledigt` | Mark the thread settled, or reopen it |

## Inbox

Free, like the sidebar it sits in: it only shows what happened elsewhere anyway,
and on an instance without comments and shares it simply stays empty.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/postfach` | Newest first. `?ungelesen=1` for unread only |
| `GET` | `/postfach/anzahl` | Unread count, for the sidebar badge |
| `POST` | `/postfach/gelesen` | Mark all read |
| `POST` | `/postfach/{id}/gelesen` | Mark one read |
| `DELETE` | `/postfach` | Drop the read ones |

Three kinds of entry and no more: comments on your pages, replies to your
comments, `@` mentions, and pages somebody shared with you. An inbox that carries
noise is one people stop opening.

## Audit trail · paid: `pruefspur` · admin only

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pruefspur` | Newest first. Filters: `aktion`, `akteur` (an account id), `objekt` (an object id), `limit` (default 200, max 1000) |
| `GET` | `/pruefspur/aktionen` | Which action names actually occur, for the filter dropdown |

Recording runs on every installation regardless of licence. Only reading is
paid — a trail with a hole over exactly the unlicensed period would not be one.

---

## Sign-in attempts · admin only

| Method | Path | Notes |
|---|---|---|
| `GET` | `/system/anmeldungen` | Every attempt, successful or not, newest first, plus a summary and a per-address roll-up |

Free of charge, unlike the rest of the audit trail: who is knocking at the door
belongs to running an instance, not to reporting on it. Admin only all the same,
because put together the entries are a map of who works here and when.

| Parameter | Effect |
|---|---|
| `nur` | `fehl` for failures only, `erfolg` for successes only |
| `ip` | Only attempts from this address |
| `tage` | How far back, default 30, max 365, `0` for everything |
| `limit` | Default 200, max 1000 |

The answer carries three parts:

- `versuche` — one row per attempt: `zeitpunkt`, `erfolg`, `kennung` (what was
  typed; `typed → account` where the two differ, which is how signing in by user
  name is told apart from signing in by address), `ip`, `weg`
  (`passwort` | `ldap` | `sso`), `grund` on failures, and the user agent.
- `zusammenfassung` — successes and failures over 24 hours and 7 days, plus how
  many distinct addresses were seen.
- `herkunft` — the last week grouped by address, most failures first, with how
  many distinct accounts each address tried. Many failures from one address
  against many accounts is the pattern this table exists for.

`grund` is more precise than the response the caller got: the sign-in endpoint
answers `invalid credentials` whether the account is unknown or the password was
wrong, so it cannot be used to enumerate accounts, while the trail distinguishes
`Kennung unbekannt` from `Passwort falsch`. A hundred attempts against one real
account and a hundred against a hundred invented ones are different events.

Attempts are stored as ordinary audit trail rows, so nothing here is a second
record that could disagree with `/pruefspur`. Passwords are never recorded, on
success or failure.

---

## Import and export

| Method | Path | Notes |
|---|---|---|
| `GET` | `/pages/{id}/markdown` | The page as Markdown. **Free forever** — getting your own content out must never depend on a licence |
| `GET` | `/pages/{id}/pdf` | Typeset PDF · paid: `export` |
| `GET` | `/pages/{id}/word` | `.docx` · paid: `export` |
| `POST` | `/import` | Free, for the same reason the export is |

### `POST /import`

multipart, with the files under `file`. Accepts single `.md` / `.html` files or a
whole `.zip` — an Obsidian vault, a Notion export (the id in every filename is
stripped), a Confluence HTML export, a git wiki, a folder of notes.

| Field | Effect |
|---|---|
| `parentId` | Hang the result under this page |
| `spaceId` | Put it in this space |
| `neueAblage` | Create a space of this name for it instead (max 120 characters). What makes an exported space importable again as a whole |
| `vorschau` | Anything non-empty: report the page tree that *would* be created and change nothing |

The archive keeps its shape: a folder becomes a page and the files inside become
its subpages; an `index.md`, `README.md` or `INHALT.md` becomes the folder's own
content. Links between imported files are rewritten to `[[Page title]]` so they
still lead somewhere and feed backlinks and the graph. Images and other files
become attachments of the page that uses them, and anything nothing referenced is
attached to its folder's page rather than dropped. Front matter supplies title,
tags and icon.

---

## Administration

Admin-only routes are enforced inside the handler, not by a separate gate.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/users` | admin |
| `POST` | `/users` | admin |
| `DELETE` | `/users/{id}` | admin, and not yourself |
| `PUT` | `/users/{id}/role` | admin |
| `PUT` | `/users/{id}/benutzername` | The account itself, or an admin |
| `GET` | `/lizenz` | What is unlocked, plus the tier table so the interface can show what a higher tier would bring. Readable by everyone; it contains no secret, and hiding it would only make the interface lie |
| `PUT` | `/system/lizenz` | Import a key, effective at once · admin |
| `POST` | `/system/lizenz/ausstellen` | Issue a key. **501** where no private signing key is present, which is every ordinary installation |

### Settings and system state

| Method | Path | Notes |
|---|---|---|
| `GET` | `/einstellungen` | The runtime settings with their type, title, explanation and default · admin |
| `PUT` | `/einstellungen` | Set one · admin |
| `DELETE` | `/einstellungen` | Reset one to its default · admin |
| `GET` | `/design` | The colour scheme. Readable by **everyone**, or an ordinary user would never see the configured colours, since the settings page itself is closed to them |
| `GET` | `/system` | The state of the stack: which surrounding services answer, how fast, which version they run, how much they hold. Nexora has no Docker socket on purpose, so this is what can be established over the network |
| `GET` | `/system/ablage` | Where attachments currently live |
| `POST` | `/system/ablage/test` | Try an object store's credentials before saving them |
| `POST` | `/system/suchindex` | Rebuild the full text index — needed after changing `such_woerterbuch` |
| `POST` | `/system/anhangindex` | Extract attachment text for files uploaded before the attachment index existed |
| `GET` | `/system/anmeldungen` | Sign-in attempts, see above · admin |

### Maintenance

| Method | Path | Notes |
|---|---|---|
| `GET` | `/system/konfig` | `config.conf` as read, credentials masked · admin |
| `PUT` | `/system/konfig` | Check or write it. Keeps a timestamped backup; masked credentials are restored, never saved as asterisks · admin |
| `POST` | `/system/neustart` | Ends the process so its supervisor restarts it. The only way a config change takes effect, since the file is read at startup · admin |
| `POST` | `/system/papierkorb` | Purge the whole instance's trash · admin |

---

## Health

| Method | Path | Notes |
|---|---|---|
| `GET` | `/healthz` | Outside `/api`, needs no session. Pings the database — a backend without one cannot serve anything useful, so an answer that ignored it would be a lie |
