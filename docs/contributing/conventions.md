---
title: Coding conventions
description: Go and Svelte conventions used across SimpleDeploy.
---

## Go

- **Layout**: `cmd/simpledeploy/` is the only main. Reusable code lives under `internal/<package>/`. The package name matches the directory.
- **Errors**: Return errors. Wrap with `fmt.Errorf("doing X: %w", err)` for context. Never `panic` in handlers, runners, or background loops. Top-level `main` may exit on init failures.
- **Logging**: Use the project logger. No `fmt.Println` in production code paths.
- **Interfaces for tests**: Anything that talks to the outside world (Docker, command runners, HTTP) is an interface with a default impl and a mock. See `docker.Client`/`MockClient` and `deployer.CommandRunner`/`MockRunner`.
- **Imports**: gofmt-grouped (stdlib, third-party, internal), single blank line between groups.
- **Naming**: Exported things follow Go's MixedCaps. Acronyms stay all-caps (`HTTP`, `URL`, `ID`).
- **Tests**: Unit tests beside the code. Integration tests use a temp SQLite DB. API tests use `httptest`.

## Svelte (UI)

- **Routes**: One file per route under `ui/src/routes/`. Hash-based routing.
- **State**: Use Svelte stores in `ui/src/lib/stores/`. Keep them small and typed.
- **Components**: One component per file. PascalCase filenames. Keep components under ~200 lines; split when they grow.
- **Reactivity**: Prefer `$:` derived values and store subscriptions over manual lifecycle hooks.
- **A11y**: Use semantic elements. Buttons get `aria-label` when icon-only.

## Commit messages

Conventional Commits. Format: `type(scope): description`.

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`.
Scopes: `api`, `cli`, `ui`, or omit if not scoped.

Example: `feat(api): per-route rate limit override`

## Lint and format

- Go: `gofmt`, `go vet`. CI fails on diffs.
- Svelte/TS: project Prettier + ESLint config (run `cd ui && npm run lint`).
- No trailing whitespace, LF line endings.

## Docs

When you change behavior, update the relevant page under `docs/`. New labels, env vars, or API endpoints must land in their reference page in the same PR.
