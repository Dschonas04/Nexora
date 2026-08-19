# Contributing

Thanks for your interest in contributing to Nexora!

**One thing to know before you start:** Nexora is [open core](LICENSING.md).
Everything outside `backend/premium` is Apache 2.0 and takes contributions
normally. `backend/premium` is under the Business Source License 1.1 — a pull
request touching it can only be merged with a separate agreement, so please
open an issue first rather than writing code that cannot be taken.

## Getting Started

1. Fork and clone the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Make sure it builds (see below)
5. Open a Pull Request with a clear description

## Development Setup

The fastest way to run the whole stack:

```bash
cp .env.example .env
docker compose up -d --build
# UI at http://localhost:3000
```

Settings live in `config.conf`, which documents itself. Environment variables
override it, so `.env` is the right place for the database password and the
session secret.

To work on the free core alone:

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

Every paid extra then answers `402`, everything else behaves normally.

### Backend (Go)

```bash
cd backend
go run .          # reads config.conf; DATABASE_URL overrides it
gofmt -l .        # must print nothing
go vet ./...
go test ./...
go build ./...
```

### Frontend (React + Vite)

```bash
cd frontend
npm install
npm run dev       # proxies /api to http://localhost:8080
npm run build     # type-check + production build
```

## Guidelines

- Keep changes focused: one feature or fix per PR
- Backend: idiomatic Go, `gofmt` and `go vet` clean, handlers stay thin
- Frontend: TypeScript, functional components, keep the UI minimal
- Comments say **why**, not what. The code already says what it does
- A new paid extra needs its name in `internal/lizenz`, a gate on the route, and
  the same name in the `Extra` union in `frontend/src/lizenz.tsx`. The union is
  what turns a typo into a build error instead of a feature that never unlocks
- Access checks belong in the backend even when the interface already hides the
  button. Hiding is a courtesy; the refusal is the protection
- Never commit a working license key — not even in a test. Verification is
  offline, so such a key can never be withdrawn. Tests generate their own pair;
  see `internal/lizenz/lizenz_test.go`
- Don't commit `.env`, `node_modules/`, `dist/` or build artifacts

## Reporting Bugs

Open an issue with steps to reproduce, expected vs. actual behavior, and your
Docker/OS version if relevant.
