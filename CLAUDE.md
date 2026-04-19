# SimpleDeploy - Codebase Guide

## What is this?

Single Go binary for deploying Docker Compose apps with built-in reverse proxy (Caddy), metrics, backups, alerts, and a Svelte dashboard.

## Build & Test

```bash
make build      # UI + Go (requires Node.js + Go)
make build-go   # Go only (skips UI, needs ui_dist/ to exist)
make test       # Go unit/integration tests
make e2e        # E2E browser tests, full suite (~20 min)
make e2e-lite   # E2E without slow specs (DB/S3/webhooks/registry) (~6-8 min)
make e2e-headed # E2E with visible browser window
make e2e-report # open last E2E HTML report
make e2e-templates # deploy every app template (on-demand, ~30-60 min)
make clean      # Remove build artifacts
make hooks-install # Install git pre-push hook (vet + build + short tests + vitest)
```

## Project Structure

```
cmd/simpledeploy/     CLI entrypoint (cobra commands, wiring)
internal/
  api/                REST API + WebSocket handlers
  auth/               Password hashing, JWT, API keys, rate limiting, credential encryption
  alerts/             Alert evaluator, webhook dispatch
  backup/             Strategies (postgres/volume), targets (s3/local), scheduler
  client/             HTTP client for remote API, context config
  compose/            Compose file parsing, label extraction
  config/             YAML config parsing
  deployer/           Docker compose CLI wrapper (pull, deploy, scale, etc.)
  docker/             Docker client wrapper + mock
  logbuf/             Ring buffer io.Writer for process log capture + WS streaming
  metrics/            Collector, writer, rollup
  proxy/              Caddy embedding, route management, Caddy handler modules
  reconciler/         Desired-state reconciler, directory watcher
  store/              SQLite (all DB access, migrations)
ui/                   Svelte SPA (Vite build)
```

## Key Patterns

- **SQLite with WAL mode.** `SetMaxOpenConns(4)` for concurrent reads. Migrations embedded via `go:embed`.
- **Interfaces for testing.** `docker.Client` interface with `MockClient`. Strategy/Target interfaces for backups. Store interfaces in metrics/alerts to avoid import cycles.
- **Buffered channels.** Metrics and request stats flow through channels with batch writers.
- **Caddy JSON config.** No Caddyfile. Config built programmatically in `proxy.buildConfig()`, loaded via `caddy.Load()`.
- **Custom Caddy modules.** `simpledeploy_metrics` and `simpledeploy_ratelimit` registered via `init()` in proxy package.
- **Compose labels.** All app config via `simpledeploy.*` labels in docker-compose.yml.
- **CommandRunner interface.** Deployer shells out to `docker compose` CLI via `CommandRunner` with `MockRunner` for tests.
- **AES-256-GCM encryption.** Registry credentials encrypted with `master_secret` via `auth.Encrypt`/`auth.Decrypt`. Uses random salt per encryption with PBKDF2 key derivation (backwards compatible with legacy fixed-salt format).
- **Log ring buffer.** Process stdout/stderr captured via `os.Pipe` into `logbuf.Buffer`, streamed to UI via WebSocket. Size configurable via `log_buffer_size` (default 500).
- **DB backup via VACUUM INTO.** WAL-safe consistent snapshots. Compact mode strips metrics/request_stats before download.
- **Image mirror.** When `SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX` env var is set (e.g. `ghcr.io/vazra/simpledeploy-mirror/`), the server rewrites docker.io-bound image refs in compose files at deploy/rollback/restore time. Used by E2E (`make e2e-mirror`) to avoid Docker Hub pull rate limits, but also usable for local dev. Mirror is populated by `.github/workflows/mirror-images.yml`, which runs on changes to template JS files or e2e fixtures. See `internal/mirror/`.

## Database

SQLite at `{data_dir}/simpledeploy.db`. Migrations in `internal/store/migrations/`. Currently 16 migrations:

1. apps table
2. app_labels
3. users, api_keys, user_app_access
4. metrics
5. request_stats
6. webhooks, alert_rules, alert_history
7. backup_configs, backup_runs
8. compose_hash (change detection)
9. compose_versions, deploy_events (deploy safety)
10. registries (private registry auth)
11. db_backup_config, db_backup_runs (system DB backup)
12. user profile (display_name, email)
13. metrics v2
14. alert history rule snapshot columns
15. backups v2
16. indexes (alert_history, backup_runs)

## Testing

### Unit/Integration Tests (Go)

```bash
go test ./...                          # all tests
go test ./internal/api/ -v             # specific package
go test ./internal/store/ -run TestUpsert  # specific test
```

- Docker tests skip gracefully when Docker is unavailable
- Store tests use temp DB files
- API tests use httptest + real store
- Mock Docker client in `internal/docker/mock.go`
- Mock command runner in `internal/deployer/runner.go`

### E2E Tests (Playwright)

Full browser-based test suite that builds the binary, starts a real server with Docker, deploys actual compose apps, and exercises every UI flow.

```bash
make e2e               # full E2E suite, ~20 min
make e2e-lite          # skips slow specs (13b, 27, 28, 29b), ~6-8 min
make e2e-mirror        # full suite via GHCR image mirror (no Docker Hub rate limits)
make e2e-lite-mirror   # lite suite via GHCR image mirror
make e2e-headed        # full suite with visible browser
make e2e-report        # open HTML report from last run
make e2e-templates     # deploy every app template end-to-end (on-demand)
make mirror-images-list # print the image set the mirror workflow pushes
```

**During local development: run only the related specs, not the full suite.** Specs build state sequentially, so you usually need `01-setup.spec.js` (creates admin) plus the spec(s) you care about. Add `03-deploy.spec.js` if your target depends on deployed apps. The full suite runs remotely in GitHub Actions; locally it is too slow for iteration.

```bash
cd e2e
# Minimal chain for a system-page change:
npx playwright test 01-setup.spec.js 17-system.spec.js --reporter=list

# Change that needs deployed apps (e.g. app-details, metrics):
npx playwright test 01-setup.spec.js 03-deploy.spec.js 10-app-metrics.spec.js --reporter=list

# Filter by test name within selected specs:
npx playwright test 01-setup.spec.js 17-system.spec.js -g "deployment badge" --reporter=list
```

Rule of thumb: only fall back to `make e2e-lite` when you cannot identify the minimal dependency chain, or right before opening a PR.

**Template validation:** `00-template-images.spec.js` runs in every mode and validates every image in `appTemplates.js`/`serviceTemplates.js` resolves via `docker manifest inspect`. `templates-deploy-all.spec.js` runs only under `E2E_TEMPLATES=1` (via `make e2e-templates` or the `templates.yml` GitHub workflow, which triggers on changes to template files) and deploys every app template end-to-end.

**Requirements:** Docker daemon running, Go toolchain, Node.js. Full suite ~20 min, lite ~6-8 min.

`e2e-lite` sets `E2E_LITE=1`, which makes `playwright.config.js` apply `testIgnore` to skip the heaviest specs: DB strategy matrix (MySQL/Mongo/Redis/SQLite), S3 backup roundtrip, webhook delivery timing, and private registry deploy. Good for fast local iteration; CI should keep running the full suite.

**Structure:**
```
e2e/
  playwright.config.js     # serial, 1 worker, chromium, 2min timeout
  global-setup.js          # builds binary, starts server on random port
  global-teardown.js       # kills server, cleans temp dirs
  fixtures/                # compose files for test apps (nginx, multi, postgres)
  helpers/
    server.js              # server lifecycle (build, start, stop, config gen)
    auth.js                # login/logout helpers, test admin credentials
    api.js                 # direct API fetch for setup/teardown
  tests/
    01-setup.spec.js       # initial admin account creation
    02-login.spec.js       # login, logout, wrong password, session redirect
    03-deploy.spec.js      # deploy 3 apps via wizard, validation, redeploy
    04-dashboard.spec.js   # app cards, metrics, search, navigation
    05-app-overview.spec.js # app detail, services list, tabs
    06-app-actions.spec.js # stop, start, restart, pull, scale
    07-app-config.spec.js  # compose editor, env vars
    08-app-endpoints.spec.js # endpoints, TLS badges, advanced settings
    09-app-logs.spec.js    # log viewer, controls, deploy events
    10-app-metrics.spec.js # chart rendering, time ranges
    11-app-versions.spec.js # deploy history entries
    12-backups.spec.js     # backup wizard, trigger, global summary
    13-alerts.spec.js      # webhooks CRUD, alert rules, history
    14-users.spec.js       # user CRUD, roles, API keys
    15-registries.spec.js  # registry add/delete
    16-docker.spec.js      # docker info, images, networks, volumes, prune
    17-system.spec.js      # system info, maintenance, audit, logs tabs
    18-profile.spec.js     # profile edit, password change, theme
    19-cleanup.spec.js     # remove all apps, verify clean state
```

**How tests work:**
- Tests run serially in numbered order. State builds up: setup creates admin, deploy creates apps, later tests use those apps.
- Each test gets a fresh browser context (isolated cookies) but shares server state.
- The server starts with TLS off, high rate limits (10k req/min), and temp data/apps dirs.
- Three fixture apps are deployed: `e2e-nginx` (single service), `e2e-multi` (nginx+redis), `e2e-postgres` (for backup testing).
- Cleanup tests remove all apps at the end.

**Writing new E2E tests:**
- Add a new numbered spec file in `e2e/tests/` (maintains execution order).
- Use `loginAsAdmin(page)` from `helpers/auth.js` in `beforeEach` for authenticated tests.
- Use `getState().baseURL` for the server URL. Hash-based routing: `${baseURL}/#/path`.
- Wait for `aside` element after login (sidebar indicates dashboard loaded).
- Use `.first()` on `getByText()` when text might appear in sidebar + main content.
- Scope assertions to `page.locator('main')` to avoid matching sidebar elements.
- For modals/dialogs, scope to `page.getByRole('dialog')`.

**Common pitfalls:**
- `getByText('foo')` matches substrings case-sensitively. Use `{ exact: true }` when needed.
- Don't run individual test files in isolation (they depend on prior tests' server state).
- Docker image pulls can be slow on first run. Deploy test timeout is 180s.
- The `Secure` cookie flag is conditional on TLS mode. Tests run with TLS off.
- Rate limiter config in `e2e/helpers/server.js` must stay permissive (10k req/min) to avoid login failures across 91 tests.

## Adding a New Feature

1. If it needs DB: add migration in `internal/store/migrations/NNN_name.sql`
2. Add store methods in `internal/store/`
3. Add business logic in `internal/{feature}/`
4. Add API endpoints in `internal/api/`
5. Add CLI commands in `cmd/simpledeploy/main.go`
6. Add UI page in `ui/src/routes/`
7. Add or update vitests in `ui/src/**/__tests__/` (see rule below)
8. Add E2E tests in `e2e/tests/` (new spec file or extend existing one)
9. Run `make test && make e2e` before submitting

## UI Test Coverage Rule

Any UI change that adds new behavior, fixes a bug, or modifies an existing component's logic MUST include or update a vitest under `ui/src/**/__tests__/` when it makes sense to improve product quality. Specifically:

- **New component or lib module** → add a `__tests__/<name>.test.js` next to it.
- **Bug fix** → add a regression test that fails on the old code and passes on the fix.
- **Prop/branch added to an existing component** → extend the existing test file to cover the new branch.
- **Pure helpers (format, validation, state derivation)** → always unit-test; they are cheap and catch regressions instantly.
- **Skip tests only for**: pure visual tweaks (class names, spacing), docblock-only edits, or changes fully covered by existing E2E specs where a vitest would duplicate without added signal.

Run `cd ui && npm test` (also part of the pre-push hook) to verify. Prefer vitests over new E2E specs for logic that can be exercised with mocks since vitests run in ~2s vs ~20min for e2e.

## UI Design Philosophy

SimpleDeploy targets non-technical users who want to deploy and maintain Docker Compose apps without complexity. Every UI decision must reflect this.

**Core rule: simple first, advanced easily available.**

- The primary (happy) path should be obvious and require no learning curve.
- Essential actions go front-and-center. Anything beyond the basics is tucked away but always reachable easily.
- When exposing technical details, explain it inline.
- Reveal complexity progressively - never all at once.
- Provide sensible defaults so most users never need to adjust settings.
- Error and status messages should be clear, human-readable, and actionable.

When building or modifying UI: ask "would someone who has never heard of Docker Compose understand this?" , If not, simplify it or add inline guidance.

## Commit Messages

Use Conventional Commits: `type(scope): description`

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`

Scopes: `api`, `cli`, `ui`, or omit if not scoped.

## Documentation

- User-facing docs source: `docs/` (markdown, with Starlight frontmatter)
- Site source: `docs-site/` (Astro Starlight)
- Site builds via `.github/workflows/docs.yml` and ships to GitHub Pages
- Add new doc pages under `docs/<section>/`. The site's sidebar is in `docs-site/astro.config.mjs`.
