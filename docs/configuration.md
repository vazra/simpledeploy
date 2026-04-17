# Configuration Reference

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

# Caddy reverse proxy listen address
listen_addr: ":443"

# Management API + dashboard port
management_port: 8443

# Management domain (for TLS cert)
domain: manage.example.com

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
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `data_dir` | string | `/var/lib/simpledeploy` | Database and backup storage |
| `apps_dir` | string | `/etc/simpledeploy/apps` | Watched directory for compose files |
| `listen_addr` | string | `:443` | Reverse proxy listen address |
| `management_port` | int | `8443` | Management API port |
| `domain` | string | - | Management UI domain |
| `tls.mode` | string | `auto` | TLS mode: `auto`, `custom`, `off`, `local` |
| `tls.email` | string | - | ACME email (required for auto TLS) |
| `master_secret` | string | **required** | Encryption/signing key |
| `metrics.tiers` | list | see above | Metrics rollup tiers |
| `ratelimit.requests` | int | `200` | Default requests per window |
| `ratelimit.window` | string | `60s` | Rate limit time window |
| `ratelimit.burst` | int | `50` | Burst allowance |
| `ratelimit.by` | string | `ip` | Rate limit key |
| `registries` | list | `[]` | Default registry names for all apps |

### TLS Modes

- **`auto`** - Caddy handles ACME (Let's Encrypt/ZeroSSL) automatically. Requires port 443 and valid domain.
- **`custom`** - provide cert/key paths in config (for custom certificates).
- **`off`** - no TLS. Use when running behind another load balancer (e.g., Cloudflare).
- **`local`** - Caddy acts as a local Certificate Authority, auto-generating certs for all domains. Ideal for home networks and local development with HTTPS. Devices must install the root CA certificate from `http://<server>:<management_port>/trust` to avoid browser warnings.

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

### Database Sizing

SimpleDeploy stores all metrics in a single SQLite database at `{data_dir}/simpledeploy.db`. Understanding the sizing factors helps with capacity planning.

**How rollup works:** Raw metrics are collected every 10s per container, then aggregated into coarser tiers (1m, 5m, 1h) and deleted from the source tier. This keeps the database compact while preserving long-term trends. Disk space freed by deleted rows is reclaimed automatically once per day via incremental vacuum.

**Per-app steady-state row counts** (assuming ~3 containers per app, default retention):

| Tier | Rows per app | Notes |
|------|-------------|-------|
| `raw` | ~26K | 3 containers x 6 points/min x 24h, constantly rotating |
| `1m` | ~30K | 3 containers x 1,440/day x 7 days |
| `5m` | ~26K | 3 containers x 288/day x 30 days |
| `1h` | ~26K | 3 containers x 24/day x 365 days |
| **Total** | **~108K** | |

Each row is roughly 150 bytes including indexes.

**Estimated database size by app count** (default retention, ~3 containers/app):

| Apps | Metric rows | DB size |
|------|------------|---------|
| 5 | ~540K | 80-100 MB |
| 10 | ~1.1M | 150-200 MB |
| 20 | ~2.2M | 300-400 MB |
| 50 | ~5.4M | 800 MB - 1 GB |

**Factors that increase size:**

- **More containers per app.** An app with 10 services generates 3x more rows than one with 3. This is the biggest multiplier.
- **Longer retention.** Doubling `1h` retention from 1 year to 2 years adds ~26K rows per app.
- **Request stats.** The `request_stats` table follows the same tiered rollup. High-traffic apps with many distinct endpoint patterns generate more rows.
- **Shorter raw interval.** Changing collection from 10s to 5s doubles raw tier throughput (though raw is pruned quickly).

**Factors that do NOT significantly affect size:**

- Number of proxied domains per app (metrics are per-container, not per-domain).
- Backup configurations (stored as config rows, not time-series).

**Reducing database size:**

- Lower retention on tiers you don't need. For most setups, `raw: 12h` and `1h: 90d` is sufficient.
- Run `VACUUM;` manually via `sqlite3 {data_dir}/simpledeploy.db "VACUUM;"` if the database grew large before upgrading to a version with automatic space reclamation.

Example config for a smaller footprint:

```yaml
metrics:
  tiers:
    - name: raw
      interval: 10s
      retention: 12h
    - name: 1m
      retention: 3d
    - name: 5m
      retention: 14d
    - name: 1h
      retention: 2160h  # 90 days
```

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
