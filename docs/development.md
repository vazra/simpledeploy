# Development Guide

## Prerequisites

- Go 1.22+
- Node.js 18+
- Docker running locally

## Quick Start

```bash
# Terminal 1: Go backend
make ui-build   # first time only, builds UI embed
make dev

# Terminal 2: Svelte UI (hot reload)
make dev-ui
```

- Management UI: http://localhost:5173 (Vite dev server, proxies API to backend)
- Management API: http://localhost:8443
- Caddy proxy: http://localhost:8080

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

| Target | Description |
|--------|-------------|
| `make dev` | Build + run Go backend with dev config |
| `make dev-ui` | Svelte dev server with hot reload |
| `make dev-server` | Same as `make dev` |
| `make build` | Full build (UI + Go) |
| `make build-go` | Go only (requires ui_dist/) |
| `make ui-build` | Build Svelte UI |
| `make test` | Run all Go tests |
| `make clean` | Remove build artifacts |

## Testing

```bash
go test ./...                              # all tests
go test ./internal/api/ -v                 # specific package
go test ./internal/store/ -run TestUpsert  # specific test
```

- Docker tests skip when Docker is unavailable
- Store tests use temp DB files
- API tests use httptest + real store

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
