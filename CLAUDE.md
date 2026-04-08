# SimpleDeploy - Codebase Guide

## What is this?

Single Go binary for deploying Docker Compose apps with built-in reverse proxy (Caddy), metrics, backups, alerts, and a Svelte dashboard.

## Build

```bash
make build      # UI + Go (requires Node.js + Go)
make build-go   # Go only (skips UI, needs ui_dist/ to exist)
make test       # Go tests
make clean      # Remove build artifacts
```

## Project Structure

```
cmd/simpledeploy/     CLI entrypoint (cobra commands, wiring)
internal/
  api/                REST API + WebSocket handlers
  auth/               Password hashing, JWT, API keys, rate limiting
  alerts/             Alert evaluator, webhook dispatch
  backup/             Strategies (postgres/volume), targets (s3/local), scheduler
  client/             HTTP client for remote API, context config
  compose/            Compose file parsing, label extraction
  config/             YAML config parsing
  deployer/           Translates compose specs to Docker API calls
  docker/             Docker client wrapper + mock
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

## Database

SQLite at `{data_dir}/simpledeploy.db`. Migrations in `internal/store/migrations/`. Currently 7 migrations:

1. apps table
2. app_labels
3. users, api_keys, user_app_access
4. metrics
5. request_stats
6. webhooks, alert_rules, alert_history
7. backup_configs, backup_runs

## Testing

```bash
go test ./...                          # all tests
go test ./internal/api/ -v             # specific package
go test ./internal/store/ -run TestUpsert  # specific test
```

- Docker tests skip gracefully when Docker is unavailable
- Store tests use temp DB files
- API tests use httptest + real store
- Mock Docker client in `internal/docker/mock.go`

## Adding a New Feature

1. If it needs DB: add migration in `internal/store/migrations/NNN_name.sql`
2. Add store methods in `internal/store/`
3. Add business logic in `internal/{feature}/`
4. Add API endpoints in `internal/api/`
5. Add CLI commands in `cmd/simpledeploy/main.go`
6. Add UI page in `ui/src/routes/`

## Commit Messages

Follow Conventional Commits format:

- `feat:` for new features
- `feat(scope):` for scoped features (e.g., `feat(api):`, `feat(cli):`, `feat(ui):`)
- `fix:` for bug fixes
- `docs:` for documentation, plans, specs
- `test:` for tests
- `refactor:` for code refactoring
- `chore:` for build/deps/tooling

Examples:
- `feat(api): add user authentication endpoints`
- `feat(cli): add deployment status command`
- `fix: resolve race condition in metrics writer`
- `docs: update API documentation`
- `test: add backup scheduler tests`
