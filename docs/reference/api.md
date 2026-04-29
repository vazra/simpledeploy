---
title: REST API
description: Management API endpoints served on the configured management_port, covering apps, metrics, backups, alerts, users, and registries.
---

Management API served on the configured `management_port` (default: 8443).

## Authentication

All endpoints except `/api/health`, `/api/auth/login`, `/api/auth/logout`, and `/api/setup` require authentication.

**Session (UI):** POST to `/api/auth/login`, receive JWT in httpOnly cookie.

**API Key (CLI/automation):** Include `Authorization: Bearer sd_...` header.

**Roles:** every authenticated request is evaluated against the caller's role (`super_admin`, `manage`, `viewer`) plus their `user_app_access` grants. App-scoped routes (`/api/apps/{slug}/...`) return `404` for callers without access. Platform routes (`/api/docker/*`, `/api/system/info`, `/api/system/audit-config`, `/api/backups/test-s3`, user CRUD, registries, git sync, DB backups) require `super_admin`. See [Users and roles](/guides/users-roles/) for the full matrix.

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
{"username": "dev", "password": "secret", "role": "manage"}
```

### `DELETE /api/users/{id}`

Delete user.

### `GET /api/users/{id}/access`

List app slugs the user has been granted. Returns `[]string`. `super_admin` users have implicit access to all apps and return `[]`.

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

## Git Sync

For setup and configuration see [Git sync](/operations/git-sync/).

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| `GET` | `/api/git/status` | super admin | Current sync state, toggle flags, commits-behind counter |
| `POST` | `/api/git/webhook` | HMAC (`X-Hub-Signature-256`) | Webhook-triggered immediate sync |
| `POST` | `/api/git/sync-now` | super admin | Force an immediate pull-and-apply cycle |
| `GET` | `/api/git/config` | super admin | Read active git sync config (secrets redacted) |
| `PUT` | `/api/git/config` | super admin | Update git sync config; DB values override YAML |
| `POST` | `/api/git/disable` | super admin | Disable git sync and clear DB config |
| `POST` | `/api/git/apply-pending` | super admin | Apply fetched remote commits when `auto_apply_enabled=false` |

### `GET /api/git/status`

Super admin only. Returns a snapshot of the sync worker state.

```json
{
  "Enabled": true,
  "Remote": "git@github.com:owner/repo.git",
  "Branch": "main",
  "HeadSHA": "a1b2c3d...",
  "LastSyncAt": "2026-04-19T10:00:00Z",
  "LastSyncError": "",
  "PendingCommits": 0,
  "DroppedRequests": 0,
  "RecentConflicts": [],
  "RecentCommits": [],
  "PollEnabled": true,
  "AutoPushEnabled": true,
  "AutoApplyEnabled": true,
  "WebhookEnabled": true,
  "CommitsBehind": 0,
  "PendingApply": false
}
```

`CommitsBehind` is non-zero only when `AutoApplyEnabled=false` and the remote has commits that have not been applied. `PendingApply` is `true` when `CommitsBehind > 0` and `AutoApplyEnabled=false`.

### `POST /api/git/webhook`

Public endpoint, HMAC-verified via `X-Hub-Signature-256`. Triggers an immediate sync. Only available when `git_sync.webhook_secret` is configured and `WebhookEnabled=true`. Returns `404` with an empty body when the webhook toggle is disabled.

Returns `202 Accepted` on success, `401` if the signature is invalid.

### `POST /api/git/sync-now`

Super admin only. Forces an immediate fetch-and-apply cycle regardless of `AutoApplyEnabled`. Rate-limited to prevent abuse.

Returns `{"status": "ok"}` once the cycle completes.

### `GET /api/git/config`

Super admin only. Returns the active git sync configuration. Sensitive fields (`https_token`, `webhook_secret`, SSH key content) are replaced with `"<redacted>"` or a boolean `_set` flag. A `source` field (`"db"` or `"yaml"`) indicates which layer is providing each value.

### `PUT /api/git/config`

Super admin only. Updates git sync configuration. Accepts a partial or full config object. Values written here are stored in the database and override `config.yml` immediately without a restart. Includes the four behaviour toggles:

```json
{
  "enabled": true,
  "remote": "git@github.com:owner/infra.git",
  "branch": "main",
  "poll_enabled": true,
  "auto_push_enabled": true,
  "auto_apply_enabled": false,
  "webhook_enabled": true
}
```

Returns the updated (redacted) config on success.

### `POST /api/git/disable`

Super admin only. Disables git sync and clears all DB-stored config, reverting to the `config.yml` defaults. The `.git` directory and local history in `apps_dir` are not removed.

Returns `{"status": "ok"}`.

### `POST /api/git/apply-pending`

Super admin only. Runs a full fetch, rebase (server-wins conflict resolution), sidecar import, and reconcile cycle on demand. Intended for use when `auto_apply_enabled=false` and you want to apply the pending remote commits after review.

If the remote is already up-to-date, the operation is a no-op and returns success. If `git sync` is not configured, returns `503`.

Returns `{"status": "ok"}` on success.
