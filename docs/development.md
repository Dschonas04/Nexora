# Development Guide

Setting up, what to run before committing, and how to add something.

## Setup

The whole stack, the quick way:

```bash
cp .env.example .env
cp config.beispiel.conf config.conf
docker compose up -d --build      # interface at http://localhost:3000
```

Working on the code, the two halves separately:

```bash
# backend — needs a PostgreSQL. Point DATABASE_URL at the compose one if you like
cd backend
go run .

# frontend — proxies /api to http://localhost:8080
cd frontend
npm install
npm run dev
```

The backend reads `config.conf` if one is present, and starts perfectly well
without it: every setting has a default that produces a working server.

To work on the free core alone:

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

Every paid extra then answers `402` and everything else behaves normally. This
is worth doing occasionally even when you are not touching licensing: the free
core has to stand on its own, and the only way to be sure of that is to build it
without the other half present.

## Before committing

```bash
cd backend
gofmt -l .          # must print nothing (except internal/dok/metriken.go, which is generated)
go vet ./...
go test ./...
go build ./...
./test/rauchtest.sh # a real PostgreSQL, the real binary, real HTTP calls

cd ../frontend
npx tsc --noEmit -p .
npm run build
```

CI runs exactly this list.

### Why the smoke test exists

`go test` touches no database, and that is precisely where the errors sit that
nothing else finds: a query that will not parse only shows up the first time it
runs. The real example that caused it was `($1 || ' days')::interval` — every
check green, and the trash never clearing itself.

So `test/rauchtest.sh` starts a throwaway PostgreSQL, runs the built binary
against it and makes real HTTP calls, in a temporary directory that disappears
at the end even when the test fails.

## Layout

```
backend/
  main.go              the route table, and the bootstrap order:
                       config → db → licence → attachment store → cache → routes
  premium/             the licence verifier and the key issuer — deletable
  internal/
    config             config.conf + environment + defaults, and the boot warnings
    db                 pool, schema, migration
    auth               JWT and bcrypt
    middleware         cookie auth
    lizenz             feature names, tiers, the gate — no cryptography
    ablage             attachment storage: disk or S3, behind one interface
    einlesen           Markdown and HTML → editor blocks
    dok                editor blocks → PDF and .docx, and .docx back
    models             the structs that cross the API boundary
    handlers           one file per subject; see architecture 5.3
frontend/src
  api/client.ts        the only module that calls fetch
  lizenz.tsx           which extras are unlocked, asked once
  design.tsx           the three colour schemes
  components/          Sidebar, PageTree, Editor, panels, dialogs
  pages/               one file per view
```

## Conventions

- **Comments say why, not what.** The code already says what it does. Nearly
  every non-obvious line in this repository carries the reason it is that way,
  and that is the convention worth keeping above all the others.
- **Identifiers are German, prose is English.** Once a thing has a name it keeps
  it, from the column through the handler to the JSON field — `postfach` in the
  network tab is the same `postfach` as in the schema. The
  [glossary](architecture.md#12-glossary) lists every term.
- **Handlers stay thin**: parse, check access, one query, write JSON.
- **Access checks belong in the backend**, even when the interface already hides
  the button. Hiding is a courtesy; the refusal is the protection.
- **One PR, one feature or fix.**
- Frontend: TypeScript, functional components, keep the interface minimal.
- Never commit `.env`, `node_modules/`, `dist/`, or `config.conf`.

## How to add …

### … a column

Append to the schema in `internal/db/db.go`:

```sql
ALTER TABLE pages ADD COLUMN IF NOT EXISTS neue_spalte text NOT NULL DEFAULT '';
```

**Never** edit the `CREATE TABLE` above it. The script runs on every start
against databases at every age; appending is what makes a fresh volume and a
three-year-old one end up identical. Constraints have no `IF NOT EXISTS` form,
so enforce allowed values in the handler and make the unrecognised case the safe
default.

### … an endpoint

1. A handler method on `Server` in the right subject file under
   `internal/handlers`.
2. A line in the route table in `main.go`. **Static segments before wildcards** —
   `/pages/trash` has to be registered before `/pages/{id}`, or chi reads
   `trash` as a page id.
3. Access: call `pagePerm` or `isAdmin`. 404 when the caller may not read, 403
   when they may read but not write.
4. The client call in `frontend/src/api/client.ts`, which is the only place
   `fetch` is called.

### … a paid extra

Four places, and the fourth is what makes a mistake loud:

1. A constant in `internal/lizenz/lizenz.go` and its entry in `Alle`.
2. The tier it belongs to, in `stufenZusatz`.
3. A route group in `main.go` wrapped in
   `r.Use(handlers.VerlangeFunktion(lizenz.Meins))`.
4. The same string in the `Extra` union in `frontend/src/lizenz.tsx` — that union
   is what turns a typo into a build error instead of a control that silently
   never unlocks.

Then decide the question every extra has to answer: **is the data still recorded
without the licence?** Version snapshots and audit entries are, so unlocking
later does not reveal a hole. Space permissions are kept but do not take effect.
Getting content out — Markdown export and import — is never gated at all.

### … a setting

- Needed **before the database is open** (port, database URL, JWT secret):
  a field in `config.Konfig`, a default in `Standard()`, a line in `Laden()`
  giving its key and environment variable, an entry in `config.beispiel.conf`,
  and a row in the [configuration reference](configuration.md).
- Changeable **at runtime**: an entry in the table in
  `handlers/einstellungen.go` with its type, title, explanation and default. It
  lands in the `einstellungen` table and overrides the file.

If it is dangerous when left wrong, add a line to `Konfig.Warnungen()`. Warn,
do not refuse — with one exception in the whole codebase, and it is documented
as such.

## Licence keys

Never commit a working key, not even in a test. Verification is offline, so a
leaked key can never be withdrawn. Tests generate their own pair; see
`internal/lizenz/lizenz_test.go`.

Issuing needs the private key, which lives in
`NEXORA_SIGNIERSCHLUESSEL` and nowhere in the repository. See
[`backend/premium/README.md`](../backend/premium/README.md).

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md). One thing to know before you start:
Nexora is under the Business Source License 1.1, not an OSI licence. By opening
a pull request you agree your contribution may be distributed under it and under
the Apache 2.0 that succeeds it on 2030-08-19.
