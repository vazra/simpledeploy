---
title: Auth and encryption
description: Password hashing, JWT sessions, API keys, rate limiting, credential encryption.
---

The `internal/auth/` package owns password hashing, sessions, API keys, rate limiting, and credential encryption.

## Passwords

Hashed with bcrypt (default cost). Plaintext is never stored or logged. The `users create` CLI reads from `SD_PASSWORD` if the `--password` flag is omitted, so passwords stay out of shell history.

## Sessions

Sessions are JWTs signed with a key derived from `master_secret`. The token is set as an HttpOnly, `SameSite=Strict` cookie (`session`) and also accepted as a `Bearer` header for programmatic access. Tokens expire after 24 hours.

If `master_secret` is a known placeholder from the docs, `cmd/simpledeploy` signs sessions with a random key from `data_dir/session-signing.key` instead (created 0600 on first start, see `sessionkey.go`). Stored credentials and API key hashes still use `master_secret`.

The cookie gets `Secure` when TLS mode is not `off` or `auth.RequestIsHTTPS` reports HTTPS. That is true when TLS terminated in-process, or when the direct peer is in `trusted_proxies` and sent `X-Forwarded-Proto: https` (the header is ignored from other peers). The same check gates `Strict-Transport-Security: max-age=63072000` (no `includeSubDomains`, since apps on sibling subdomains may serve plain HTTP).

## Cross-origin protection

`Server.Handler()` wraps the mux in Go's `http.CrossOriginProtection`. Non-GET/HEAD/OPTIONS requests that a browser marks as cross-origin (`Sec-Fetch-Site`, falling back to `Origin` vs `Host`) get `403`. Sibling subdomains count as same-site, so `SameSite=Strict` alone does not cover them. The dashboard `domain` is added as a trusted origin (`https://`, plus `http://` when TLS is off) for proxies that rewrite `Host`. CLI, curl and git webhooks send neither header and pass.

WebSocket upgrades are GETs, so `checkWebSocketOrigin` (`internal/api/logs.go`) checks them: `Origin` must match `Host` including port (default port filled in from the scheme), or the browser must send `Sec-Fetch-Site: same-origin`. Bearer-authenticated clients may omit `Origin`. Every socket re-runs the auth checks every 60s (`watchWSAuth` / `wsAuthStillValid`): user still exists with the same role, session token version or API key still valid, and (for app sockets) app access still granted. On failure it closes with code 1008; the events socket instead drops `app:` topics the user lost.

## API keys

API keys are minted as `sd_<random>`. Only an HMAC-SHA256 of the key is stored. The plaintext is shown to the user once at creation and never again. The HMAC key is derived from `master_secret`.

API keys inherit the creator's role and per-app grants at the moment of creation. Revocation is immediate (delete the row).

## Rate limiting

A per-IP token bucket limits authentication attempts and dashboard requests. Defaults: 200 requests per 60 seconds with a burst of 50, keyed by IP. Configurable in `config.yaml`. Apps can override per-route via compose labels.

A separate, stricter limit on `/api/auth/login` blocks credential stuffing. After repeated failures, the account is temporarily locked.

## Credential encryption

Registry passwords, S3 keys, and webhook secrets are encrypted at rest with AES-256-GCM. The key is derived from `master_secret` via PBKDF2 with a per-record random salt. The legacy fixed-salt format is still readable so older databases keep working.

`master_secret` itself is plain text in `config.yaml`. Treat it like a database root credential: file mode 0600, owned by the simpledeploy user, never committed, rotated only with care (rotation re-encrypts every credential row).

`config.Load` warns at startup when `master_secret` is a known docs placeholder (`PlaceholderMasterSecret`; also any `<...>` value) or shorter than 32 characters. It does not refuse to start, since changing the secret makes stored credentials unreadable. `simpledeploy init` writes a random secret and refuses to overwrite an existing config unless `--force` is passed.

## Audit log

All write actions on the API (login, deploy, role change, key issuance) are logged to `audit_log` with user id, action, target, and timestamp. Retention is configurable.
