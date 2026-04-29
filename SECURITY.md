# Security Policy

SimpleDeploy is a self-hosted application platform. We take security seriously and welcome reports from researchers, downstream operators, and the broader community.

## Reporting a vulnerability

**Preferred channel:** GitHub's [private vulnerability reporting](https://github.com/vazra/simpledeploy/security/advisories/new) for this repo. This keeps the report between you and the maintainers until a fix ships.

**Email:** `security@vazra.us` (PGP key on request) if you cannot use GitHub.

Please do **not** open public GitHub issues, pull requests, or discussions for unfixed vulnerabilities.

A useful report includes:

- A description of the issue and the affected component (file path, function, or endpoint).
- The version (`simpledeploy version`) and deployment shape (binary, Docker, distro package).
- Steps to reproduce. A minimal proof-of-concept is appreciated but not required.
- Your assessment of impact and severity.
- Any required preconditions (e.g. authenticated user, specific config).

We will:

1. Acknowledge receipt within **3 business days**.
2. Confirm reproduction and triage within **7 business days**.
3. Aim to ship a fix or mitigation within **30 days** for High/Critical and **90 days** for Medium/Low. Extensions are coordinated with you when upstream changes are required.
4. Credit you in the release notes and CVE record unless you ask to remain anonymous.

## Safe harbor

We will not pursue legal action or report you to law enforcement for security research that:

- Is conducted against your own deployment of SimpleDeploy (or one you have explicit permission to test).
- Stops once a vulnerability is identified — no data exfiltration, lateral movement, or denial-of-service.
- Avoids accessing, modifying, or destroying data that is not yours.
- Discloses to us privately first via the channels above.

## Supported versions

Only the latest **minor release** of the `main` branch receives security fixes. Older minors are not patched. Operators are expected to upgrade within a reasonable window after a security release.

## Scope

**In scope:**

- The `simpledeploy` binary (REST API, dashboard, CLI, reconciler, embedded Caddy modules).
- The Svelte UI shipped in this repository.
- Build/release artifacts produced by this repository.

**Out of scope (report to upstream):**

- The Docker daemon and its supply chain.
- Caddy core and CertMagic (`caddyserver/caddy`, `caddyserver/certmagic`).
- The Go standard library and `golang-jwt/jwt`.
- SQLite (`modernc.org/sqlite`).
- Linux kernel, systemd, distro packaging.
- Compose files, Docker images, and recipes authored outside this repo.

If unsure, send the report and we will route it.

## Auditing SimpleDeploy

For researchers and downstream auditors, see:

- [Security architecture](docs/operations/security-architecture.md) — design overview, cryptographic primitives, trust boundaries, and what's mitigated.
- [Threat model](docs/operations/threat-model.md) — trust assumptions, in/out of scope, and known design trade-offs.
- [Security hardening](docs/operations/security-hardening.md) — operator-facing controls.
- [Activity & Audit Log](docs/operations/security-audit.md) — what's recorded for forensic purposes.
