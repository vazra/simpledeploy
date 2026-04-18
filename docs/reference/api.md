---
title: REST API
description: Management API endpoints served on the configured management_port, covering apps, metrics, backups, alerts, users, and registries.
---

Management API served on the configured `management_port` (default: 8443).

## Authentication

All endpoints except `/api/health`, `/api/auth/login`, `/api/auth/logout`, and `/api/setup` require authentication.

**Session (UI):** POST to `/api/auth/login`, receive JWT in httpOnly cookie.

**API Key (CLI/automation):** Include `Authorization: Bearer sd_...` header.

## Public Endpoints

### `GET /api/health`

Health check.

```json
{"status": "ok"}
```

### `POST /api/auth/login`

Authenticate and receive session cookie.

```json
// Request
{"username": "admin", "password": "secret"}

// Response 200
{"username": "admin", "role": "super_admin"}
```

Rate limited by client IP.

### `POST /api/auth/logout`

Clear session cookie. Returns `{"status": "ok"}`.

### `POST /api/setup`

Create initial admin account. Only works when no users exist.

```json
// Request
{"username": "admin", "password": "secret"}

// Response 201
{"username": "admin", "role": "super_admin"}

// Response 409 (users already exist)
```

## App Endpoints

### `GET /api/apps`

List all apps (filtered by user access for non-super_admins).

```json
[
  {"ID": 1, "Name": "myapp", "Slug": "myapp", "Status": "running", "Domain": "myapp.example.com"},
  {"ID": 2, "Name": "postgres", "Slug": "postgres", "Status": "running", "Domain": ""}
]
```

### `GET /api/apps/{slug}`

Get app details. Returns 404 for unauthorized apps (not 403).

### `POST /api/apps/deploy`

Deploy an app by uploading a compose file.

```json
// Request
{"name": "myapp", "compose": "<base64-encoded compose file>"}

// Response 201
{"name": "myapp", "status": "deployed"}
```

### `DELETE /api/apps/{slug}`

Remove an app. Stops containers, removes network, deletes from store.

### `GET /api/apps/{slug}/compose`

Get the raw compose file content. Returns `text/yaml`.

## App Actions

### `POST /api/apps/{slug}/restart`

Restart all containers for the app. Returns `{"status": "ok"}`.

### `POST /api/apps/{slug}/stop`

Stop all containers. Returns `{"status": "ok"}`.

### `POST /api/apps/{slug}/start`

Start stopped containers. Returns `{"status": "ok"}`.

### `POST /api/apps/{slug}/pull`

Pull latest images and redeploy. Uses configured registry auth if available. Returns `{"status": "ok"}`.

### `POST /api/apps/{slug}/scale`

Scale services.

```json
// Request
{"scales": {"web": 3, "worker": 2}}

// Response 200
{"status": "ok"}
```

### `GET /api/apps/{slug}/services`

List service status.

```json
[{"service": "web", "state": "running", "health": "healthy"}]
```

## Deploy History

### `GET /api/apps/{slug}/versions`

List compose file versions for an app.

### `POST /api/apps/{slug}/rollback`

Rollback to a previous compose version.

```json
// Request
{"version_id": 3}

// Response 200
{"status": "ok"}
```

### `GET /api/apps/{slug}/events`

List deploy events (deploys, rollbacks).

## Compose Validation

### `POST /api/apps/validate-compose`

Validate a compose file without deploying.

```json
// Request
{"compose": "<base64-encoded compose>"}

// Response 200
{"valid": true, "services": [...]}
```

## Metrics Endpoints

### `GET /api/metrics/system`

System-level metrics (CPU, memory).

| Param | Default | Description |
|-------|---------|-------------|
| `from` | 1h ago | Start time (RFC3339 or Unix timestamp) |
| `to` | now | End time |

Auto-selects tier based on time range.

```json
[
  {"timestamp": "2026-04-08T12:00:00Z", "cpu_pct": 12.5, "mem_bytes": 1048576, "mem_limit": 4194304, "net_rx": 0, "net_tx": 0, "disk_read": 0, "disk_write": 0}
]
```

### `GET /api/apps/{slug}/metrics`

Per-app container metrics. Same params and format as system metrics.

### `GET /api/apps/{slug}/requests`

Per-app request statistics.

| Param | Default | Description |
|-------|---------|-------------|
| `from` | 1h ago | Start time |
| `to` | now | End time |

```json
{
  "total_requests": 1234,
  "avg_latency_ms": 45.2,
  "status_codes": {"2xx": 1100, "4xx": 100, "5xx": 34},
  "points": [
    {"timestamp": "...", "status_code": 200, "latency_ms": 42.0, "method": "GET", "path_pattern": "/users/{id}"}
  ]
}
```

## Log Streaming

### `GET /api/apps/{slug}/logs` (WebSocket)

Stream container logs via WebSocket.

| Param | Default | Description |
|-------|---------|-------------|
| `follow` | `true` | Live stream |
| `tail` | `100` | Historical lines |
| `since` | | Start timestamp |
| `service` | `web` | Compose service name |

Messages:
```json
{"ts": "2026-04-08T12:00:00Z", "stream": "stdout", "line": "Server started on :3000"}
```

## Backup Endpoints

### `GET /api/apps/{slug}/backups/configs`

List backup configurations for an app.

### `POST /api/apps/{slug}/backups/configs`

Create backup configuration.

```json
{
  "strategy": "postgres",
  "target": "s3",
  "schedule_cron": "0 2 * * *",
  "target_config_json": "{\"endpoint\":\"s3.amazonaws.com\",\"bucket\":\"backups\",\"prefix\":\"simpledeploy/\",\"access_key\":\"...\",\"secret_key\":\"...\",\"region\":\"us-east-1\"}",
  "retention_count": 7
}
```

Strategies: `postgres` (pg_dump), `volume` (tar+gzip).
Targets: `s3` (S3-compatible), `local` (filesystem).

### `DELETE /api/backups/configs/{id}`

Delete backup configuration.

### `GET /api/apps/{slug}/backups/runs`

List backup runs (most recent first).

### `POST /api/apps/{slug}/backups/run`

Trigger immediate backup. Returns 202 Accepted (runs async).

### `POST /api/backups/restore/{id}`

Restore from a backup run. Returns 202 Accepted (runs async).

## Alert Endpoints

### `GET /api/webhooks`

List configured webhooks.

### `POST /api/webhooks`

Create webhook.

```json
{
  "name": "slack-alerts",
  "type": "slack",
  "url": "https://hooks.slack.com/services/...",
  "template_override": "",
  "headers_json": ""
}
```

Types: `slack`, `telegram`, `discord`, `custom`.

### `DELETE /api/webhooks/{id}`

Delete webhook.

### `GET /api/alerts/rules`

List alert rules. Optional `?app_id=N` filter.

### `POST /api/alerts/rules`

Create alert rule.

```json
{
  "app_id": 1,
  "metric": "cpu_pct",
  "operator": ">",
  "threshold": 80,
  "duration_sec": 300,
  "webhook_id": 1,
  "enabled": true
}
```

Metrics: `cpu_pct`, `mem_bytes`, `mem_pct`.
Operators: `>`, `<`, `>=`, `<=`.

### `PUT /api/alerts/rules/{id}`

Update alert rule.

### `DELETE /api/alerts/rules/{id}`

Delete alert rule.

### `GET /api/alerts/history`

List alert history. Optional `?rule_id=N&limit=50`.

## User Endpoints

All require `super_admin` role except API key management.

### `GET /api/users`

List users.

### `POST /api/users`

Create user.

```json
{"username": "dev", "password": "secret", "role": "admin"}
```

### `DELETE /api/users/{id}`

Delete user.

### `POST /api/users/{id}/access`

Grant app access to user.

```json
{"app_slug": "myapp"}
```

### `DELETE /api/users/{id}/access/{slug}`

Revoke app access.

### `GET /api/apikeys`

List current user's API keys.

### `POST /api/apikeys`

Create API key. Returns plaintext key (shown once).

```json
// Request
{"name": "ci-deploy"}

// Response 201
{"id": 1, "name": "ci-deploy", "key": "sd_a1b2c3..."}
```

### `DELETE /api/apikeys/{id}`

Revoke API key.

## Registry Endpoints

### `GET /api/registries`

List configured registries (passwords redacted).

```json
[{"id": "abc123", "name": "ghcr-org", "url": "ghcr.io", "username": "myuser", "created_at": "...", "updated_at": "..."}]
```

### `POST /api/registries`

Add a registry.

```json
// Request
{"name": "ghcr-org", "url": "ghcr.io", "username": "myuser", "password": "mytoken"}

// Response 201
{"id": "abc123", "name": "ghcr-org", "url": "ghcr.io", "username": "myuser", "created_at": "...", "updated_at": "..."}
```

### `PUT /api/registries/{id}`

Update a registry. Same request body as create.

### `DELETE /api/registries/{id}`

Delete a registry. Returns `{"status": "ok"}`.
