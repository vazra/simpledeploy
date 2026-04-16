# SimpleDeploy - Codebase Guide

## What is this?

Single Go binary for deploying Docker Compose apps with built-in reverse proxy (Caddy), metrics, backups, alerts, and a Svelte dashboard.

## Build & Test

```bash
make build      # UI + Go (requires Node.js + Go)
make build-go   # Go only (skips UI, needs ui_dist/ to exist)
make test       # Go unit/integration tests
make e2e        # E2E browser tests (builds + starts server + Playwright)
make e2e-headed # E2E with visible browser window
make e2e-report # open last E2E HTML report
make clean      # Remove build artifacts
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

- **SQLite with WAL mode.** Single writer, `SetMaxOpenConns(1)`. Migrations embedded via `go:embed`.
- **Interfaces for testing.** `docker.Client` interface with `MockClient`. Strategy/Target interfaces for backups. Store interfaces in metrics/alerts to avoid import cycles.
- **Buffered channels.** Metrics and request stats flow through channels with batch writers.
- **Caddy JSON config.** No Caddyfile. Config built programmatically in `proxy.buildConfig()`, loaded via `caddy.Load()`.
- **Custom Caddy modules.** `simpledeploy_metrics` and `simpledeploy_ratelimit` registered via `init()` in proxy package.
- **Compose labels.** All app config via `simpledeploy.*` labels in docker-compose.yml.
- **CommandRunner interface.** Deployer shells out to `docker compose` CLI via `CommandRunner` with `MockRunner` for tests.
- **AES-256-GCM encryption.** Registry credentials encrypted with `master_secret` via `auth.Encrypt`/`auth.Decrypt`.
- **Log ring buffer.** Process stdout/stderr captured via `os.Pipe` into `logbuf.Buffer`, streamed to UI via WebSocket. Size configurable via `log_buffer_size` (default 500).
- **DB backup via VACUUM INTO.** WAL-safe consistent snapshots. Compact mode strips metrics/request_stats before download.

## Database

SQLite at `{data_dir}/simpledeploy.db`. Migrations in `internal/store/migrations/`. Currently 11 migrations:

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
make e2e          # build + run all E2E tests (headless)
make e2e-headed   # same but with visible browser
make e2e-report   # open HTML report from last run
```

**Requirements:** Docker daemon running, Go toolchain, Node.js. Runs ~2.5 minutes.

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
7. Add E2E tests in `e2e/tests/` (new spec file or extend existing one)
8. Run `make test && make e2e` before submitting

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
