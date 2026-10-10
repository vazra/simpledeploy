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
- `tv` is bumped on logout, password change, and role change, so a stolen
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

Viewers see `.env` variable names but not their values (`masked: true` in the API). Manage and super_admin users see values.

### API Key Ownership

Users can only delete their own API keys. `super_admin` can delete any key.

## Deployment Safety

### App Name Validation

App names must match `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`. This prevents:
- Path traversal attacks (`../../etc/cron.d`)
- Null byte injection
- Filesystem escapes

### Compose File Validation

Every compose file is parsed the way `docker compose` sees it (variables
filled in from the app's `.env`) and checked before any container starts:
on deploy, import, rollback, version restore, pull, scale, and when the
reconciler picks up a file from disk. Rejected directives include
privileged mode, host or cross-container namespaces, dangerous
capabilities, security options that disable AppArmor/seccomp/SELinux,
devices, and bind mounts of system folders, SimpleDeploy's data folder or
other apps' folders. Files the compose loader reads from the host
(`env_file`, `secrets`, `configs`, build contexts) must stay inside the
app folder, and symlinked `docker-compose.yml` / `.env` files are refused.

The full list is in [Compose labels: security validation](/reference/compose-labels/#compose-security-validation).

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

All dashboard and API responses include:

```
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: camera=(), microphone=(), geolocation=()
Strict-Transport-Security: max-age=63072000  (when the request arrived over HTTPS)
```

Behind a TLS-terminating proxy listed in `trusted_proxies`, `X-Forwarded-Proto: https` counts as HTTPS for HSTS and for the session cookie's `Secure` flag. The header is ignored from untrusted peers.

Responses from your apps get `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: strict-origin-when-cross-origin` and, on TLS endpoints, `Strict-Transport-Security: max-age=31536000; includeSubDomains`, each only when the app does not send that header itself. See [default security headers](/concepts/endpoints-and-routing/#default-security-headers).

### Request Size Limits

All non-GET requests are limited to 1MB body size to prevent memory exhaustion from oversized payloads.

### WebSocket Security

WebSocket endpoints (app logs, deploy logs, realtime events, process logs) require the browser's `Origin` host and port to match the dashboard host (or `Sec-Fetch-Site: same-origin`), so pages on other ports or subdomains cannot open them with your session. Every 60 seconds each socket re-checks that the user still exists, has the same role, still holds a valid session or API key, and still has access to the app; otherwise it closes (code 1008). Container log frames larger than 1 MiB end the stream instead of being buffered. Idle connections are closed after 5 minutes.

### Cross-Site Request Protection

The session cookie is `SameSite=Strict`, but browsers treat sibling subdomains (`app.example.com` and `manage.example.com`) as the same site. All state-changing API requests are therefore checked with Go's cross-origin protection: requests a browser marks as cross-origin (`Sec-Fetch-Site`, or `Origin` not matching `Host`) get `403`. The CLI, curl, and git webhooks send neither header and are unaffected. This also protects first-run setup (`POST /api/setup`) from being triggered by another site in your browser.

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
master_secret: "<output of openssl rand -hex 32>"
```

### Backup Security Enhancements

- Database credentials for backup scripts (MySQL, PostgreSQL, MongoDB) are passed via Docker environment variables, never embedded in shell scripts
- Backup download paths are validated to prevent path traversal
- Error messages from backup operations are truncated to prevent credential leakage in logs
- Certificate upload requires valid PEM format and DNS-safe domain names

## Breaking Changes on Upgrade

### Upgrading to the next release after 1.4.3

- **Header names with a dot are no longer passed to apps.** The built-in proxy (Caddy 2.11.7) drops request headers whose names contain `.`, as it already did for `_`. An app that reads such a header must switch to a name made of letters, digits and `-`. The same applies to `simpledeploy.ratelimit.by: header:NAME`: with `.` or `_` in the name, every request shares one bucket.
- **Stalled uploads are cut after a minute.** A request body to an `http` endpoint that sends no data for one minute is closed (slow-client protection). Normal slow uploads that keep sending are not affected, and `grpc`/`h2c` endpoints have no such limit.

### Upgrading to the next release after 1.4.2

- **Placeholder `master_secret` values are flagged.** If your config still contains an example value from the docs (for example `change-me-to-a-random-string`), the server logs a warning at startup and signs sessions with a random key stored in `data_dir/session-signing.key` instead of the public value (everyone signs in again once). Stored credentials are still encrypted with the public value, so replace it with `openssl rand -hex 32` soon, then re-enter registry and S3 credentials and re-create API keys.
- **Stricter compose validation.** Refused: bind mounts of system folders (including `/home`, `/usr`, `/var/lib`, writable `/var/log`), other apps' folders, a writable bind of the app folder itself, binds nested inside another writable bind, host files via `env_file`/`label_file`/`extends`/`secrets`/`configs`, `include`, `use_api_socket`, privileged `post_start`/`pre_stop` hooks, `provider` services, networks named `host`, and symlinked `docker-compose.yml`/`.env`. Apps already running keep their routes while their compose file is unchanged; redeploys, pulls, scaling and endpoint/IP-access edits show the reasons until the file is fixed. To allow a specific host folder (for example a media library under `/home`), list it in `allowed_bind_paths` in `config.yaml`.
- **Private S3 endpoints need an opt-in.** MinIO or other S3 endpoints on loopback, private, link-local or CGNAT/Tailscale addresses require `SIMPLEDEPLOY_ALLOW_PRIVATE_S3=1`; existing backup configs pointing at them fail until it is set.
- **Git sync pulls are narrower.** `_global.yml` is push-only, pulled access-grant edits are reverted, and symlinks in the repo block the apply.
- **Endpoint domains belong to one app.** Each domain is served by one app, even on different paths. When several apps claim a domain, the app that already serves it keeps it while it still claims it; otherwise (including after a restart) the app created first (lowest app ID) wins. The other apps' routes on that domain are dropped and logged. Endpoint edits onto another app's domain, or (for non-super_admin users) onto the dashboard `domain`, are refused, and custom certs can only be uploaded for an app's own endpoints, with a key that matches the certificate and a certificate valid for the domain. Non-super_admin users cannot add wildcard (`*`) endpoint domains.
- **Custom alert webhook templates receive JSON-escaped values.** Every string field (app name, metric, status and so on) is escaped for use inside a JSON string before your template runs, so a template that escaped values itself may now double-escape them. Remove your own escaping and place the fields directly inside quotes.
- **`.env` edits are checked like a deploy.** Saving variables that would make the compose file fail the security checks, or point an endpoint at another app's domain, is refused with the reasons.
- **Restore limits.** Uploaded restore archives are capped at 8 GiB decompressed (`SIMPLEDEPLOY_RESTORE_MAX_GB` changes it); at most 4 restores run at once.
- **Viewers see `.env` keys only.** Values come back empty with `masked: true`; manage and super_admin users see values.
- **`simpledeploy init` keeps an existing config.** Pass `--force` to overwrite it.

### Older releases

- **API keys must be re-created.** Key hashing changed from SHA-256 to HMAC-SHA256. Existing keys in the database will not match.
- **Registry credentials are auto-migrated.** New encryption uses random per-encryption salt (PBKDF2). Existing credentials encrypted with the fixed salt are automatically decrypted and re-encrypted on first access.
- **`master_secret` is required.** The server refuses to start without it.
