# Search engines

Nexora is a wiki behind a login. What a search engine may find of an instance
is therefore small on purpose: the login page, which says what Nexora is.
Everything else is either behind the login or a shared page, and neither
belongs in an index.

## Switching it on

Without a public address an instance keeps every crawler out. That is the
default, because most instances run in their own network.

```env
# .env next to docker-compose.yml
NEXORA_OEFFENTLICHE_ADRESSE=https://wiki.example.org
# optional, Google Search Console, verification by meta tag (content value only)
NEXORA_GOOGLE_VERIFIZIERUNG=AbCdEf123
```

Then `docker compose up -d frontend`. At start, `frontend/tls-start.sh` writes:

| | with address | without |
|---|---|---|
| `<meta name="robots">` | `index, follow` | `noindex, nofollow` |
| canonical, `og:url`, `og:image`, JSON-LD | absolute, pointing at `/login` | removed |
| `robots.txt` | admits `/`, `/login`, assets, favicon, preview image; names the sitemap | `Disallow: /` |
| `sitemap.xml` | the login page | none |

In addition, nginx sends `X-Robots-Tag: noindex, nofollow` on every answer of
the API and on every route of the application apart from `/` and `/login`.
The backend sends the same header for shared pages (`/share/...`).

## What is in place

- Title, description, Open Graph and Twitter card, theme colour, favicon
  (`frontend/index.html`, `frontend/public/`).
- Preview image `og-image.png`, 1200×630, 87 KB.
- Structured data: `SoftwareApplication` as JSON-LD. It is a data block and no
  script, so the CSP (`script-src 'self'`) does not stand in its way.
- One browser title per view (`App.tsx`, the page name in `PageView.tsx`).
- Exactly one `h1` per view and headings without skipped levels.
- Phones: the sidebar lies over the page and closes after every navigation.
- HTTPS: the reverse proxy redirects HTTP with 301 and sends HSTS.

## Addresses (slugs)

Pages are addressed as `/page/<uuid>`. That stays: the address must survive a
renaming, and every one of these addresses is behind the login and carries
`noindex`. The only addresses a search engine sees are `/` and `/login`.

## Search Console

1. <https://search.google.com/search-console> → *Add property* → *URL prefix*
   → the public address.
2. Choose *HTML tag*, copy only the value of `content="…"`.
3. Put it into `NEXORA_GOOGLE_VERIFIZIERUNG`, `docker compose up -d frontend`.
4. *Verify* in the Search Console.
5. *Sitemaps* → `sitemap.xml` → submit.
6. Bing: <https://www.bing.com/webmasters> can take the property over from
   the Google Search Console directly.

The verification needs the Google account of whoever owns the domain; nobody
else can do this step.

## Backlinks

With one indexable page, links bring less than for a content site, but they
decide whether the name *Nexora* finds this instance at all. In the order of
effort:

1. **Own places.** The GitHub repository (website field and README), the
   personal site, profiles on GitHub, LinkedIn, Mastodon.
2. **Lists for self-hosted software.** `awesome-selfhosted` (section *Wikis*),
   `selfh.st/apps`, AlternativeTo as an alternative to Notion, Confluence and
   Outline. These are exactly the pages people read who look for something like
   Nexora.
3. **Communities.** A short post with screenshots in r/selfhosted and on the
   Hacker News *Show HN*; for German speakers, a thread in the forum of
   heise or ComputerBase. The demo instance is the argument: to be tried
   without signing up.
4. **Write about decisions.** Articles about something that was solved in
   Nexora (runtime translation without i18n keys, writing together with Yjs
   over one WebSocket, the licence question with MPL and GPL in one image)
   get linked by others because they help, not because they advertise.

Do not buy links, and no link exchanges or directories that exist only for
links: Google devalues them, and in the worst case the domain with them.
