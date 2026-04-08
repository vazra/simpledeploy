# SimpleDeploy

Lightweight deployment manager for Docker Compose apps. Single binary with built-in reverse proxy, automatic TLS, metrics, backups, alerts, and a web dashboard.

Designed for small VPS instances. Targets ~60MB RAM for 10-20 apps.

## Features

- **Docker Compose deployments** via config-as-code (compose files with labels)
- **Reverse proxy with automatic TLS** (embedded Caddy, Let's Encrypt)
- **System and container metrics** with tiered rollup (raw/1m/5m/1h)
- **Request tracking** per app (rate, latency, status codes)
- **Per-app rate limiting** via compose labels
- **Scheduled backups** for Postgres (pg_dump) and volumes (tar+gzip) to S3 or local
- **Alerts** with webhooks (Slack, Telegram, Discord, custom)
- **Log streaming** via WebSocket
- **Multi-user RBAC** with per-app access scoping
- **Remote client CLI** with context management (like kubectl)
- **Web dashboard** (embedded Svelte SPA)
- **Single binary**, SQLite storage, minimal dependencies

## Quick Start

### Install

```bash
# Build from source
git clone https://github.com/vazra/simpledeploy.git
cd simpledeploy
make build

# Binary at bin/simpledeploy
```

### Server Setup

```bash
# Generate default config
simpledeploy init --config /etc/simpledeploy/config.yaml

# Edit config (set domain, TLS email, master secret)
vim /etc/simpledeploy/config.yaml

# Start server
simpledeploy serve --config /etc/simpledeploy/config.yaml
```

On first run, simpledeploy prints a setup URL. Create the initial admin account via `POST /api/setup` or the web UI.

### Deploy an App

Create a compose file with simpledeploy labels:

```yaml
services:
  web:
    image: myapp:latest
    ports:
      - "3000:3000"
    labels:
      simpledeploy.domain: "myapp.example.com"
      simpledeploy.port: "3000"
    restart: unless-stopped
```

Deploy:

```bash
# Local (on the server)
simpledeploy apply -f docker-compose.yml --name myapp

# Remote (from your laptop)
simpledeploy context add prod --url https://manage.example.com --api-key sd_...
simpledeploy apply -f docker-compose.yml --name myapp
```

### Create Users and API Keys

```bash
# Create admin user
simpledeploy users create --username admin --password secret --role super_admin

# Create API key for CLI/automation
simpledeploy apikey create --name "ci-deploy" --user-id 1
```

## Documentation

- [Configuration Reference](docs/configuration.md)
- [CLI Reference](docs/cli.md)
- [API Reference](docs/api.md)
- [Compose Labels](docs/compose-labels.md)
- [Deployment Guide](docs/deployment.md)

## Architecture

```
simpledeploy (single process)
+-- Caddy (embedded reverse proxy, TLS, rate limiting, request metrics)
+-- API server (REST + WebSocket, management UI)
+-- Reconciler (watches config dir, deploys via Docker API)
+-- Metrics collector (Docker stats + gopsutil, every 10s)
+-- Metrics rollup (tiered aggregation + pruning)
+-- Backup scheduler (cron-based, Postgres/volume, S3/local)
+-- Alert evaluator (checks thresholds every 30s, fires webhooks)
+-- SQLite (WAL mode, all state)
```

## Requirements

- Go 1.22+ (build)
- Node.js 18+ (UI build)
- Docker (runtime)
- Linux recommended for production (macOS for development)

## License

MIT
