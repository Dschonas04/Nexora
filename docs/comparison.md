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
| Licence | AGPL 3.0 core + paid extras | commercial SaaS | BUSL 1.1 | AGPL 3.0 | AGPL 3.0 | MIT | AGPL 3.0 |
| Stack | Go, React, PostgreSQL |, | Node, PostgreSQL, Redis | Node, PostgreSQL, Redis | Node, several databases | PHP, MySQL | Rust, Flutter |
| Sign-in without an identity provider | yes, e-mail and password |, | no, needs an OAuth provider | yes | yes | yes | yes |
| Block editor with a slash menu | yes | yes | yes | yes | markdown or WYSIWYG | WYSIWYG or markdown | yes |
| Several people in one page at the same time | no, conflicts are refused | yes | yes | yes | no | no | in the cloud version |
| Nested pages without a depth limit | yes | yes | yes | yes | folders | three levels, fixed | yes |
| Knowledge graph | yes | no | no | no | no | no | no |
| Backlinks | yes | yes | yes | yes | no | no | yes |
| Version history | yes, without a key | paid plans | yes | yes | yes | yes | yes |
| Comments | yes | yes | yes | yes | no | no | no |
| Attachments in an S3 bucket | yes |, | yes | yes | yes | yes |, |
| Import from Notion, Obsidian, Confluence | yes |, | markdown | markdown, Confluence | several formats | markdown, HTML | markdown |
| Export as PDF and Word | Pro tier | yes | PDF | PDF | PDF | PDF | markdown |
| Public share links | Pro tier | yes | yes | yes | yes | yes | yes |
| Desktop and mobile apps | on a branch, not on main | yes | web, mobile web | web | web | web | yes, it is an app first |
| Search | PostgreSQL full text; inside attachments in the Pro tier | yes | PostgreSQL full text | PostgreSQL full text | several engines | full text | local |
| Runs on one small machine | yes, two containers and a database |, | yes | yes, plus Redis | yes | yes | needs its cloud for sync |

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
- **Getting out matters as much as getting in.** Markdown import and the
  Markdown export of a page are free and always will be: a deliberate licence
  decision: the way in and the way out must never sit behind a key. The
  typeset exports (PDF, Word) and exporting a whole space as one archive are
  part of the Pro tier.
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
- **Apps in the stores.** The desktop and mobile wrappers in
  [`packaging/`](../packaging) are a window onto an instance, not an offline
  client. AppFlowy is an app first and works offline.
- **A plugin ecosystem, or AI.** There is no extension interface, no
  marketplace, no assistant writing text. Wiki.js has modules for nearly
  everything; Nexora has settings.
- **Everything for free.** The core is AGPL 3.0 and complete, but sharing,
  real-time editing, export, groups, audit trail, SSO and LDAP need a paid key.
  Wiki.js and BookStack ship those for free.
- **A large community.** Nexora is young and mostly one person's work. Docmost,
  Outline and BookStack have years of issues, packages and people who have hit
  your problem before.

## The honest summary

Nexora is for the person who wants one small service that holds nested pages,
finds things again, shows how they connect, and can be operated and backed up
without a second thought. If two people have to type in the same paragraph at
the same second, or if the wiki is really a database in disguise, take another
one, and if you need SSO, groups or an audit trail without paying for a key,
take Wiki.js or BookStack.
