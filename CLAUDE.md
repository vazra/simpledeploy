# SimpleDeploy - Codebase Guide

Single Go binary for deploying Docker Compose apps with built-in reverse proxy (Caddy), metrics, backups, alerts, and a Svelte dashboard.

## Build & Test

```bash
make build      # UI + Go
make test       # Go unit/integration tests
make e2e        # full E2E (~20 min)
make e2e-lite   # E2E without slow specs (~6-8 min)
```

Full target list: `docs/contributing/build-test.md`. Pre-push hook (vet/lint/build/short tests/vitest): `make hooks-install`.

## Project Structure

```
cmd/simpledeploy/     CLI entrypoint (cobra, wiring)
internal/
  api/                REST API + WebSocket handlers
  auth/               Password hashing, JWT, API keys, rate limiting, credential encryption
  alerts/             Alert evaluator, webhook dispatch
  audit/              Audit recorder + summary rendering
  backup/             Strategies, targets, scheduler
  client/             HTTP client for remote API
  compose/            Compose parsing, label extraction
  config/             YAML config parsing
  deployer/           Docker compose CLI wrapper
  docker/             Docker client wrapper + mock
  events/             In-process pub/sub bus for realtime WS
  logbuf/             Ring buffer io.Writer for process log capture
  metrics/            Collector, writer, rollup
  mirror/             Image mirror rewrite for compose files
  proxy/              Caddy embedding, route management, custom modules
  reconciler/         Desired-state reconciler, directory watcher
  store/              SQLite (DB access, migrations)
ui/                   Svelte SPA (Vite build)
```

Architecture details per package: `docs/architecture/`.

## Key Patterns

- **SQLite + WAL.** `SetMaxOpenConns(4)`. Embedded migrations. See `docs/architecture/store.md`.
- **Interfaces for testing.** `docker.Client`/`MockClient`, `CommandRunner`/`MockRunner`, store interfaces in metrics/alerts.
- **Caddy programmatic config.** No Caddyfile. Config built in `proxy.buildConfig()`. Custom modules `simpledeploy_metrics`/`simpledeploy_ratelimit`. See `docs/architecture/proxy.md`.
- **Compose labels.** All app config via `simpledeploy.*` labels.
- **AES-256-GCM encryption.** Registry creds via `auth.Encrypt`/`Decrypt` with PBKDF2 + random salt (legacy fixed-salt still readable).
- **Log ring buffer.** Process stdout/stderr -> `os.Pipe` -> `logbuf.Buffer` -> WS stream.
- **DB backup.** `VACUUM INTO` for WAL-safe snapshots; compact mode strips metrics/request_stats.
- **Image mirror.** `SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX` rewrites docker.io refs at deploy/rollback/restore. See `docs/contributing/e2e-tests.md`.
- **Community recipes.** Catalog fetched from `recipes_index_url` with 10min TTL + stale-on-error. See `docs/contributing/community-recipes.md`.
- **Audit recording.** Mutating paths emit a row to `audit_log` via `audit.Recorder` in the same tx. Pre-rendered summaries in `internal/audit/render`.
- **Realtime events.** Notify-only pub/sub at `GET /api/events`; REST is source of truth. See `docs/architecture/realtime.md`.

## Testing

Go: `go test ./...`. Mocks in `internal/docker/mock.go`, `internal/deployer/runner.go`. Docker tests skip when daemon unavailable.

E2E: `make e2e`. **Locally run only the related specs**, not the full suite (specs build state sequentially, so chain `01-setup.spec.js` + your target):

```bash
cd e2e
npx playwright test 01-setup.spec.js 17-system.spec.js --reporter=list
# add 03-deploy.spec.js if your target needs deployed apps
```

Fall back to `make e2e-lite` only when you cannot identify the minimal chain. Full E2E details, helpers, fixtures, gotchas: `e2e/README.md`.

## UI Test Coverage Rule

Any UI change that adds new behavior, fixes a bug, or modifies an existing component's logic MUST include or update a vitest under `ui/src/**/__tests__/`:

- New component or lib module -> add `__tests__/<name>.test.js` next to it.
- Bug fix -> add a regression test that fails on the old code and passes on the fix.
- Prop/branch added -> extend the existing test file.
- Pure helpers (format, validation, state derivation) -> always unit-test.
- Skip only for: pure visual tweaks, docblock-only edits, or behavior fully covered by E2E where a vitest would duplicate without added signal.

Run `cd ui && npm test`. Prefer vitests over new E2E specs (~2s vs ~20min).

## UI Design Philosophy

SimpleDeploy targets non-technical users. **Simple first, advanced easily available.**

- Primary path obvious, no learning curve.
- Essential actions front-and-center; advanced tucked away but reachable.
- Explain technical details inline.
- Reveal complexity progressively.
- Sensible defaults so most users never adjust settings.
- Errors and status messages clear, human-readable, actionable.

Ask: "would someone who has never heard of Docker Compose understand this?" If not, simplify or add inline guidance.

## Adding a Feature

1. DB change -> migration in `internal/store/migrations/NNN_name.sql` (rules: `docs/contributing/migrations.md`).
2. Store methods, business logic, API endpoints, CLI command, UI route.
3. Add/update vitests (UI Test Coverage Rule above).
4. Add E2E spec if it's a full-stack flow.
5. `make test` before submitting.

## Workflow for Features and Bugfixes

1. Work on a branch in a git worktree, not on main.
2. Open a PR and merge it; do not push commits directly to main.
3. Check whether user-facing (`docs/`) or contributor-facing ( `CONTRIBUTING.md`, `docs/contributing/`, `docs/architecture/`, `e2e/README.md`) docs need updating; update them in the same PR.
4. Add or update test coverage when it makes sense: Go tests, vitests (per UI Test Coverage Rule), or E2E specs for full-stack flows.

## Commit Messages

Conventional Commits: `type(scope): description`. Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`. Scopes: `api`, `cli`, `ui`, or omit.

## Documentation

User-facing source: `docs/` (Starlight markdown). Site: `docs-site/` (Astro Starlight), built via `.github/workflows/docs.yml`. Sidebar in `docs-site/astro.config.mjs`.
