---
title: Architecture overview
description: Single-binary process layout with embedded Caddy, SQLite, channel-based metrics pipeline, and custom Caddy modules.
---

## One process, many goroutines

SimpleDeploy is a single Go binary. There is no agent, no sidecar, no separate proxy process. Caddy runs as an in-process library, not a child process. SQLite is the only datastore. Everything else is a goroutine inside one PID.

```
cmd/simpledeploy/      CLI entrypoints (cobra), wires every subsystem
internal/
  api/                 REST + WebSocket handlers, middleware, routes
  auth/                bcrypt, JWT, API keys, rate limit, AES-GCM crypto
  alerts/              rule evaluator + webhook dispatch (SSRF-guarded)
  backup/              Strategy + Target interfaces, cron scheduler
  client/              HTTP client used by the CLI
  compose/             Compose YAML parser + label extraction
  config/              YAML config loader
  deployer/            shells out to `docker compose` via CommandRunner
  docker/              Docker SDK wrapper + MockClient
  logbuf/              ring buffer io.Writer + WS fan-out
  metrics/             Docker stats + gopsutil collector + rollup
  proxy/               Caddy embedding, route builder, custom modules
  reconciler/          fsnotify watcher + diff loop
  store/               SQLite (WAL), embedded migrations
ui/                    Svelte SPA, served from embedded fs at /
```

## Component diagram

```mermaid
flowchart TB
  subgraph Process[simpledeploy single process]
    direction TB
    cli[cobra CLI / main]
    api[REST + WS API]
    rec[Reconciler]
    dep[Deployer]
    cad[Caddy embedded]
    met[Metrics collector]
    mw[Metrics writer]
    ag[Rollup manager]
    al[Alert evaluator]
    wb[Webhook dispatcher]
    bk[Backup scheduler]
    lb[logbuf ring]
    db[(SQLite WAL)]
  end

  user([Operator]) --> cli
  cli --> api
  cli --> rec
  api --> db
  rec --> dep --> docker[(Docker daemon)]
  rec --> cad
  met -- chan MetricPoint --> mw --> db
  ag --> db
  db --> al --> wb --> internet([Webhook endpoint])
  bk --> db
  bk --> docker
  cad --> upstreams[(Containers)]
  dep -. os.Pipe .-> lb
  lb --> api
```

## Why these choices

**Embedded Caddy.** Running Caddy as a library means there is one process to supervise, one binary to ship, and route reloads happen with `caddy.Load(JSON)` instead of HUP signals or socket reloads. Caddy is configured purely via JSON; there is no Caddyfile anywhere. See [/internal/proxy/proxy.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/proxy.go).

**Custom Caddy modules.** Three modules are registered in the proxy package's `init()`: `simpledeploy_metrics` (records request stats into a channel), `simpledeploy_ratelimit` (per-domain token bucket), and `simpledeploy_ipaccess` (CIDR/IP allowlist). They sit in front of `reverse_proxy` in every route's handler chain.

**SQLite with WAL.** Single-writer, many-reader is exactly the workload: reconciler + metrics writer write, the API reads constantly. WAL avoids reader-blocks-writer. `SetMaxOpenConns(4)` lets multiple read connections proceed in parallel. Migrations are embedded with `go:embed` and run on `Open()`. See [/internal/store/store.go](https://github.com/vazra/simpledeploy/blob/main/internal/store/store.go).

**Channel-based metrics.** The collector samples every interval and pushes `MetricPoint` values into a buffered channel. A separate writer goroutine batches them and calls `InsertMetrics` once per flush window. This decouples sampling cadence from DB write latency. The rollup manager runs on its own ticker and aggregates raw -> 1m -> 5m -> 1h -> 1d every 60 seconds.

**Interfaces for testing.** Every external dependency has an interface: `docker.Client` (with `MockClient`), `deployer.CommandRunner` (with `MockRunner`), `backup.Strategy` and `backup.Target`, `store.*` subset interfaces declared in the consumer package. Tests do not need Docker or a network.

<Aside type="note">
The single-binary boundary is deliberate. There is no "control plane vs data plane." If the process dies, routing dies. SimpleDeploy is for one box, not a cluster. For HA, run two boxes behind a TCP load balancer with shared storage.
</Aside>

## Data flow summary

- **Request path:** TCP -> Caddy -> handler chain -> upstream container.
- **Deploy path:** file write -> fsnotify -> reconciler -> deployer -> `docker compose` -> Docker.
- **Observability path:** Docker stats -> collector -> channel -> writer -> SQLite -> rollup -> alert evaluator -> webhook.
- **Backup path:** cron tick -> scheduler -> Strategy (`docker exec`) -> Target (local FS or S3).
- **Auth path:** request -> middleware -> JWT cookie or `Authorization: Bearer sd_...` -> store lookup -> RBAC -> handler.
