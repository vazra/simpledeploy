# SimpleDeploy - Design Spec

A single Go binary for deploying and managing Docker Compose apps on a single node. Built-in reverse proxy, automatic TLS, metrics, backups, alerts, rate limiting, and a Svelte dashboard. Targets ~50MB RAM for 10-20 apps.

## Decisions

- **Language:** Go, single binary
- **Reverse proxy/TLS:** Caddy embedded as library
- **Config format:** Docker Compose files with `simpledeploy.*` labels + thin global YAML config
- **Frontend:** Svelte SPA, embedded via `go:embed`
- **Metrics:** Docker API + gopsutil + cgroup reads, SQLite storage, configurable tiered rollup
- **API tracking:** Caddy middleware at reverse proxy layer
- **Alerts:** Webhook with built-in templates (Slack, Telegram, Discord, custom)
- **Backups:** Pluggable strategies (Postgres native + generic volume), S3-compatible + local targets
- **Logs:** Docker API stream, no persistence
- **Auth:** Multi-user RBAC with per-app scoping
- **Config-as-code:** Directory watch + CLI push, bidirectional sync (UI changes pullable to files)
- **Target:** ~50-64MB RAM, 10-20 apps per node

## Architecture

Monolithic goroutine-based. Single process, all components run as goroutines communicating via Go channels and shared interfaces.

```
simpledeploy (single process)
+-- Caddy (embedded, reverse proxy + TLS + API metrics middleware + rate limiting)
+-- API server (REST API, WebSocket for logs/metrics)
+-- Reconciler (watches config dir, applies desired state via Docker API)
+-- Metrics collector (polls Docker stats + gopsutil on interval)
+-- Metrics roller (periodic aggregation + pruning)
+-- Backup scheduler (cron-based, runs pg_dump / volume tar)
+-- Alert evaluator (checks thresholds, fires webhooks)
+-- SQLite (apps, users, metrics, backup history, alert rules)
```

## Project Structure

```
simpledeploy/
+-- cmd/simpledeploy/       # main entrypoint
+-- internal/
|   +-- api/                # REST API handlers + WebSocket
|   +-- auth/               # users, roles, API keys, sessions
|   +-- proxy/              # Caddy embedding, dynamic route config, metrics middleware
|   +-- compose/            # docker compose file parsing, label extraction
|   +-- reconciler/         # desired-state reconciler, dir watcher, CLI apply
|   +-- docker/             # Docker client wrapper (deploy, stats, logs)
|   +-- metrics/            # collection (Docker stats + gopsutil + cgroups), rollup, storage
|   +-- backup/             # scheduler, strategies (postgres, volume), targets (s3, local)
|   +-- alerts/             # rule evaluation, webhook dispatch, templates (slack, telegram)
|   +-- config/             # global config parsing, defaults
|   +-- store/              # SQLite repository layer (all DB access)
+-- ui/                     # Svelte SPA source
+-- migrations/             # SQLite migrations (embedded)
+-- docs/
+-- go.mod
```

All packages under `internal/` are private. Each exposes a clean interface. No cross-package internal imports; communication happens through the API server or shared interfaces.

The Svelte app builds to `ui/dist/`, embedded into the binary via `//go:embed`. A single `make build` or `go generate` + `go build` produces the final binary.

## Data Model

### Core Tables

```sql
-- Auth
users           (id, username, password_hash, role[super_admin|admin|viewer], created_at)
api_keys        (id, user_id, key_hash, name, created_at, expires_at)
user_app_access (user_id, app_id)

-- Apps
apps            (id, name, slug, compose_path, status[running|stopped|error],
                 domain, created_at, updated_at)
app_labels      (app_id, key, value)

-- Metrics
metrics         (id, app_id[nullable], container_id, cpu_pct, mem_bytes, mem_limit,
                 net_rx, net_tx, disk_read, disk_write, timestamp, tier[raw|1m|5m|1h])

request_stats   (id, app_id, timestamp, status_code, latency_ms, method,
                 path_pattern, tier[raw|1m|5m|1h])

-- Backups
backup_configs  (id, app_id, strategy[postgres|volume], target[s3|local],
                 schedule_cron, target_config_json, retention_count)
backup_runs     (id, backup_config_id, status[running|success|failed],
                 size_bytes, started_at, finished_at, error_msg, file_path)

-- Alerts
alert_rules     (id, app_id[nullable], metric, operator, threshold, duration_sec,
                 webhook_id, enabled)
alert_history   (id, rule_id, fired_at, resolved_at, value)
webhooks        (id, name, type[slack|telegram|discord|custom], url,
                 template_override, headers_json)
```

### Design Notes

- `tier` column on metrics/request_stats: raw data rolled up into coarser tiers, old raw data pruned. Single table with tier discriminator.
- `app_id` nullable on metrics: null = system-level metric (host CPU, RAM, disk).
- `target_config_json`: S3 bucket/credentials/local path stored as JSON. Avoids column sprawl per target type.
- SQLite WAL mode for concurrent reads during writes.
- Indexes on `(app_id, tier, timestamp)` for metrics and request_stats.
- Credentials in `target_config_json` encrypted using a key derived from `master_secret` in global config.

## Reverse Proxy & TLS

Caddy runs embedded, configured programmatically (no Caddyfile).

### Dynamic Route Management
- Reconciler deploys/updates/removes apps, calls into the proxy package to update Caddy's route table via its admin API (localhost-only).
- Routes built from compose labels: `simpledeploy.domain=app.example.com` maps to the container's published port.
- Automatic HTTPS via Let's Encrypt/ZeroSSL. Custom certs supported via labels.

### API Metrics Middleware
- Custom Caddy middleware intercepts every proxied request, records: app_id, status code, latency, method, path pattern.
- Writes to a buffered channel, a separate goroutine batch-inserts into SQLite every few seconds.
- Path pattern normalization (e.g., `/users/123` becomes `/users/{id}`) to avoid cardinality explosion. Configurable per app via labels.

### Rate Limiting Middleware
- In-memory token bucket per key (IP, header value, path).
- Configured per app via compose labels:
  - `simpledeploy.ratelimit.requests` - requests per window
  - `simpledeploy.ratelimit.window` - time window (e.g., `60s`)
  - `simpledeploy.ratelimit.by` - `ip` | `header:X-API-Key` | `path`
  - `simpledeploy.ratelimit.burst` - burst allowance above limit
- Returns `429 Too Many Requests` with `Retry-After` header.
- Rate limit hits counted in request_stats for dashboard visibility.
- Global defaults in config, per-app labels override.

### Management API
- Served on a separate port (e.g., `:8443`) or a configured subdomain.
- Only the management port requires simpledeploy auth. Proxied app traffic passes through without simpledeploy auth.

### TLS Modes
- `auto` (default): Caddy handles ACME automatically.
- `custom`: user provides cert/key paths via global config.
- `off`: for running behind another load balancer.

## Config-as-Code & Reconciler

### App Directory Structure
```
/etc/simpledeploy/apps/         # default, configurable
+-- myapp/
|   +-- docker-compose.yml
+-- api-service/
|   +-- docker-compose.yml
+-- postgres/
    +-- docker-compose.yml
```

Each subdirectory = one app. Directory name = app slug.

### Compose Labels
```yaml
services:
  web:
    image: myapp:latest
    labels:
      simpledeploy.domain: "myapp.example.com"
      simpledeploy.port: "3000"
      simpledeploy.tls: "auto"
      simpledeploy.backup.strategy: "postgres"
      simpledeploy.backup.schedule: "0 2 * * *"
      simpledeploy.backup.target: "s3"
      simpledeploy.backup.retention: "7"
      simpledeploy.alerts.cpu: ">80,5m"
      simpledeploy.alerts.memory: ">90,5m"
      simpledeploy.path.patterns: "/users/{id},/posts/{id}"
      simpledeploy.ratelimit.requests: "100"
      simpledeploy.ratelimit.window: "60s"
      simpledeploy.ratelimit.by: "ip"
      simpledeploy.ratelimit.burst: "20"
```

### Reconciler Loop
1. Watches apps directory via fsnotify.
2. On change: parses compose file using the compose-go library, diffs against current state in SQLite.
3. Translates compose spec to Docker API calls (create networks, pull images, create/start containers) for new/changed apps. Stops and removes containers for removed apps. Does not shell out to `docker compose` CLI.
4. Updates Caddy routes, backup configs, alert rules from labels.
5. Records desired state in SQLite.

### CLI Apply
```bash
simpledeploy apply -f ./docker-compose.yml --name myapp
```
Copies compose file into apps directory, triggering the reconciler. Same code path as directory watch.

### Bidirectional Sync
- UI config changes write back to the compose file's labels.
- `simpledeploy pull --app myapp` exports current state to files.
- `simpledeploy pull --all` exports all apps.
- UI edits flagged as "pending sync" in dashboard until pulled.

## Metrics Collection & Storage

### Collection (every 10s)
- **Per-container:** Docker API `/containers/{id}/stats?stream=false`. Extracts CPU %, memory usage/limit, network RX/TX, disk read/write. Cross-references cgroup files under `/sys/fs/cgroup/` for precision.
- **System-level:** gopsutil for host CPU %, memory, disk usage, load average, network. Stored with `app_id = NULL`.
- Buffered channel, batch-inserted into SQLite every 10s.

### Rollup (configurable defaults)
| Tier | Aggregation interval | Retention |
|------|---------------------|-----------|
| raw  | 10s                 | 24h       |
| 1m   | 1 minute            | 7 days    |
| 5m   | 5 minutes           | 30 days   |
| 1h   | 1 hour              | 1 year    |

Aggregation: avg CPU, max memory, sum network/disk. Old data pruned after retention window.

### Query API
Dashboard requests specify a time range; API auto-selects the appropriate tier. Last hour uses raw, last 24h uses 1m, last week uses 5m, beyond that uses 1h. Returns JSON arrays for chart rendering.

## Backup System

### Architecture
```
backup scheduler (cron goroutine)
+-- strategy interface
|   +-- PostgresStrategy  - pg_dump via docker exec
|   +-- VolumeStrategy    - tar + gzip of mounted volume
+-- target interface
    +-- S3Target           - S3-compatible upload (AWS, Minio, R2, etc.)
    +-- LocalTarget        - local directory with rotation
```

### Strategy Interface
```go
type BackupStrategy interface {
    Backup(ctx context.Context, app App) (io.Reader, filename string, error)
    Restore(ctx context.Context, app App, data io.Reader) error
}
```

Adding MySQL = implement this interface. No other changes needed.

### Backup Flow
1. Scheduler reads `backup_configs`, builds cron schedule per config.
2. On trigger: creates `backup_runs` entry (status=running), calls `Backup()`.
3. Streams output directly to target (no temp file). Postgres: `pg_dump` pipes through gzip, pipes to S3 multipart upload.
4. Success: updates run with size/duration. Failure: records error.
5. Retention: prunes old backups beyond `retention_count`.

### Restore Flow
- CLI: `simpledeploy restore --app myapp --backup-id 42`
- API/UI: select run, confirm, calls `Restore()`.
- Postgres: streams from target into `pg_restore`/`psql` via docker exec.
- Volumes: stops container, extracts tar, restarts.

### Credential Storage
S3 credentials in `target_config_json` encrypted using a key derived from `master_secret` in global config.

## Alerts & Webhooks

### Evaluation (every 30s)
1. Loads active rules from SQLite.
2. For each rule: queries latest N seconds of metrics based on `duration_sec`.
3. Condition met + no active alert: fires webhook, creates `alert_history` entry.
4. Condition clears for full duration: marks alert resolved, optionally fires "resolved" webhook.

### Built-in Webhook Templates
- **Slack** - `{"text": "...", "blocks": [...]}` with color-coded severity
- **Telegram** - `{"chat_id": "...", "text": "...", "parse_mode": "HTML"}`
- **Discord** - `{"content": "...", "embeds": [...]}`
- **Custom** - raw JSON with template variables: `{{.AppName}}`, `{{.Metric}}`, `{{.Value}}`, `{{.Threshold}}`, `{{.Status}}`

Templates use Go `text/template`. Users can override via `template_override`.

### Default Alert Rules (auto-created per app, user can disable/tune)
- CPU > 80% for 5min
- Memory > 90% for 5min
- Container restart detected
- Disk > 85% (system-level)

### Supported Conditions
- `>`, `<`, `>=`, `<=` on any metric field
- Container status changes (restart, crash, OOM killed)
- Backup failure

## Auth & RBAC

### Roles
- **super_admin** - full access to everything, manages users and global config
- **admin** - manages specific apps they're granted access to (deploy, configure, backup, restore, logs, metrics)
- **viewer** - read-only access to specific apps (dashboard, logs, metrics)

### Per-App Scoping
- `user_app_access` links users to apps. Super admins bypass this.
- Non-authorized apps return 404 (not 403) to avoid leaking app names.

### Sessions & API Keys
- UI login: JWT in httpOnly cookie, 24h expiry with refresh.
- API keys: long-lived bearer tokens, same permissions as creating user. For CLI/LLM tool access.
- First run: if no users exist, prints one-time setup URL to stdout for initial super_admin creation.

### Security
- Password hashing: bcrypt, cost 12.
- Login rate limiting: in-memory token bucket on login endpoint.

## Log Streaming

### Flow
- Client opens WebSocket to `/api/apps/{slug}/logs?follow=true&tail=100&service=web`.
- API attaches to container log stream via Docker API (`ContainerLogs` with `Follow: true`).
- Multiplexes stdout/stderr, streams lines as JSON: `{"ts": "...", "stream": "stdout", "line": "..."}`.
- Multiple clients can watch the same container; each gets its own Docker API stream.
- WebSocket close cancels Docker log stream via context.

### Parameters
- `follow` - live streaming (true) or historical tail (false)
- `tail` - number of historical lines (default 100)
- `since` - timestamp to start from
- `service` - specific compose service (optional, defaults to all)

No persistence. Docker's json-file log driver handles retention. Users configure Docker log rotation independently.

## Global Config

Location: `/etc/simpledeploy/config.yaml` (overridable via `--config` flag).

```yaml
data_dir: /var/lib/simpledeploy
apps_dir: /etc/simpledeploy/apps
listen_addr: ":443"
management_port: 8443
domain: manage.example.com
tls:
  mode: auto
  email: admin@example.com
master_secret: "..."
metrics:
  tiers:
    - name: raw
      interval: 10s
      retention: 24h
    - name: 1m
      retention: 7d
    - name: 5m
      retention: 30d
    - name: 1h
      retention: 8760h
ratelimit:
  requests: 200
  window: 60s
  burst: 50
  by: ip
```

## CLI

The same binary serves as both the server and the client. All non-server commands talk to a remote (or local) simpledeploy instance via REST API.

### Connection Config

Client connection is configured via `~/.simpledeploy/config.yaml`:

```yaml
contexts:
  production:
    url: https://manage.myserver.com
    api_key: sd_key_...
  staging:
    url: https://manage.staging.myserver.com
    api_key: sd_key_...
current_context: production
```

- `simpledeploy context use staging` switches the active context.
- `simpledeploy context add prod --url https://... --api-key sd_key_...` adds a new context.
- All commands accept `--context <name>` or `--url` + `--api-key` flags to override.

### Local Workflow (manage configs on macbook, deploy to remote)

```
~/projects/my-infra/              # local directory, version controlled
+-- myapp/
|   +-- docker-compose.yml
+-- api-service/
|   +-- docker-compose.yml
+-- postgres/
    +-- docker-compose.yml
```

```bash
# deploy a single app to remote instance
simpledeploy apply -f ./myapp/docker-compose.yml --name myapp

# deploy all apps in a directory
simpledeploy apply -d ./                           # applies all subdirs

# pull current state from remote to local files
simpledeploy pull --all -o ./                       # writes compose files locally

# diff local config vs remote state
simpledeploy diff --app myapp                       # shows what would change
simpledeploy diff -d ./                             # diff all apps

# sync: apply all local changes to remote
simpledeploy sync -d ./                             # apply + remove apps not in dir
```

The `apply` command uploads the compose file contents to the remote API. The server writes it to its apps directory and the reconciler picks it up. No SSH or direct file access needed.

### All Commands

```
# Server
simpledeploy serve                                 # start server
simpledeploy init                                  # generate default server config

# Context management
simpledeploy context add <name> --url --api-key    # add remote instance
simpledeploy context use <name>                    # switch active context
simpledeploy context list                          # list contexts

# App management
simpledeploy apply -f compose.yml --name myapp     # deploy single app
simpledeploy apply -d ./                           # deploy all apps in dir
simpledeploy remove --name myapp                   # tear down app
simpledeploy list                                  # list apps + status
simpledeploy diff --app myapp                      # diff local vs remote
simpledeploy sync -d ./                            # full sync dir to remote

# Logs
simpledeploy logs myapp --follow                   # stream logs

# Backups
simpledeploy backup run --app myapp                # trigger backup now
simpledeploy backup list --app myapp               # list backup history
simpledeploy restore --app myapp --id 42           # restore from backup

# Config sync
simpledeploy pull --app myapp -o ./                # export remote state to local
simpledeploy pull --all -o ./                       # export all apps

# User/key management
simpledeploy users create/list/delete              # user management
simpledeploy apikey create/list/revoke             # API key management
```

## Memory Budget

Target: ~50-64MB RSS (excluding app containers).

| Component | Estimated RAM |
|---|---|
| Go runtime + binary | ~15MB |
| Caddy embedded (proxy, TLS) | ~15-20MB |
| SQLite (WAL mode, small cache) | ~5-8MB |
| Metrics collection buffers | ~2-3MB |
| WebSocket/log streams (5 concurrent) | ~2-3MB |
| Rate limiter maps (20 apps) | ~1MB |
| Svelte SPA (static, served from embed) | ~0 |
| **Total** | **~40-55MB** |

### Key Optimizations
- SQLite page cache limited to 2000 pages (~8MB).
- Metrics batch writes via buffered channel (cap 500 entries).
- Docker stats calls sequential (not parallel): 10-20 containers at ~50ms each = under 1s per cycle.
- No in-memory metrics caching. Dashboard queries hit SQLite directly.
- Rate limiter entries expire and get GC'd periodically.
- Log streams are pass-through (Docker to WebSocket), no buffering.
- Build: `go build -ldflags="-s -w"` targets ~30-40MB binary.
