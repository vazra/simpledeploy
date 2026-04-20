---
title: Configuration
description: Server config (config.yaml) and client context config reference, including all fields, defaults, and TLS modes.
---

SimpleDeploy uses two config files:

1. **Server config** (`/etc/simpledeploy/config.yaml`) - server-side settings
2. **Client config** (`~/.simpledeploy/config.yaml`) - remote connection contexts

## Server Config

Generate defaults with `simpledeploy init --config /etc/simpledeploy/config.yaml`.

```yaml
# Where SimpleDeploy stores its SQLite database and local backups
data_dir: /var/lib/simpledeploy

# Directory watched for app compose files (each subdirectory = one app)
apps_dir: /etc/simpledeploy/apps

# Caddy reverse proxy listen address (HTTPS when TLS is enabled)
listen_addr: ":443"

# Optional HTTP listener. When set and TLS is enabled, every request to this
# port is 308-redirected to https://. Leave empty to disable.
http_listen_addr: ":80"

# Management API + dashboard port
management_port: 8443

# Management domain (for TLS cert)
domain: manage.example.com

# Public hostname or IP used to build sslip.io auto-domains for template
# "Quick test" deploys. Optional. Editable at runtime via the dashboard
# (Save as default) or PUT /api/system/public-host.
public_host: ""

# TLS configuration
tls:
  mode: auto          # auto (Let's Encrypt) | custom | off
  email: admin@example.com  # ACME account email (required for auto)

# Secret for encrypting stored credentials and signing JWTs
master_secret: "change-me-to-a-random-string"

# Metrics collection and retention
metrics:
  tiers:
    - name: raw
      interval: 10s    # collection interval
      retention: 24h
    - name: 1m
      retention: 7d
    - name: 5m
      retention: 30d
    - name: 1h
      retention: 8760h  # 1 year

# Global rate limit defaults (per-app labels override)
ratelimit:
  requests: 200
  window: 60s
  burst: 50
  by: ip              # ip | header:X-API-Key | path

# Default registries applied to all apps (names reference stored registries)
registries:
  - ghcr-org
  - my-ecr

# Two-way git sync (optional). See docs/operations/git-sync.md for setup.
git_sync:
  enabled: false
  remote: git@github.com:owner/repo.git
  branch: main
  author_name: SimpleDeploy
  author_email: bot@simpledeploy.local
  ssh_key_path: ""         # path to private key file; leave empty for system default
  https_username: ""       # for HTTPS remotes instead of SSH
  https_token: ""
  poll_interval: 60s
  webhook_secret: ""       # set to enable POST /api/git/webhook
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `data_dir` | string | `/var/lib/simpledeploy` | Database and backup storage |
| `apps_dir` | string | `/etc/simpledeploy/apps` | Watched directory for compose files |
| `listen_addr` | string | `:443` | Reverse proxy listen address (HTTPS) |
| `http_listen_addr` | string | `""` | Optional plain-HTTP listener that 308-redirects to HTTPS. Ignored when `tls.mode: off`. |
| `management_port` | int | `8443` | Management API port |
| `domain` | string | - | Management UI domain |
| `public_host` | string | `""` | Server hostname/IP used for sslip.io auto-domains in template Quick test mode. Editable at runtime. |
| `tls.mode` | string | `auto` | TLS mode: `auto`, `custom`, `off`, `local` |
| `tls.email` | string | - | ACME email (required for auto TLS) |
| `master_secret` | string | **required** | Encryption/signing key |
| `metrics.tiers` | list | see above | Metrics rollup tiers |
| `ratelimit.requests` | int | `200` | Default requests per window |
| `ratelimit.window` | string | `60s` | Rate limit time window |
| `ratelimit.burst` | int | `50` | Burst allowance |
| `ratelimit.by` | string | `ip` | Rate limit key |
| `registries` | list | `[]` | Default registry names for all apps |
| `git_sync.enabled` | bool | `false` | Enable two-way git sync |
| `git_sync.remote` | string | - | Git remote URL (required if enabled); SSH or HTTPS |
| `git_sync.branch` | string | `main` | Branch to sync against |
| `git_sync.author_name` | string | `SimpleDeploy` | Git commit author name |
| `git_sync.author_email` | string | `bot@simpledeploy.local` | Git commit author email |
| `git_sync.ssh_key_path` | string | `""` | Path to SSH private key; empty uses the system default |
| `git_sync.https_username` | string | `""` | Username for HTTPS remotes |
| `git_sync.https_token` | string | `""` | Token/password for HTTPS remotes |
| `git_sync.poll_interval` | duration | `60s` | How often to poll the remote |
| `git_sync.webhook_secret` | string | `""` | HMAC secret; when set, enables `POST /api/git/webhook` |

See [Git sync](/operations/git-sync/) for setup and operational details.

### TLS modes

See [TLS and HTTPS](/guides/tls/) for the full mode breakdown and tradeoffs.

### Metrics Tiers

Each tier defines an aggregation level and retention period:

| Tier | Resolution | Default Retention | Query Range |
|------|-----------|-------------------|-------------|
| `raw` | 10s | 24h | last hour |
| `1m` | 1 minute | 7 days | last 24h |
| `5m` | 5 minutes | 30 days | last week |
| `1h` | 1 hour | 1 year | beyond 1 week |

The API auto-selects the appropriate tier based on the requested time range.

Retention values support Go duration format (`24h`, `168h`) and day format (`7d`, `30d`).

### Database sizing

See [Capacity and sizing](/operations/capacity-sizing/) for row counts, DB size estimates by app count, and tuning examples.

## Client Config

Stored at `~/.simpledeploy/config.yaml`. Managed via `simpledeploy context` commands.

```yaml
contexts:
  production:
    url: https://manage.myserver.com
    api_key: sd_abc123...
  staging:
    url: https://manage.staging.myserver.com
    api_key: sd_def456...
current_context: production
```

### Managing Contexts

```bash
simpledeploy context add production --url https://manage.example.com --api-key sd_...
simpledeploy context use production
simpledeploy context list
```

All remote commands (apply, remove, list, pull, diff, sync) use the current context automatically.
