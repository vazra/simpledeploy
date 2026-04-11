# Compose Labels Reference

SimpleDeploy reads `simpledeploy.*` labels from your Docker Compose services to configure routing, access control, backups, alerts, and rate limiting.

## Example

```yaml
services:
  web:
    image: myapp:latest
    ports:
      - "3000:3000"
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
      simpledeploy.access.allow: "10.0.0.0/8,203.0.113.5"
    restart: unless-stopped
```

## Routing Labels

| Label | Required | Default | Description |
|-------|----------|---------|-------------|
| `simpledeploy.domain` | Yes (for proxy) | - | Domain name for reverse proxy routing |
| `simpledeploy.port` | No | First port mapping | Container port to proxy to |
| `simpledeploy.tls` | No | `auto` | TLS mode: `auto`, `custom`, `off` |

If `simpledeploy.domain` is not set, the app runs but has no proxy route (accessible only via host-mapped ports).

If `simpledeploy.port` is not set, SimpleDeploy uses the first port mapping it finds in the compose file.

## Access Control Labels

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.access.allow` | - (all traffic allowed) | Comma-separated IPs and/or CIDRs |

When set, only requests from matching IPs reach the app. Non-matching requests receive `404 Not Found`. Supports both individual IPs (`203.0.113.5`) and CIDR ranges (`10.0.0.0/8`).

When absent or empty, all traffic is allowed (no restriction).

Managed via the UI or `PUT /api/apps/{slug}/access`.

## Backup Labels

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.backup.strategy` | - | `postgres` (pg_dump) or `volume` (tar+gzip) |
| `simpledeploy.backup.schedule` | - | Cron expression (5-field, e.g., `0 2 * * *`) |
| `simpledeploy.backup.target` | - | `s3` or `local` |
| `simpledeploy.backup.retention` | `7` | Number of backups to keep |

All four labels must be set to enable backups. The backup strategy determines how data is extracted:

- **`postgres`** - runs `pg_dump -U postgres` via docker exec, gzips output
- **`volume`** - runs `tar -czf` on the volume mount path via docker exec

The S3 target config (endpoint, bucket, credentials) is set via the API or UI, not compose labels.

## Alert Labels

| Label | Format | Description |
|-------|--------|-------------|
| `simpledeploy.alerts.cpu` | `>80,5m` | CPU threshold and duration |
| `simpledeploy.alerts.memory` | `>90,5m` | Memory threshold and duration |

Format: `{operator}{threshold},{duration}` where operator is `>`, `<`, `>=`, `<=` and duration is like `5m`, `10m`.

These labels configure default alert rules auto-created when the app is deployed. Rules can be tuned or disabled via the API/UI.

## Rate Limiting Labels

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.ratelimit.requests` | global default (200) | Max requests per window |
| `simpledeploy.ratelimit.window` | global default (60s) | Time window |
| `simpledeploy.ratelimit.burst` | global default (50) | Burst allowance above limit |
| `simpledeploy.ratelimit.by` | global default (ip) | Rate limit key |

Rate limit keys:
- `ip` - rate limit per client IP
- `header:X-API-Key` - rate limit per header value
- `path` - rate limit per URL path

When rate limited, clients receive `429 Too Many Requests` with a `Retry-After` header.

If no rate limit labels are set, the global defaults from the server config apply.

## Request Tracking Labels

| Label | Description |
|-------|-------------|
| `simpledeploy.path.patterns` | Comma-separated path patterns for normalization |

Path patterns replace dynamic segments in URL paths for metrics grouping. Example: `/users/{id},/posts/{id}` normalizes `/users/123` to `/users/{id}`.

If not set, SimpleDeploy auto-normalizes by replacing all-digit path segments with `{id}`.

## Registry Labels

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.registries` | global config | Comma-separated registry names for this app |

Override which registries are used when pulling images for this app:

```yaml
labels:
  simpledeploy.registries: "ghcr-org,my-ecr"
```

Special value `none` disables all registries (including global defaults):

```yaml
labels:
  simpledeploy.registries: "none"
```

If not set, the global `registries` list from the server config applies. Registry names reference credentials stored via `simpledeploy registry add` or the API.

## Multi-Service Apps

Labels can be placed on any service in the compose file. SimpleDeploy merges labels across all services (first occurrence wins for duplicate keys).

For proxy routing, only one service per app should have `simpledeploy.domain` and `simpledeploy.port`.

For backups, place the backup labels on the service containing the data (e.g., the database service).

## Directory Structure

Each app is a subdirectory under the apps directory:

```
/etc/simpledeploy/apps/
+-- myapp/
|   +-- docker-compose.yml
+-- api-service/
|   +-- docker-compose.yml
+-- postgres/
    +-- docker-compose.yml
```

The directory name becomes the app slug used in URLs and CLI commands.
