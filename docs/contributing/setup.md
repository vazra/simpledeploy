---
title: Development setup
description: Local development environment for SimpleDeploy, covering prerequisites, dev config, sample apps, make targets, and backend conventions.
---

## Prerequisites

- Go 1.22+
- Node.js 18+
- Docker running locally

## Quick Start

**Recommended for local dev (hot reload):**

```bash
make dev
```

This runs both backend (Go, auto-reload via air) and frontend (Svelte, HMR via Vite) in one command.

- Management UI: http://localhost:5173 (Vite, proxies to backend)
- Management API: http://localhost:8080
- Caddy proxy: http://localhost:8080

**If you need to develop in a Docker container** (e.g., testing Docker networking on Docker Desktop):

```bash
make dev-docker          # Start in Docker
make dev-docker-rebuild  # Rebuild + restart on code changes
make dev-docker-down     # Stop
```

Access at https://localhost:8500/ (no hot reload in this mode).

## Dev Config

`config.dev.yaml` at repo root. TLS off, data stored in `/tmp/simpledeploy-dev`.

```yaml
data_dir: /tmp/simpledeploy-dev
apps_dir: ./dev/apps
listen_addr: ":8080"
management_port: 8443
domain: localhost
tls:
  mode: "off"
master_secret: "dev-secret-do-not-use-in-production"
```

## Sample Apps

Three sample apps in `dev/apps/` for testing different features:

| App | What it tests | Direct access | Via proxy |
|-----|--------------|---------------|-----------|
| whoami | Proxy/domain routing | http://localhost:9001 | http://whoami.localhost:8080 |
| webapp | Multi-service + postgres backup | http://localhost:9002 | http://webapp.localhost:8080 |
| redis | Volume backup strategy | localhost:6379 | N/A (TCP) |

Deploy all sample apps:

```bash
./bin/simpledeploy apply -d ./dev/apps/ --config config.dev.yaml
```

Deploy a single app:

```bash
./bin/simpledeploy apply -f ./dev/apps/whoami/docker-compose.yml --name whoami --config config.dev.yaml
```

If `*.localhost` domains don't resolve, add to `/etc/hosts`:

```
127.0.0.1 whoami.localhost webapp.localhost
```

## Make Targets

### Development

| Target | Description |
|--------|-------------|
| `make dev` | Go backend + UI (both with hot reload) |
| `make api` | Go backend only (air auto-reload) |
| `make ui` | Svelte UI dev server only (Vite HMR) |
| `make build` | Full build (UI + Go) |
| `make build-go` | Go only (requires ui_dist/) |
| `make ui-build` | Build Svelte UI |

### Docker-based Development (for Docker Desktop)

| Target | Description |
|--------|-------------|
| `make dev-docker` | Build + run simpledeploy in Docker (~docker compose up) |
| `make dev-docker-rebuild` | Rebuild binary + restart Docker container |
| `make dev-docker-down` | Stop and clean up Docker container |

Use `make dev-docker` when you need the binary in a container (e.g., testing networking features on Docker Desktop). Note: no hot reload in Docker mode; use `make dev` for faster local iteration.

### Testing

| Target | Description |
|--------|-------------|
| `make test` | Run all Go tests |
| `make e2e` | Full Playwright suite (~20 min) |
| `make e2e-lite` | Playwright suite without slow specs (~6-8 min) |
| `make clean` | Remove build artifacts |

## Testing

```bash
go test ./...                              # all tests
go test ./internal/api/ -v                 # specific package
go test ./internal/store/ -run TestUpsert  # specific test
make e2e                                   # full Playwright suite (~10-15 min)
```

- Docker tests skip when Docker is unavailable
- Store tests use temp DB files
- API tests use httptest + real store
- E2E tests build the binary, run a real server + real containers, and verify end-to-end flows. See `e2e/README.md` for a list of gotchas (container naming, Node fetch+Host header quirk, pg_dump -d, etc.) before writing new functional tests.

## Backend conventions worth knowing

- **Compose project name** is `simpledeploy-<app.Slug>`. Containers are `simpledeploy-<slug>-<service>-<replica>`. Backup strategies, detection, and any code calling `docker exec` must account for this prefix.
- **`store.App` has no JSON tags.** Responses from `/api/apps/:slug` serialize as `Name`, `Slug`, `Status`, etc. (PascalCase). Either add tags or handle both cases on the client.
- **Async deploys return 202.** `handleDeploy` spawns a goroutine with `context.Background()`. Anything that triggers reconciliation from a request handler should do the same. `r.Context()` is cancelled when the response is sent.
- **Local backup target stores only the filename** in `run.file_path`. Resolve against `<data_dir>/backups/` when reading.
- **`superAdminMiddleware`** wraps destructive system endpoints (vacuum, prune, audit-clear, audit-config write). Add it to any new endpoint that mutates system-wide state.
- **Backup strategy credentials** come from the container env at exec time via `docker exec ... sh -c '...'` reading `$POSTGRES_DB`, `$MYSQL_ROOT_PASSWORD`, `$MONGO_INITDB_ROOT_*`, etc. The scheduler does NOT populate `opts.Credentials` from container inspection. Don't rely on it. See `docs/backup-system.md` "Credential sourcing pattern".
- **Proxy handlers must strip port from `r.Host`** before looking up by domain. Caddy's `r.Host` includes the listen port (e.g. `example.com:8080` with curl `--resolve`), but the `RateLimiters` / `IPAccessRules` registries are keyed on bare domain. `net.SplitHostPort` before looking up.
- **`Deployer.Deploy` takes variadic `RegistryAuth`**. If any registry auth is passed, the deploy invokes `docker --config <tmpDir> compose up` with a generated `config.json`. `reconciler.deployApp` calls `resolveRegistries(cfg)` and threads the result through, so apps with `simpledeploy.registries` labels can pull private images on initial deploy (not just on `pull`).
- **S3 uploads use `manager.Uploader`**, not `PutObject`. The backup pipeline streams through pipes that aren't seekable, and `PutObject` needs a seekable body to compute SHA-256. Match this pattern if you add a new S3-based target.
- **Redis BGSAVE readiness**: read `LASTSAVE` *before* triggering `BGSAVE`. If you read after, a fast save (empty DB) may already have bumped the timestamp, and the poll loop will never observe a change.

## Project Layout

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
  proxy/              Caddy embedding, route management, custom modules
  reconciler/         Desired-state reconciler, directory watcher
  store/              SQLite (all DB access, migrations)
ui/                   Svelte SPA (Vite build)
dev/apps/             Sample compose apps for local testing
```
