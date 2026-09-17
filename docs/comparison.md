# Nexora next to the others

Written for the one question that matters before an installation: *is this the
right tool for me, or is one of the others?* It therefore says plainly where
Nexora is behind, because finding that out after the migration is worse for
everybody.

Feature lists change faster than documentation. Treat the table as the shape of
the field, not as a citation, and check the one project you are actually
considering.

## The field

| | Nexora | Notion | Outline | Docmost | Wiki.js | BookStack | AppFlowy |
|---|---|---|---|---|---|---|---|
| Self-hosted | yes | no | yes | yes | yes | yes | yes |
| Licence | BUSL 1.1, Apache 2.0 in 2030 | commercial SaaS | BUSL 1.1 | AGPL 3.0 | AGPL 3.0 | MIT | AGPL 3.0 |
| Stack | Go, React, PostgreSQL | — | Node, PostgreSQL, Redis | Node, PostgreSQL, Redis | Node, several databases | PHP, MySQL | Rust, Flutter |
| Sign-in without an identity provider | yes, e-mail and password | — | no, needs an OAuth provider | yes | yes | yes | yes |
| Block editor with a slash menu | yes | yes | yes | yes | markdown or WYSIWYG | WYSIWYG or markdown | yes |
| Several people in one page at the same time | no, conflicts are refused | yes | yes | yes | no | no | in the cloud version |
| Nested pages without a depth limit | yes | yes | yes | yes | folders | three levels, fixed | yes |
| Knowledge graph | yes | no | no | no | no | no | no |
| Backlinks | yes | yes | yes | yes | no | no | yes |
| Version history | yes | paid plans | yes | yes | yes | yes | yes |
| Comments | yes | yes | yes | yes | no | no | no |
| Attachments in an S3 bucket | yes | — | yes | yes | yes | yes | — |
| Import from Notion, Obsidian, Confluence | yes | — | markdown | markdown, Confluence | several formats | markdown, HTML | markdown |
| Export as PDF and Word | yes | yes | PDF | PDF | PDF | PDF | markdown |
| Public share links | yes | yes | yes | yes | yes | yes | yes |
| Desktop and mobile apps | on a branch, not on main | yes | web, mobile web | web | web | web | yes, it is an app first |
| Search | PostgreSQL full text, and inside attachments | yes | PostgreSQL full text | PostgreSQL full text | several engines | full text | local |
| Runs on one small machine | yes, two containers and a database | — | yes | yes, plus Redis | yes | yes | needs its cloud for sync |

## When Nexora is the better choice

- **The instance is supposed to be small.** A Go service, an nginx serving the
  interface, PostgreSQL. No Redis needed, no search engine next to it, no
  message queue. Two containers plus a database, and the numbers under
  [capacity](../README.md#capacity) say what that carries.
- **Nobody wants to set up an identity provider.** Outline needs an OAuth
  provider before the first login. Nexora ships accounts with a password and
  two-factor authentication; SSO and LDAP exist for whoever wants them and are
  part of the paid extras.
- **The structure grows sideways, not into folders.** Pages nest as deeply as
  you like, `[[links]]` produce backlinks on the other side without anybody
  maintaining them, and the graph shows what actually hangs together.
- **Getting out matters as much as getting in.** Markdown, PDF and Word export
  are free and always will be, and a whole space exports as one archive that
  imports again as a space. That is a deliberate licence decision: the way in
  and the way out must never sit behind a key.
- **The wiki holds more than text.** Attachments live on disk or in any
  S3-compatible bucket, with a viewer for images, PDFs and text, and the search
  can look inside the attached files.

## When it is not

- **Several people typing in one page at the same time.** Nexora has no
  real-time editing. It detects the conflict and refuses the save rather than
  overwriting a colleague, which is honest but not the same thing. Docmost and
  Outline do the real thing.
- **Notion's databases.** No table views, no boards, no calendars, no formulas.
  Nexora is a wiki, not a small application platform.
- **Apps in the stores.** The desktop and mobile wrappers live on the
  [`verpackung`](../../../tree/verpackung) branch and are a window onto an
  instance, not an offline client. AppFlowy is an app first and works offline.
- **A plugin ecosystem, or AI.** There is no extension interface, no
  marketplace, no assistant writing text. Wiki.js has modules for nearly
  everything; Nexora has settings.
- **An OSI-approved licence today.** BUSL 1.1 lets you run the core in
  production, commercially, without paying anyone, and it turns into Apache 2.0
  on 2030-08-19 — but until then it is not an open-source licence in the
  official sense, and some catalogues and company policies stop right there.
  Docmost, Wiki.js and BookStack are AGPL or MIT.
- **A large community.** Nexora is young and mostly one person's work. Docmost,
  Outline and BookStack have years of issues, packages and people who have hit
  your problem before.

## The honest summary

Nexora is for the person who wants one small service that holds nested pages,
finds things again, shows how they connect, and can be operated and backed up
without a second thought. If two people have to type in the same paragraph at
the same second, or if the wiki is really a database in disguise, take another
one — and if the licence is the sticking point, take an AGPL project instead of
waiting for 2030.
