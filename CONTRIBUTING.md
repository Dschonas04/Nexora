# Contributing

Thanks for your interest in contributing to Nexora!

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

### Backend (Go)

```bash
cd backend
go run .          # needs DATABASE_URL pointing at a Postgres instance
go vet ./...
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
- Backend: idiomatic Go, `go vet` clean, handlers stay thin
- Frontend: TypeScript, functional components, keep the UI minimal
- Don't commit `.env`, `node_modules/`, `dist/` or build artifacts

## Reporting Bugs

Open an issue with steps to reproduce, expected vs. actual behavior, and your
Docker/OS version if relevant.
