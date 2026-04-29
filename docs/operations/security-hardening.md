---
title: Security hardening
description: Defense-in-depth controls across auth, deployment, data storage, and network layers, plus operational best practices.
---

SimpleDeploy includes defense-in-depth security across authentication, deployment, data storage, and network layers. This document covers what's protected, how to configure it, and operational best practices.

## Quick Checklist

Before going to production:

- [ ] Set a strong `master_secret` in config (at least 32 random characters)
- [ ] Enable TLS (`tls.mode: auto` or behind a TLS-terminating proxy)
- [ ] Set `trusted_proxies` if running behind a load balancer
- [ ] Create a named admin account and delete any default users
- [ ] Store API keys securely (they are shown only once at creation)
- [ ] Review deployed compose files for privileged containers

## Authentication

### Password Security

- Passwords are hashed with **bcrypt** (cost factor 12, ~250ms per hash)
- Bcrypt's 72-byte limit applies silently; longer passwords are truncated
- No password is stored in plaintext anywhere in the system

### JWT Sessions

- Session tokens are signed JWTs (HS256) with 24-hour expiry. The signing
  key is HKDF-SHA256-derived from `master_secret` with purpose label
  `simpledeploy-jwt-v1` so it is domain-separated from credential
  encryption and API-key HMAC.
- Tokens carry `iss=simpledeploy`, `aud=simpledeploy-dashboard`, and a
  per-user `tv` (token version) claim. Bumping `tv` server-side
  invalidates all outstanding JWTs for that user.
- `tv` is bumped on logout, password change, and role change — a stolen
  cookie cannot outlive any of those events.
- Cookies are `HttpOnly`, `Secure` (when TLS), `SameSite=Strict`, `MaxAge=86400`.

### API Keys

- API keys use the `sd_` prefix followed by 64 hex characters (32 bytes of entropy)
- Keys are hashed with **HMAC-SHA256** using your `master_secret` before storage
- Even if the database is stolen, keys cannot be recovered without the master secret
- Keys support optional `expires_at` (set on create); expired keys are rejected at the middleware level
- `last_used_at` is updated lazily on every successful auth so operators can spot stale keys
- The plaintext key is shown exactly once at creation and never stored

### Login Rate Limit

The login endpoint has its own rate limiter, capped at **10 requests per
minute per client IP**. Login abuse cannot deplete the global request
budget and vice versa. `trusted_proxies` accepts CIDR ranges
(e.g. `10.0.0.0/8`); without a matching entry the proxy IP is treated as
the client.

### Account Lockout

After 10 failed login attempts per (username, IP) tuple, the account is
temporarily locked with progressive backoff. Locking is per-IP-per-user
so an attacker on one IP cannot DoS a victim's login from a different
IP. A locked-out attempt returns the same `401 invalid credentials` as
a wrong password, eliminating an enumeration tell.

| Failures over threshold | Lockout duration |
|------------------------|-----------------|
| 1 (11th attempt) | 1 minute |
| 2 | 2 minutes |
| 3 | 4 minutes |
| 4 | 8 minutes |
| 5 | 16 minutes |
| 6+ | 30 minutes (cap) |

Lockout is tracked per-username AND per-IP independently. A successful login resets both counters.

### Rate Limiting

The login endpoint is rate-limited to 10 requests per minute per client IP. This works alongside account lockout to prevent brute-force attacks.

## Authorization

### Role-Based Access Control

Three roles with increasing privilege:

| Role | Dashboard | Own Apps | All Apps | User Management | System |
|------|-----------|----------|----------|-----------------|--------|
| `viewer` | read | read | - | - | - |
| `admin` | read | read/write | - | - | - |
| `super_admin` | read | read/write | read/write | full | full |

### Per-App Access Control

Non-admin users can be granted access to specific apps via `user_app_access`. The `super_admin` role bypasses all app-level access checks.

### API Key Ownership

Users can only delete their own API keys. `super_admin` can delete any key.

## Deployment Safety

### App Name Validation

App names must match `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`. This prevents:
- Path traversal attacks (`../../etc/cron.d`)
- Null byte injection
- Filesystem escapes

### Compose File Validation

Every compose file is parsed and validated before deployment. The following directives are **rejected**:

| Directive | Reason |
|-----------|--------|
| `privileged: true` | Full host access, container escape |
| `network_mode: host` | Bypasses network isolation |
| `pid: host` | Access to host process namespace |
| `ipc: host` | Shared memory with host |
| `cap_add: ALL` | All Linux capabilities |
| `cap_add: SYS_ADMIN` | Mount/unmount, container escape |
| `cap_add: SYS_PTRACE` | Process debugging, secret extraction |
| `cap_add: NET_ADMIN` | Network reconfiguration |
| Bind mounts of `/etc`, `/proc`, `/sys`, `/dev`, `/root` | Sensitive host paths |
| Bind mounts of `/var/run/docker.sock` | Docker socket = root access |
| Volume paths containing `..` | Path traversal |

## Config Sync Storage

Config sidecar files are written with mode `0600` (owner read/write only). There are three sidecar locations: `{apps_dir}/{slug}/simpledeploy.yml` for per-app settings (alert rules, backup configs, access grants), `{apps_dir}/_global.yml` for git-safe global state (redacted: no password hashes, no secret URLs), and `{data_dir}/config.yml` for sensitive global state (users with bcrypt hashes, AES-GCM encrypted registry and backup credentials).

`{data_dir}/config.yml` is the most sensitive sidecar. It contains bcrypt password hashes and AES-GCM encrypted credential blobs. Without `master_secret`, those blobs are unrecoverable even if the file is intact. Keep `master_secret` in a password manager separate from the host.

`{apps_dir}/_global.yml` is redacted and git-safe: it contains no hashes and no secret URLs. It is the file intended for committing to a private repository when using two-way git sync. See [Config sidecars](/operations/config-sidecars/) for full details.

## Data Protection

### Encryption at Rest

- Registry credentials (username/password) are encrypted with **AES-256-GCM**
- Encryption keys are derived from `master_secret` using **PBKDF2** (100,000 iterations, SHA-256) with a random 16-byte salt per encryption
- Each encryption operation uses a random nonce and random salt
- Decryption is backwards compatible with the legacy fixed-salt format

### Database Security

- SQLite database file is set to `0600` (owner read/write only)
- WAL mode with foreign key constraints enforced
- All queries use parameterized statements (no SQL injection)
- Table names in dynamic queries are validated against a strict whitelist

### Backup Security

- Backup files are created with `0600` permissions (owner-only)
- Backup directories use `0700` permissions
- Filenames are validated to prevent path traversal
- S3 credentials are encrypted before storage (same AES-256-GCM scheme)

### Error Handling

Internal error messages (SQL errors, file paths, Docker output) are never exposed to API clients. Errors are logged server-side; clients receive only generic HTTP status messages.

## Network Security

### Response Headers

All responses include:

```
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: camera=(), microphone=(), geolocation=()
Strict-Transport-Security: max-age=63072000; includeSubDomains  (when TLS active)
```

### Request Size Limits

All non-GET requests are limited to 1MB body size to prevent memory exhaustion from oversized payloads.

### WebSocket Security

WebSocket endpoints (`/api/apps/{slug}/logs`, `/api/apps/{slug}/deploy-logs`) validate the `Origin` header against the request `Host`. Cross-origin WebSocket connections are rejected to prevent Cross-Site WebSocket Hijacking. Idle connections are closed after 5 minutes.

### Webhook Security

Outbound webhooks (for alerts) are protected against SSRF:

- Only `http://` and `https://` schemes are allowed (validated at create and update time)
- DNS resolution is checked before sending
- Requests to loopback, private, link-local, and cloud metadata IPs (169.254.169.254) are blocked
- Dangerous headers (`Host`, `Content-Length`, `Transfer-Encoding`) cannot be overridden via webhook config
- Header values containing `\r` or `\n` are silently rejected to prevent header injection

### Trusted Proxies

If SimpleDeploy runs behind a load balancer or reverse proxy, configure `trusted_proxies` so rate limiting and lockout use the real client IP instead of the proxy IP:

```yaml
trusted_proxies:
  - "127.0.0.1"
  - "10.0.0.1"
```

When the direct connection comes from a trusted proxy, the client IP is extracted from `X-Forwarded-For` (rightmost untrusted entry). Without this config, `RemoteAddr` is used directly.

## Activity & Audit Logging

Every security-relevant action is captured in the persistent activity log: auth events (`login`, `login_failed`, `password_changed`), user and API key CRUD, deploy outcomes, and all config changes. Entries are stored in SQLite with configurable retention (default 365 days).

View the log at System → Audit Log or via:

```
GET /api/activity?categories=auth,system&limit=100
```

See [Activity & Audit Log](/operations/security-audit/) for the full event list, retention configuration, and export.

## CLI Security

### Password Handling

The `--password` flag for `users create` and `registry add` commands is optional. When omitted, the password is read from:

1. `SD_PASSWORD` environment variable (for CI/scripted use)
2. Interactive stdin prompt with no echo (for terminal use)

This prevents passwords from appearing in shell history or `ps` output.

```bash
# Interactive (recommended)
simpledeploy users create --username admin --role super_admin

# Environment variable (CI)
SD_PASSWORD=hunter2 simpledeploy users create --username admin --role super_admin

# Flag (not recommended - visible in history)
simpledeploy users create --username admin --password hunter2 --role super_admin
```

## Configuration Reference

Security-related config fields:

| Field | Required | Description |
|-------|----------|-------------|
| `master_secret` | Yes | Encryption key for credentials + HMAC key for API key hashes + JWT signing. Use 32+ random characters. |
| `tls.mode` | No | `auto` (default), `custom`, or `off`. Use `auto` for production. |
| `tls.email` | For auto | ACME account email for Let's Encrypt. |
| `trusted_proxies` | No | List of proxy IPs to trust for X-Forwarded-For. |

### Generating a Master Secret

```bash
openssl rand -hex 32
```

Copy the output into your config:

```yaml
master_secret: "a1b2c3d4e5f6..."
```

### Backup Security Enhancements

- Database credentials for backup scripts (MySQL, PostgreSQL, MongoDB) are passed via Docker environment variables, never embedded in shell scripts
- Backup download paths are validated to prevent path traversal
- Error messages from backup operations are truncated to prevent credential leakage in logs
- Certificate upload requires valid PEM format and DNS-safe domain names

## Breaking Changes on Upgrade

If upgrading from an older version:

- **API keys must be re-created.** Key hashing changed from SHA-256 to HMAC-SHA256. Existing keys in the database will not match.
- **Registry credentials are auto-migrated.** New encryption uses random per-encryption salt (PBKDF2). Existing credentials encrypted with the fixed salt are automatically decrypted and re-encrypted on first access.
- **`master_secret` is now required.** The server will refuse to start without it. Previously it would fall back to a default.
