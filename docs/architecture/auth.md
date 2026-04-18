---
title: Auth and encryption
description: Password hashing, JWT sessions, API keys, rate limiting, credential encryption.
---

The `internal/auth/` package owns password hashing, sessions, API keys, rate limiting, and credential encryption.

## Passwords

Hashed with bcrypt (default cost). Plaintext is never stored or logged. The `users create` CLI reads from `SD_PASSWORD` if the `--password` flag is omitted, so passwords stay out of shell history.

## Sessions

Sessions are JWTs signed with a key derived from `master_secret`. The token is set as an HttpOnly cookie (`simpledeploy_session`) and also accepted as a `Bearer` header for programmatic access. Default expiry is short (hours); the UI silently refreshes.

## API keys

API keys are minted as `sd_<random>`. Only an HMAC-SHA256 of the key is stored. The plaintext is shown to the user once at creation and never again. The HMAC key is derived from `master_secret`.

API keys inherit the creator's role and per-app grants at the moment of creation. Revocation is immediate (delete the row).

## Rate limiting

A per-IP token bucket limits authentication attempts and dashboard requests. Defaults: 200 requests per 60 seconds with a burst of 50, keyed by IP. Configurable in `config.yaml`. Apps can override per-route via compose labels.

A separate, stricter limit on `/api/auth/login` blocks credential stuffing. After repeated failures, the account is temporarily locked.

## Credential encryption

Registry passwords, S3 keys, and webhook secrets are encrypted at rest with AES-256-GCM. The key is derived from `master_secret` via PBKDF2 with a per-record random salt. The legacy fixed-salt format is still readable so older databases keep working.

`master_secret` itself is plain text in `config.yaml`. Treat it like a database root credential: file mode 0600, owned by the simpledeploy user, never committed, rotated only with care (rotation re-encrypts every credential row).

## Audit log

All write actions on the API (login, deploy, role change, key issuance) are logged to `audit_log` with user id, action, target, and timestamp. Retention is configurable.
