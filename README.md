# SimpleDeploy

[![CI](https://github.com/vazra/simpledeploy/actions/workflows/ci.yml/badge.svg)](https://github.com/vazra/simpledeploy/actions/workflows/ci.yml)
[![Release](https://github.com/vazra/simpledeploy/releases/latest/badge.svg)](https://github.com/vazra/simpledeploy/releases/latest)

Lightweight deployment manager for Docker Compose apps. Single binary with built-in reverse proxy, automatic TLS, metrics, backups, alerts, and a web dashboard.

Designed for small VPS instances. Targets ~60MB RAM for 10-20 apps.

## Install

### macOS

```bash
brew install vazra/tap/simpledeploy
```

### Ubuntu/Debian

```bash
curl -fsSL https://vazra.github.io/apt-repo/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/vazra.gpg
echo "deb [signed-by=/usr/share/keyrings/vazra.gpg arch=$(dpkg --print-architecture)] https://vazra.github.io/apt-repo stable main" | sudo tee /etc/apt/sources.list.d/vazra.list
sudo apt update && sudo apt install simpledeploy
```

### Linux (binary)

```bash
curl -L https://github.com/vazra/simpledeploy/releases/latest/download/simpledeploy_linux_amd64.tar.gz | tar xz
sudo mv simpledeploy /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/vazra/simpledeploy.git
cd simpledeploy
make build
# binary at bin/simpledeploy
```

Requires Go 1.22+ and Node.js 18+.

## Features

- **Docker Compose deployments** via config-as-code (compose files with labels)
- **Reverse proxy with automatic TLS** (embedded Caddy, Let's Encrypt)
- **System and container metrics** with tiered rollup (raw/1m/5m/1h)
- **Request tracking** per app (rate, latency, status codes)
- **Per-app rate limiting** via compose labels
- **Scheduled backups** for Postgres (pg_dump) and volumes (tar+gzip) to S3 or local
- **Alerts** with webhooks (Slack, Telegram, Discord, custom)
- **Log streaming** via WebSocket
- **Private registry support** for pulling from Docker Hub, GHCR, ECR, ACR, and self-hosted registries
- **Multi-user RBAC** with per-app access scoping
- **Remote client CLI** with context management (like kubectl)
- **Deploy safety** with compose versioning, rollback, and audit trail
- **App lifecycle controls** (restart, stop, start, pull, scale) via CLI, API, and UI
- **Web dashboard** (embedded Svelte SPA)
- **Single binary**, SQLite storage, minimal dependencies

## Quick Start

### Server Setup

```bash
# Generate config
simpledeploy init --config /etc/simpledeploy/config.yaml

# Edit config (set domain, TLS email, master secret)
vim /etc/simpledeploy/config.yaml

# Start server
simpledeploy serve --config /etc/simpledeploy/config.yaml
```

If installed via `.deb`, a systemd service is included. Enable with:

```bash
sudo systemctl enable --now simpledeploy
```

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
simpledeploy users create --username admin --password secret --role super_admin
simpledeploy apikey create --name "ci-deploy" --user-id 1
```

## Documentation

- [Deployment Guide](docs/deployment.md)
- [Configuration Reference](docs/configuration.md)
- [CLI Reference](docs/cli.md)
- [API Reference](docs/api.md)
- [Compose Labels](docs/compose-labels.md)

## Architecture

```
simpledeploy (single process)
+-- Caddy (embedded reverse proxy, TLS, rate limiting, request metrics)
+-- API server (REST + WebSocket, management UI)
+-- Reconciler (watches config dir, deploys via Docker Compose CLI)
+-- Metrics collector (Docker stats + gopsutil, every 10s)
+-- Metrics rollup (tiered aggregation + pruning)
+-- Backup scheduler (cron-based, Postgres/volume, S3/local)
+-- Alert evaluator (checks thresholds every 30s, fires webhooks)
+-- SQLite (WAL mode, all state)
```

## License

MIT
