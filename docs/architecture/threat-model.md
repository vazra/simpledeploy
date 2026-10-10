---
title: Threat model
description: Trust boundaries, assets, attacker classes, and how SimpleDeploy mitigates them.
---

SimpleDeploy targets a single host operated by a small team. The threat model below describes who we trust, what we protect, and the mitigations in place. See also the full [security audit](/operations/security-audit/) and [hardening guide](/operations/security-hardening/).

## Trust boundaries

1. **Operator** with shell on the host. Fully trusted. Owns `master_secret`, the database, and the binary.
2. **Dashboard user** with role `super_admin`, `manage`, or `viewer`. Authenticated via password + JWT or API key. Trusted within their role and per-app scope.
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
Mitigated by: compose validation runs on the file as docker compose sees it (`${VARS}` filled from the app's `.env`) before every `compose up` (deploy, import, rollback, version restore, pull, scale) and on reconciler scans. It rejects privileged mode (including privileged lifecycle hooks), `use_api_socket`, `provider` services, host or cross-container namespaces and host networks, dangerous capabilities (any case), security options that disable AppArmor/seccomp/SELinux, devices and device cgroup rules, bind mounts of system folders (and their parents), SimpleDeploy's data folder or other apps' folders (symlinks followed), a writable bind of the app folder itself or binds nested inside another writable bind, host files pulled in via `env_file`/`label_file`/`extends`/`secrets`/`configs`/build contexts, and volume driver options that mount host disks. Compose files are versioned so suspicious changes are reviewable.

**Compromised git sync remote.**
Mitigated by: pulled compose goes through the same validator; `_global.yml` is never applied on pull (users, roles, registries, DB backup settings are dashboard-only); pulled access-grant edits are reverted; the repo is checked out with `core.symlinks=false` and symlinked managed files block the apply; remote URL schemes and branch names are validated and git transport helpers are disabled.

**Malicious page on a sibling subdomain.**
Mitigated by: Go's `http.CrossOriginProtection` on the API rejects state-changing requests that a browser marks as cross-origin (`Sec-Fetch-Site`, or `Origin` vs `Host`). `SameSite=Strict` alone does not help because sibling subdomains are same-site. WebSocket upgrades require `Origin` to match the dashboard host and port.

**Malicious or compromised app container.**
Mitigated by: standard Docker isolation. SimpleDeploy does not grant special access to apps; the API is on a separate port and requires auth. Limitation: services with endpoints join the shared `simpledeploy-public` network so Caddy can reach them, which also lets those containers reach each other directly, bypassing per-app `access.allow` and rate limits. Do not rely on those labels to isolate apps from each other; require app-level auth for internal services.

**Webhook SSRF.**
Mitigated by: outbound webhook resolver refuses private, link-local, and loopback addresses by default. Opt-in via `SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS=1`.

**Cross-tenant access via the dashboard.**
Mitigated by: per-app access rows on every protected endpoint. Tested via integration tests. Also: endpoint domains are owned by one app (the dashboard `domain` is reserved), custom certs can only be uploaded for an app's own endpoints, upload-restore only targets the app's own containers, version rollback/delete check the version belongs to the app, and S3 backup endpoints on private/loopback/link-local addresses are refused unless `SIMPLEDEPLOY_ALLOW_PRIVATE_S3=1`.

**Theft of database file.**
Mitigated by: credentials encrypted at rest with AES-256-GCM. The attacker still needs `master_secret` (plain text on the host) to decrypt.

## Out of scope

- Multi-host orchestration security (run separate SimpleDeploy instances per trust zone).
- Compromise of the host kernel.
- Side-channel attacks on TLS.
