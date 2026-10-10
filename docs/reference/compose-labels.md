---
title: Compose labels
description: Reference for simpledeploy.* labels on Docker Compose services covering routing, access, backups, alerts, rate limiting, and registries.
---

SimpleDeploy reads `simpledeploy.*` labels from your Docker Compose services to configure routing, access control, backups, alerts, and rate limiting.

## Example

```yaml
services:
  web:
    image: myapp:latest
    ports:
      - "3000:3000"
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
      simpledeploy.endpoints.0.tls: "auto"
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

Routing is configured per endpoint with indexed labels `simpledeploy.endpoints.N.*` (`N` = 0, 1, 2, ... per service). See [Endpoints and routing](/simpledeploy/concepts/endpoints-and-routing/).

| Label | Required | Default | Description |
|-------|----------|---------|-------------|
| `simpledeploy.endpoints.N.domain` | Yes (for proxy) | - | Domain name for reverse proxy routing |
| `simpledeploy.endpoints.N.port` | No | First port mapping | Container port to proxy to |
| `simpledeploy.endpoints.N.tls` | No | `auto` | TLS mode: `auto`, `local`, `custom`, `off` |
| `simpledeploy.endpoints.N.protocol` | No | `http` | Upstream protocol: `http`, `h2c` (HTTP/2 cleartext), `grpc` (h2c, only for native gRPC requests) |
| `simpledeploy.endpoints.N.path` | No | - (all paths) | Caddy path matcher, e.g. `/ws*` (prefix) or `/health` (exact). Must start with `/`; letters, digits, `. _ ~ % / * -`; max 256 chars |

If no endpoint has a domain, the app runs but has no proxy route (accessible only via host-mapped ports).

If `port` is not set, SimpleDeploy uses the first port mapping it finds in the compose file.

### Several endpoints on one domain (gRPC, websockets, REST)

Endpoints may share a domain when their matchers differ. Example: one service exposing native gRPC, a websocket path and a default HTTP port on the same host:

```yaml
services:
  api:
    image: example/api:1.0
    labels:
      simpledeploy.endpoints.0.domain: "api.example.com"
      simpledeploy.endpoints.0.port: "50051"
      simpledeploy.endpoints.0.protocol: "grpc"
      simpledeploy.endpoints.1.domain: "api.example.com"
      simpledeploy.endpoints.1.port: "8000"
      simpledeploy.endpoints.1.path: "/ws*"
      simpledeploy.endpoints.2.domain: "api.example.com"
      simpledeploy.endpoints.2.port: "8001"
```

Per domain, routes are tried in this order: `grpc` endpoints, then endpoints with a `path` (longest first), then the catch-all.

- `protocol: grpc` matches requests with `Content-Type: application/grpc*` and proxies over h2c. gRPC-Web (`application/grpc-web*`) is excluded so browsers keep hitting the HTTP/1.1 port.
- `protocol: h2c` proxies everything matched by the endpoint over HTTP/2 cleartext (for upstreams that only speak h2c).
- `h2c`/`grpc` responses are flushed immediately, so server-streaming and bidi RPCs work. Websockets and chunked/streaming HTTP responses (gRPC-Web, SSE) stream on `http` endpoints without extra config.
- Two endpoints on one domain with the same path and grpc-ness are rejected at deploy (`http` and `h2c` catch-alls count as the same matcher).
- Endpoints sharing a domain must use the same `tls` mode (empty, `auto` and `letsencrypt` are equivalent); a mismatch is rejected at deploy.
- Apps already on disk (written by an older version, gitsync or a restore) are never dropped for these problems: the reconciler logs a warning, keeps the first endpoint per matcher (ordered by domain, label index `N`, then service name) and uses the first endpoint's `tls` mode for the domain.
- With `tls.mode: off` (TLS terminated in front of SimpleDeploy) the proxy listener also accepts h2c, so gRPC clients can connect in plaintext.
- gRPC clients that default to a non-443 port (e.g. `:50051`) can reach the same routes when that port is listed in `extra_listen_addrs` in `config.yaml` (see [Configuration](/simpledeploy/reference/configuration/)).

Endpoint services do not need to publish host ports to be reachable. SimpleDeploy auto-attaches them to a shared `simpledeploy-public` Docker network and reverse-proxies over that. `ports:` still works and, when present, takes precedence over the shared-network path.

### Published port loopback rewrite

`ports: "8080:80"` style mappings are rewritten at deploy time to
`127.0.0.1:8080:80` so the published port is reachable only from the host
itself. Caddy still proxies external traffic to the same upstream, but
this prevents an unauthenticated attacker from reaching the app raw on
:8080 and bypassing per-app `simpledeploy.access.allow` and
`simpledeploy.ratelimit.*` controls.

Operator-explicit interface bindings (`"0.0.0.0:8080:80"`,
`"127.0.0.1:9090:90"`, `"[::1]:5432:5432"`) are preserved verbatim. To
disable the rewrite globally, set `SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK=true`.

### Compose security validation

SimpleDeploy checks every compose file before it starts containers: on
deploy, import, rollback, version restore, pull, scale, and when the
reconciler finds a file on disk (git sync, SSH). The check sees the file
exactly as `docker compose` will, with `${VARS}` filled in from the app's
`.env` and the server environment, so the values that are checked are
the values that run. A refused deploy shows the reasons in the deploy log.

These are refused:

- `privileged: true`, and `privileged: true` on `post_start`/`pre_stop`
  hooks.
- `use_api_socket` (it hands the container the Docker API).
- `provider` services (they run a host-side plugin instead of a container).
- Sharing host or other containers' namespaces: `network_mode: host`,
  `network_mode: container:*`, `pid: host` or any other `pid` value,
  `ipc: host`, `ipc: container:*`, `uts: host`, `userns_mode: host`,
  `cgroup: host`, and top-level `networks` whose `name` is `host` or
  `container:*` or that use the `host` driver.
- Dangerous capabilities via `cap_add` (any letter case, with or without
  `CAP_`): `ALL`, `SYS_ADMIN`, `SYS_PTRACE`, `SYS_MODULE`, `SYS_RAWIO`,
  `SYS_BOOT`, `SYS_TIME`, `NET_ADMIN`, `NET_RAW`, `DAC_READ_SEARCH`,
  `DAC_OVERRIDE`, `BPF`, `PERFMON`, `MKNOD`, and similar.
- `security_opt` that turns protections off (`=` or `:` form):
  `apparmor=unconfined`, `seccomp=unconfined` or a custom seccomp profile,
  `label=disable`, `label=type:*`, `systempaths=unconfined`,
  `no-new-privileges=false`.
- `devices`, `device_cgroup_rules`, `volumes_from`.
- Build settings that reach the host: `network: host`, privileged builds,
  insecure entitlements.
- `include` (not supported), and `extends` files outside the app folder.

`pre_start` hooks and top-level `jobs` start their own containers, so each
one gets the same checks as a service, including the host folder rules
below. Their `env_file`, `label_file` and `extends` files must be inside
the app folder too.

GPU access through `gpus:` or `deploy.resources.reservations.devices` is
allowed: Docker grants it through device requests, not raw host device
nodes.

**Host folders (bind mounts).** Folders inside the app's own folder (for
example `./data`) are fine, subject to the layout rules below. Paths are cleaned and symlinks are
followed before checking. A bind is refused when its source is, contains,
or sits inside any of:

- System folders: `/`, `/bin`, `/boot`, `/dev`, `/etc`, `/home`, `/lib*`,
  `/proc`, `/root`, `/run`, `/sbin`, `/snap`, `/sys`, `/usr`,
  `/var/lib`, `/var/run`, `/var/spool`, `/var/backups`, `/var/mail`
  (so `/var` itself is refused too). `/var/log` is allowed read-only
  (`/var/log:/logs:ro`).
- SimpleDeploy's data folder, the folder holding all apps, and any other
  app's folder.

Other host folders such as `/opt/myapp`, `/srv/data` or `/mnt/disk` are
allowed. To allow a specific folder under a protected one (for example
`/home/media`), add it to `allowed_bind_paths` in `config.yaml` (binds
inside it are then allowed; SimpleDeploy's data folder and other apps'
folders stay protected). Do not list parents of sensitive folders such as
`/var` or `/etc`.

**Bind layout.** Two layouts are refused even inside the app folder:

- A writable bind of the app folder itself (for example `.:/app`). Mount a
  subfolder such as `./data:/app/data` instead, or add `:ro`.
- A bind whose host folder sits inside another writable bind's host folder
  (for example `./data:/data` plus `./data/config:/etc/app`, in the same
  or another service). Use separate folders.

**Files read from the host.** `env_file`, `secrets`/`configs` with
`file:`, `label_file`, `extends` files, and `build` context,
`dockerfile`, `additional_contexts` and SSH keys must stay inside the app
folder.

**Volumes.** Top-level volumes using the `local` driver may only use
`driver_opts` for a bind to an allowed folder, `tmpfs`, `nfs`/`nfs4`, or
`cifs`/`smb3`. Other drivers have any host `device`/`mountpoint` checked
like a bind. A volume `name:` that points at another app's volumes
(`simpledeploy-<other-app>_*`) is refused.

**Symlinks.** `docker-compose.yml` and `.env` must be regular files, not
symlinks.

**Upgrading.** An app deployed before these rules keeps running and keeps
its domain routes while its compose file is unchanged. Redeploys, pulls
and scaling are refused with the reasons listed until the file is fixed.

## Access Control Labels

See [IP access control](/guides/access-control/) for the full guide.

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.access.allow` | - (all traffic allowed) | Comma-separated IPs and/or CIDRs |

## Backup Labels

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.backup.strategy` | auto-detected | `postgres`, `mysql`, `mongo`, `redis`, `sqlite`, or `volume`. Auto-detection picks a strategy from image keywords; set this label to force one explicitly. |
| `simpledeploy.backup.schedule` | - | Cron expression (5-field, e.g., `0 2 * * *`) |
| `simpledeploy.backup.target` | - | `s3` or `local` |
| `simpledeploy.backup.retention` | `7` | Number of backups to keep |

Backup strategies and what they produce:

- **`postgres`**: `pg_dump -U $POSTGRES_USER -d $POSTGRES_DB` via docker exec, gzipped. Reads user/db from container env.
- **`mysql`**: `mysqldump --all-databases -u root -p"$MYSQL_ROOT_PASSWORD"` via docker exec, gzipped.
- **`mongo`**: `mongodump --archive --gzip --authenticationDatabase admin -u $MONGO_INITDB_ROOT_USERNAME -p $MONGO_INITDB_ROOT_PASSWORD`. Restore uses `--drop`.
- **`redis`**: triggers `BGSAVE`, waits for `LASTSAVE` to bump, then `docker cp` the RDB file out and gzips it.
- **`sqlite`**: `sqlite3 <path> .backup` to produce a consistent snapshot. Requires the explicit file path in the backup config (`paths: ["/data/app.db"]`); auto-detect returns the mount dir but not the DB filename.
- **`volume`**: `tar -czf -` on the configured paths. Good for files and as a fallback for DBs where a strategy-specific option doesn't apply. Not safe to restore over a running DB.

The S3 target config (endpoint, bucket, credentials) is set via the API or UI, not compose labels.

## Alert Labels

| Label | Format | Description |
|-------|--------|-------------|
| `simpledeploy.alerts.cpu` | `>80,5m` | CPU threshold and duration |
| `simpledeploy.alerts.memory` | `>90,5m` | Memory threshold and duration |

Format: `{operator}{threshold},{duration}` where operator is `>`, `<`, `>=`, `<=` and duration is like `5m`, `10m`.

These labels configure default alert rules auto-created when the app is deployed. Rules can be tuned or disabled via the API/UI.

## Rate Limiting Labels

See [Rate limiting](/guides/rate-limiting/) for the full guide, including key types and caveats.

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.ratelimit.requests` | global default (200) | Max requests per window |
| `simpledeploy.ratelimit.window` | global default (60s) | Time window |
| `simpledeploy.ratelimit.burst` | global default (50) | Burst allowance above limit |
| `simpledeploy.ratelimit.by` | global default (ip) | Rate limit key |

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

For proxy routing, put `simpledeploy.endpoints.N.*` labels on the service that should receive the traffic. Different services may serve the same domain on different paths or protocols.

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
