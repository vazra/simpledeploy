---
title: Threat model
description: Trust boundaries, assets, attacker classes, and how SimpleDeploy mitigates them.
---

SimpleDeploy targets a single host operated by a small team. The threat model below describes who we trust, what we protect, and the mitigations in place. See also the full [security audit](/operations/security-audit/) and [hardening guide](/operations/security-hardening/).

## Trust boundaries

1. **Operator** with shell on the host. Fully trusted. Owns `master_secret`, the database, and the binary.
2. **Dashboard user** with role `super_admin`, `admin`, or `viewer`. Authenticated via password + JWT or API key. Trusted within their role and per-app scope.
3. **Deployed app**. Untrusted. Could be compromised. Should not be able to harm SimpleDeploy or other apps beyond standard Docker isolation.
4. **Public network**. Untrusted.

## Assets

- `master_secret`: encrypts every credential at rest. Compromise rotates everything.
- SQLite database: passwords (hashed), encrypted credentials, full app and user inventory.
- Apps' on-disk volumes: arbitrary tenant data.
- TLS private keys: ACME-issued or operator-uploaded.
- Audit log: post-incident forensic trail.

## Attacker classes and mitigations

**Anonymous internet attacker hitting the dashboard or app endpoints.**
Mitigated by: per-route rate limit, per-IP login rate limit + lockout, TLS, HSTS, secure cookies, no autoindex, no debug routes, Caddy's HTTP hardening defaults. Apps can additionally enforce IP allowlists via the `simpledeploy.access.allow` label.

**Credential stuffing or brute force.**
Mitigated by: bcrypt password hashing, login rate limit, account lockout, JWT short expiry, audit log of failures.

**Malicious or compromised compose YAML.**
Mitigated by: compose validation rejects host bind mounts to sensitive paths, privileged mode, host network, host PID, host IPC, and dangerous capabilities by default. Compose files are versioned so suspicious changes are reviewable.

**Malicious or compromised app container.**
Mitigated by: standard Docker isolation. SimpleDeploy does not grant special access to apps; the API is on a separate port and requires auth.

**Webhook SSRF.**
Mitigated by: outbound webhook resolver refuses private, link-local, and loopback addresses by default. Opt-in via `SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS=1`.

**Cross-tenant access via the dashboard.**
Mitigated by: per-app access rows on every protected endpoint. Tested via integration tests.

**Theft of database file.**
Mitigated by: credentials encrypted at rest with AES-256-GCM. The attacker still needs `master_secret` (plain text on the host) to decrypt.

## Out of scope

- Multi-host orchestration security (run separate SimpleDeploy instances per trust zone).
- Compromise of the host kernel.
- Side-channel attacks on TLS.
