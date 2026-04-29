---
title: Threat model
description: What SimpleDeploy is designed to defend against, the trust assumptions it makes, and the design trade-offs operators should be aware of.
---

This page sets the scope for security analysis: what SimpleDeploy considers a threat, what it does not, and why. It pairs with [Security architecture](/operations/security-architecture/) (the *how*).

## Trust principals

| Principal | Trust level | Capability |
|---|---|---|
| Host root | Fully trusted | Owns the SQLite DB, `master_secret`, Docker socket, kernel. Anything below this is bounded by host root. |
| `simpledeploy` daemon | Fully trusted | Runs as root by design (docker.sock + privileged ports). Compromise of the daemon equals host root. |
| `super_admin` user | Equivalent to host root | Can deploy arbitrary compose to a daemon SimpleDeploy talks to as root. Treat super_admin as a privileged operator. |
| `manage` user (with grant) | Trusted within their app set | Can mutate their accessible apps, restore backups, change env, etc. Cannot reach platform-level config. |
| `viewer` user (with grant) | Read-only within their app set | Can view, download logs, fetch activity, but not mutate. |
| Authenticated client (cookie or API key) | Bounded by the user's role | The role+grants of the user the credential belongs to. |
| Unauthenticated network traffic | Untrusted | Reaches Caddy on `:80`/`:443`, the dashboard if exposed, public health/setup endpoints. Treated as adversarial. |

## In-scope adversaries

The following are explicitly part of the threat model. SimpleDeploy is designed to make these costly or impossible without legitimate credentials:

- **External network attacker on the public internet** trying to reach the dashboard, an app, or the docker socket.
- **External attacker on the same LAN** trying to bypass TLS, ride session cookies, or reach loopback-bound services.
- **Compromised compose file or recipe** trying to escape the container onto the host.
- **Compromised gitsync remote or backup tarball** trying to deliver a privileged compose or write outside the container's volume.
- **Hostile DNS or compromised CA** trying to redirect a webhook dispatch to an internal endpoint (DNS rebinding / SSRF).
- **Authenticated `viewer` or `manage` user** trying to escalate to platform-level access, read another user's apps, exfiltrate audit history, or smuggle through ID parameters.
- **Stolen JWT cookie or API key** trying to outlive logout / password change / role change.

## Out-of-scope adversaries

These are explicit non-goals. If your threat model includes them, layer additional controls below SimpleDeploy:

- **Host root compromise.** Once root, the attacker rewrites `config.yaml`, the SQLite DB, the systemd unit, or the daemon binary. SimpleDeploy is not a sandbox against root.
- **Hypervisor or kernel exploit** that escapes Docker's isolation. Mitigated by upstream Linux + Docker; we do not add a second layer.
- **Physical access to the host or backup target** without disk encryption.
- **Side-channel timing on bcrypt** beyond the dummy-hash equalization on user-not-found.
- **Side-channel power/EM analysis** of the AES-GCM implementation.
- **Long-running cryptanalytic attacks** on AES-256-GCM, HMAC-SHA256, HKDF-SHA256, or bcrypt cost 12.
- **Supply-chain compromise of upstream dependencies** (Go stdlib, Caddy, Docker SDK, modernc/sqlite). Tracked via `govulncheck` and Dependabot; not separately mitigated.
- **A super_admin acting maliciously.** super_admin is trusted to the level of host root. Use the audit trail to detect, not prevent. Forward audit events to an external sink for tamper-evidence beyond the local DB.
- **Compromise of operator-supplied secrets at rest** (e.g. the operator pastes `master_secret` into a chat).

## Boundary diagram

```
                   ┌───────────────────────────────────────────┐
                   │  Untrusted: public internet + LAN         │
                   └───────────────┬───────────────────────────┘
                                   │ TLS, ACME, app traffic
                                   ▼
                   ┌───────────────────────────────────────────┐
                   │  Caddy (in-process)                       │
                   │  - per-app IP allow / rate-limit          │
                   │  - HSTS, security headers                 │
                   │  - HTTP→HTTPS redirect                    │
                   └───────────────┬───────────────────────────┘
                                   │ reverse_proxy localhost:N
                                   ▼
                   ┌───────────────────────────────────────────┐
                   │  App containers                           │
                   │  - port mappings rewritten to 127.0.0.1   │
                   │  - compose validator rejects host-escape  │
                   │  - shared bridge `simpledeploy-public`    │
                   └───────────────┬───────────────────────────┘
                                   │ docker.sock (root)
                                   ▼
                   ┌───────────────────────────────────────────┐
                   │  Docker daemon                            │
                   └───────────────────────────────────────────┘

                   ┌───────────────────────────────────────────┐
                   │  Dashboard listener                       │
                   │  default 127.0.0.1:8443 (local-only)      │
                   │  - JWT cookie (HttpOnly, Secure, Strict)  │
                   │  - Bearer API key                         │
                   │  - per-IP login rate limit                │
                   │  - per-(user,IP) lockout                  │
                   └───────────────────────────────────────────┘
```

## Known design trade-offs

These are deliberate choices, surfaced for transparency:

### 1. super_admin == host root

**Why:** Deploys go through the docker.sock as root. Even with the compose validator, a super_admin who chooses a permissive image effectively executes arbitrary code on the host.

**Mitigation:** Restrict super_admin to break-glass operators; use `manage` for day-to-day. Forward audit events offsite for forensic continuity.

### 2. master_secret is the single root of trust

**Why:** Simplifies operator UX. A multi-secret model would require a key-management story (rotation, backup, restore) that adds operational risk for small deployments.

**Mitigation:** Per-purpose subkeys are derived via HKDF where compatibility allows (JWT signing). AES-GCM credential encryption and API-key HMAC continue to use the master directly so existing data stays decryptable. Document a rotation procedure if the master is ever exposed.

### 3. Recipes index is HTTPS TOFU, not signed

**Why:** Catalog publishing pipeline cost. End-to-end signing of an index plus per-recipe content requires a key-management story we have not yet committed to.

**Mitigation:** Same-host enforcement on sub-resource fetches; deploy-time compose validation catches privileged recipes; a malicious recipe still passes through the same security validator as a hand-written compose.

### 4. The audit trail is operator-trust-bound

**Why:** Audit lives in the same SQLite DB as everything else. A super_admin (or anyone with host root) can rewrite it.

**Mitigation:** Sentinel rows on `audit_purged` and on app purge. For Byzantine-fault-tolerant audit, forward events to an external sink (webhook category, syslog forwarder, etc.).

### 5. Default `tls.mode: off` is permitted

**Why:** local development and behind-LB setups need it.

**Mitigation:** Cookies are still `SameSite=Strict` and `HttpOnly` even when `Secure` is omitted. Operators are warned in the docs not to expose plain HTTP to the network.

### 6. Released artifacts are not cryptographically signed (yet)

**Why:** SLSA provenance + cosign keyless is a roadmap item, not yet shipped.

**Mitigation:** Operators can pin Docker images by digest after first pull and verify GitHub release commits against tag annotations. Tracked as a release-engineering item.

## Evidence we expect a researcher to look for

If you are auditing SimpleDeploy and this list does not match what you find, that is a finding worth reporting:

- Every mutating endpoint emits an `audit_log` row.
- Every authenticated route has either a role or an app-access middleware.
- Every `exec.Command` uses argv form (no shell interpolation).
- Every SQL query uses placeholders (the three `Sprintf`-built queries interpolate validated whitelists only).
- Every cookie is `HttpOnly` + `SameSite=Strict`.
- Every WebSocket upgrade either matches `Origin == Host` or holds a Bearer token.
- Every restore tar is pre-walked before extraction.
- Every JWT carries `iss`, `aud`, `tv`, and is HS256.

If you find a place where the codebase deviates from these invariants, please [report it](https://github.com/vazra/simpledeploy/security/advisories/new). It is much more likely to be a regression than an intentional choice.

## Versioning and changelog

Security-relevant changes are tagged with `fix(security)` or `fix(auth)` in commit messages and surfaced in the [changelog](https://github.com/vazra/simpledeploy/blob/main/CHANGELOG.md). Operator-impacting defaults (e.g. `management_addr` becoming `127.0.0.1`) are called out in release notes.
