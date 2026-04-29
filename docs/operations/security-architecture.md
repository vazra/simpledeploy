---
title: Security architecture
description: Design overview for security researchers and downstream auditors. Cryptographic primitives, trust boundaries, and the controls that enforce them.
---

This page describes how SimpleDeploy is built from a security standpoint. It is intended for researchers, downstream auditors, and operators who want to understand the design choices before deploying.

It is **not** a vulnerability disclosure or an exploit guide. For reporting issues, see [SECURITY.md](https://github.com/vazra/simpledeploy/blob/main/SECURITY.md).

## Components and process model

A single Go binary that:

- Hosts a REST API + Svelte SPA on `management_addr:management_port` (default `127.0.0.1:8443`).
- Embeds Caddy v2 to terminate TLS for app traffic on `:80` / `:443`.
- Drives Docker via the local socket (`/var/run/docker.sock`) and the `docker compose` CLI.
- Persists state in a single SQLite file (`$data_dir/simpledeploy.db`, mode `0600`, WAL).

There is no second daemon, no message queue, no separate worker pool. Every action is in-process.

## Cryptographic primitives

| Purpose | Algorithm | Parameters |
|---|---|---|
| Password hashing | bcrypt | cost 12 |
| Session token | JWT HS256 | 24h expiry; `iss=simpledeploy`, `aud=simpledeploy-dashboard`, custom `tv` (token version) claim |
| JWT signing key | HKDF-SHA256 from `master_secret` | info=`simpledeploy-jwt-v1`, 32-byte output |
| API key | random 32 bytes from `crypto/rand` | `sd_` prefix + 64 hex |
| API key storage | HMAC-SHA256 keyed by `master_secret` | constant-time compare via DB index lookup |
| Credential at rest (registry, S3, gitsync token) | AES-256-GCM | random 16-byte salt + random 12-byte nonce; key via PBKDF2-HMAC-SHA256, 600k iterations (legacy 100k accepted on read) |
| Git webhook signature | HMAC-SHA256 | `hmac.Equal` constant-time compare |
| TLS automation | ACME via Caddy + CertMagic | local CA mode also available |

`master_secret` is operator-supplied at install time and persisted in `config.yaml` (mode `0600`). It is the single root of trust for all symmetric crypto in the binary. Different purposes derive subkeys via HKDF where backward-compat permits; existing AES-GCM ciphertexts and API key HMACs continue to use the master directly to keep stored data decryptable.

## Authentication

Two parallel paths reach the same `AuthUser` context:

- `Authorization: Bearer sd_<hex>` → API-key path. The full key is hashed with the master HMAC and compared against `api_keys.key_hash` (UNIQUE indexed). Expired keys are rejected at the middleware. `last_used_at` is lazy-updated.
- `Cookie: session=<jwt>` → JWT path. The token is verified (alg pinned to HMAC), issuer/audience checked, the user fetched, and `claims.tv` compared against `users.token_version`.

Both paths populate an `audit.Ctx` (actor user id, name, source, IP) carried through the request context, so every recorded mutation attributes to a real principal.

## Authorization

Three roles: `super_admin`, `manage`, `viewer`. Per-app grants in `user_app_access` extend `manage`/`viewer` to specific apps. Middleware:

- `authMiddleware` — required on every authenticated route.
- `appAccessMiddleware` — read access to `/api/apps/{slug}/…`. super_admin bypass.
- `mutatingAppMiddleware` — same as above but rejects viewers.
- `superAdminMiddleware` — super_admin only.

For routes keyed by a body or referenced row id (e.g. `PUT /api/backups/configs/{id}`), the handler resolves the underlying app id and calls `canMutateForApp`. The router registration in `internal/api/server.go` is the source of truth for which middleware applies where.

## Session invalidation

`users.token_version` is bumped server-side on:

- **Logout** (best-effort: the unauthenticated logout endpoint reads and validates the cookie before bumping).
- **Password change** (`UpdatePassword`).
- **Role change** (`UpdateUserRole`).

JWTs minted before any of those events fail the `tv` check on the next request and are rejected.

## Network exposure

Default bindings:

- `:80`, `:443` — Caddy. Public-facing reverse proxy + ACME.
- `127.0.0.1:8443` — dashboard. Local-only by default; operators front it under a `manage.<domain>` route through Caddy if external access is needed.
- App `ports:` mappings are rewritten at deploy time to bind `127.0.0.1:` so the published port cannot be used to bypass per-app Caddy controls. Operator-explicit interface bindings (`0.0.0.0:`, `127.0.0.1:`, `[::1]:`) are preserved verbatim. The rewrite can be disabled globally with `SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK=true`.

The Caddy admin API (default `:2019`) is **disabled** programmatically. There is no pprof, no `/debug` endpoint.

## Outbound traffic from the dashboard

| Destination | When |
|---|---|
| Configured `recipes_index_url` | UI catalog browsing (HTTPS, same-host enforcement on sub-resources) |
| Operator-configured webhook URLs | Alert dispatch — public IPs only, with DNS-rebind protection in the dialer |
| Configured registries | Compose deploy (image pulls happen via the Docker daemon, not the binary) |
| Configured S3 endpoint | Backup target (operator-supplied creds) |
| Configured git remote | git sync (operator-supplied creds) |

The webhook dispatcher's HTTP client uses a custom `DialContext` that re-validates the resolved IP at connect time and rejects private, loopback, link-local, multicast, CGNAT, IETF-reserved, and class-E ranges.

## Compose validation

Compose files are validated on every code path that produces them: API deploy, bundle import, reconciler scan (catches gitsync / SSH side-channel writes), and rollback. Rejection rules are documented in [Compose labels](/reference/compose-labels/). The validator is in `internal/compose/validate.go` with unit-test coverage in `validate_test.go`.

## Restore archive validation

The `volume` and `sqlite` restore strategies pre-walk the uploaded tar (`internal/backup/tarsafe.go`) and reject:

- absolute paths
- `..` segments
- symlinks and hardlinks
- block/char/fifo entries
- NUL in names

After validation the stream is replayed verbatim into `docker exec ... tar -xzf -` with `--no-same-owner --no-overwrite-dir`. Decompressed size is capped at 8 GiB by default. Concurrent restores are capped server-side.

## Audit trail

Every mutating endpoint records a row in `audit_log` with the actor, IP, source, before/after JSON snapshots (secrets redacted), and a pre-rendered summary. Two tamper-resistance properties:

- `DELETE /api/activity` (super_admin only) writes a sentinel `system/audit_purged` row immediately after the wipe, including the pre-purge row count and actor info. Anyone trying to wipe the trail leaves a row recording the wipe.
- App purge does **not** delete `audit_log` rows. The `app_id` FK is set to NULL while the denormalized `app_slug` is preserved, so the trail is intact even after the app is gone.

A super_admin can still tamper at the SQLite level. The trail is operator-trust-bound, not Byzantine-fault-tolerant.

## Logging

Process stdout/stderr is teed into an in-process ring buffer (`internal/logbuf`). Buffered messages are sanitized: ANSI/OSC escape sequences are stripped, ASCII control characters except tab are dropped, and any single line is truncated at 8 KiB. The buffer is exposed at `GET /api/system/process-logs` to super_admin only.

The api logger (`log.Printf("[api] …")`) writes structured-ish lines and is also captured by the buffer. Handler errors are routed through `httpError`, which logs server-side and returns generic `http.StatusText` to the client; `err.Error()` is not echoed.

## DoS / resource controls

- `http.Server.ReadHeaderTimeout = 10s`, `IdleTimeout = 120s` (read/write deadlines are per-handler so streaming WS is not killed).
- Per-path body limit: 32 MiB for `upload-restore`, 256 KiB for cert uploads, 1 MiB elsewhere.
- WS endpoints set `SetReadLimit(16 KiB)` and a 30s ping ticker; auth is rechecked every 60s.
- Login: dedicated 10/min/IP rate limiter.
- Account lockout: per-(username, IP) tuple, max 30 minute backoff. Locked-out attempts return `401 invalid credentials` (no enumeration tell).
- Webhook dispatcher: 10s overall timeout, 5s TLS handshake, 10s response-header.
- Restore concurrency: server-wide semaphore caps to 4.
- Decompression: 8 GiB cap on gzip readers in restore paths.

## Build and release integrity

The release pipeline is described in [`.github/workflows/release.yml`](https://github.com/vazra/simpledeploy/blob/main/.github/workflows/release.yml) and `.goreleaser.yml`. As of this writing:

- Builds run on GitHub-hosted Ubuntu runners.
- Artifacts are produced by `goreleaser` and attached to the GitHub release.
- Container images are pushed to GHCR.

Cryptographic signing of release artifacts (cosign), SBOM emission (syft), and SLSA provenance are tracked as roadmap items. Until they ship, downstream operators are expected to verify GitHub release commits against tag annotations and pin Docker images by digest after the first pull.

## Auditing the source

Recommended starting points for a code audit:

- `internal/api/server.go` — full route table.
- `internal/api/middleware.go` — auth + audit context plumbing.
- `internal/auth/` — JWT, API keys, password, lockout, real-IP, AES-GCM.
- `internal/compose/validate.go` — compose security validator.
- `internal/backup/tarsafe.go` — restore archive validator.
- `internal/proxy/proxy.go` — Caddy config builder + custom modules.
- `internal/store/migrations/` — schema history.
- `internal/audit/` — audit recorder + render.

Run `go test ./...` and `go test -race ./...` from a clean checkout. Run `cd ui && npm test` for the dashboard.

## Known design trade-offs

These are choices, not bugs. They are documented in [Threat model](/operations/threat-model/).

- super_admin is host-root-equivalent because it can deploy arbitrary compose to a daemon SimpleDeploy talks to as root.
- A super_admin who controls the host can rewrite the SQLite file directly. The audit trail is operator-trust-bound.
- The recipes index is fetched over HTTPS (TOFU) and is not yet cryptographically signed.
- `master_secret` rotation requires re-encrypting stored credentials and forces re-issuance of API keys.
