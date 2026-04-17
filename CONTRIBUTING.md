# Contributing to SimpleDeploy

## Prerequisites

- Go 1.22+
- Node.js 18+
- Docker (for E2E tests)

## Setup

```bash
git clone https://github.com/vazra/simpledeploy.git
cd simpledeploy
make build
```

## Development

```bash
make dev          # hot-reload API + Svelte UI
make api          # API only with hot-reload
make ui           # UI dev server only
```

## Testing

Run tests before submitting a PR:

```bash
make test         # Go unit/integration tests (fast, no Docker needed)
make e2e          # full E2E browser tests (needs Docker running)
make e2e-headed   # E2E with visible browser for debugging
make e2e-report   # open last E2E HTML report
```

### What CI checks

| Job | Trigger | What it does |
|-----|---------|--------------|
| **lint** | push + PR | `golangci-lint` |
| **test** | push + PR | `go test ./...` |
| **build** | push + PR | full `make build` (UI + Go) |
| **e2e** | push to `main` | Playwright browser tests against a real server |

All jobs except E2E must pass before a PR can merge.

## Project Structure

See [CLAUDE.md](CLAUDE.md) for detailed architecture and code conventions.

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`

Scopes: `api`, `cli`, `ui`, or omit if broad.

## Submitting Changes

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Run `make test` (and `make e2e` if touching API/UI)
4. Commit with a conventional commit message
5. Open a PR against `main`
