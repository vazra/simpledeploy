---
title: Security audit
description: Findings from the 2026-04-10 full-codebase security audit, including fixes applied and accepted risks.
---

**Date:** 2026-04-10
**Auditor:** Security Review (Automated)
**Scope:** Full codebase - auth, API, store, deployer, docker, proxy, backup, alerts, UI
**Status:** 27 of 28 fixed, 1 won't-fix (#10 CSRF, #14 template override - SameSite/admin-only sufficient)

Legend: [FIXED] = patched, [OPEN] = not yet addressed

---

## CRITICAL

### 1. [FIXED] Hardcoded Default JWT Secret

- **Location:** `cmd/simpledeploy/main.go:304-307`
- **Issue:** Falls back to `"simpledeploy-default-secret"` when `master_secret` is empty. Attacker can forge JWTs for any user/role.
- **Fix:** Refuse to start without `master_secret`, or auto-generate and persist a random one.

### 2. [FIXED] WebSocket Accepts All Origins

- **Location:** `internal/api/logs.go:16-18`
- **Issue:** `CheckOrigin` returns `true` unconditionally. Enables Cross-Site WebSocket Hijacking on `/api/apps/{slug}/logs` and `/api/apps/{slug}/deploy-logs`.
- **Fix:** Validate `Origin` header against configured domain.

### 3. [FIXED] Path Traversal in App Name

- **Location:** `internal/api/deploy.go:59`
- **Issue:** `body.Name` used directly in `filepath.Join(s.appsDir, body.Name)` with no validation. Names like `../../etc/cron.d` escape the apps directory.
- **Fix:** Validate app names against `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`.

### 4. [FIXED] No Compose File Content Validation

- **Location:** `internal/api/deploy.go:53-73`
- **Issue:** User-supplied compose YAML is written and executed without checking for `privileged: true`, `network_mode: host`, dangerous capabilities, or sensitive host mounts.
- **Fix:** Parse and validate compose file before writing. Reject dangerous directives.

### 5. [FIXED] SSRF via Webhook URLs

- **Location:** `internal/alerts/webhook.go:49`
- **Issue:** No validation of webhook URL. Can target cloud metadata endpoints, localhost, or `file://` URIs.
- **Fix:** Validate URL scheme (https only), resolve DNS, reject private/reserved IP ranges.

---

## HIGH

### 6. [FIXED] API Key Expiry Never Checked

- **Location:** `internal/api/middleware.go:33-41`
- **Issue:** `expires_at` column exists but middleware never checks it. Expired keys work forever.
- **Fix:** Add `time.Now().After(*keyRecord.ExpiresAt)` check.

### 7. [FIXED] Any User Can Delete Any API Key (IDOR)

- **Location:** `internal/api/users.go:208-225`
- **Issue:** Deletes by `id` without verifying ownership. User A can delete User B's keys.
- **Fix:** Verify `keyRecord.UserID == user.ID` before deletion.

### 8. [FIXED] Missing `Secure` Flag on Session Cookie

- **Location:** `internal/api/auth.go:55-61`
- **Issue:** JWT sent in cleartext over HTTP.
- **Fix:** Add `Secure: true`.

### 9. [FIXED] Logout Cookie Missing Security Attributes

- **Location:** `internal/api/auth.go:71-75`
- **Issue:** Only sets `Name` and `MaxAge: -1`. Browser may not match original cookie.
- **Fix:** Mirror all attributes from login cookie.

### 10. [WONTFIX] No CSRF Protection

- **Issue:** No CSRF tokens on state-changing endpoints. `SameSite=Strict` helps but doesn't cover older browsers.
- **Decision:** SameSite=Strict sufficient. API key auth (Bearer) is inherently CSRF-safe. All modern browsers support SameSite.

### 11. [FIXED] Missing Security Response Headers

- **Location:** `internal/api/server.go`
- **Issue:** No `X-Frame-Options`, `X-Content-Type-Options`, `Content-Security-Policy`, `Strict-Transport-Security`.
- **Fix:** Add security headers middleware.

### 12. [FIXED] Backup Files World-Readable

- **Location:** `internal/backup/local.go:22-25`
- **Issue:** `os.Create` uses default permissions (0666 & ~umask).
- **Fix:** Use `os.OpenFile` with `0600`.

### 13. [FIXED] Webhook Header Injection

- **Location:** `internal/alerts/webhook.go:55-62`
- **Issue:** User-controlled headers can override `Host`, `Authorization`, etc.
- **Fix:** Whitelist allowed header names.

### 14. [WONTFIX] Template Override in Webhooks

- **Location:** `internal/alerts/webhook.go:39`
- **Issue:** `text/template` with user-supplied template. Combined with SSRF, allows crafting arbitrary request bodies.
- **Decision:** SSRF is now blocked. Only super_admins can set template overrides. Accepted risk for a trusted role.

---

## MEDIUM

### 15. [FIXED] SQL String Concatenation for Table Name

- **Location:** `internal/store/system.go:65`
- **Issue:** `table` parameter concatenated into SQL. Currently internal-only but fragile.
- **Fix:** Whitelist with `switch` statement.

### 16. [FIXED] User Enumeration via Timing

- **Location:** `internal/api/auth.go:33-41`
- **Issue:** Missing user returns fast; wrong password returns after bcrypt (~250ms).
- **Fix:** Always run bcrypt with a dummy hash on user-not-found.

### 17. [FIXED] Weak Key Derivation for Encryption

- **Location:** `internal/auth/crypto.go:57-59`
- **Issue:** Single SHA-256 pass is not a proper KDF.
- **Fix:** Use `pbkdf2.Key()` or `argon2.IDKey()`.

### 18. [FIXED] No Account Lockout

- **Issue:** No tracking of failed login attempts per username.
- **Fix:** Progressive lockout after 10 failures (1m, 2m, 4m... 30m cap). Tracks per-username and per-IP.

### 19. [FIXED] Rate Limiter Ineffective Behind Proxy

- **Location:** `internal/api/auth.go:14-16`
- **Issue:** Uses `r.RemoteAddr` only. Behind proxy, all requests share one IP.
- **Fix:** Added `trusted_proxies` config. `auth.RealIP()` extracts real client IP from X-Forwarded-For when behind trusted proxy.

### 20. [FIXED] Error Messages Leak Internal Details

- **Issue:** ~67 instances of `http.Error(w, err.Error(), ...)` across API layer.
- **Fix:** Log errors server-side, return generic messages to clients.

### 21. [FIXED] Dynamic SQL in Metrics Aggregation

- **Location:** `internal/store/metrics.go:126-143`, `internal/store/reqstats.go:100-123`
- **Issue:** `fmt.Sprintf` inserts bucket into SQL. Whitelist-validated but fragile.
- **Fix:** Validate tier in store functions.

### 22. [FIXED] CLI Credentials in Shell History

- **Location:** `cmd/simpledeploy/main.go`
- **Issue:** `--password` and `--username` flags visible in shell history and `ps`.
- **Fix:** `--password` now optional. Reads from `SD_PASSWORD` env var or prompts securely via stdin (no echo).

### 23. [FIXED] Path Traversal in Backup Filenames

- **Location:** `internal/backup/local.go:21,34,42`
- **Issue:** `filename` from DB used in `filepath.Join` without validation.
- **Fix:** Validate filename contains no path separators or `..`.

### 24. [FIXED] No Cookie MaxAge on Login

- **Location:** `internal/api/auth.go:55-61`
- **Issue:** Session cookie has no explicit expiry. Browser-dependent behavior.
- **Fix:** Set `MaxAge` to match JWT expiry (24h).

---

## LOW

### 25. [FIXED] API Key Hash Unsalted

- **Location:** `internal/auth/apikey.go:21-25`
- **Issue:** SHA-256 without salt. Keys are random so low practical risk.
- **Fix:** Switched to HMAC-SHA256 keyed by master_secret. DB theft without master_secret makes hashes useless.

### 26. [FIXED] No Audit Logging

- **Issue:** No logging of login attempts, role changes, deployments, API key operations.
- **Fix:** Structured JSON audit log to stderr with in-memory ring buffer (500 entries). GET /api/system/audit-log endpoint (super_admin only). Events: login, login_failed, user_created, user_deleted, apikey_created, apikey_deleted, deploy.

### 27. [FIXED] XSS Risk in DataTable Component

- **Location:** `ui/src/components/DataTable.svelte:23`
- **Issue:** `{@html col.render(row)}` renders unsanitized HTML.
- **Fix:** Replaced `{@html}` with `{}` text interpolation (Svelte auto-escapes).

### 28. [FIXED] Database File Permissions Not Set

- **Location:** `internal/store/store.go`
- **Issue:** SQLite file created with default permissions. Should be `0600`.
