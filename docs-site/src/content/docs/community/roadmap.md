---
title: Roadmap
description: Themes and direction for SimpleDeploy. No dates.
---

SimpleDeploy follows a small, focused roadmap. The day-to-day backlog lives in [GitHub Issues](https://github.com/vazra/simpledeploy/issues) and the [GitHub Project board](https://github.com/vazra/simpledeploy/projects). The themes below describe direction, not commitments.

## Shipped

- **Shared Docker network for apps**: endpoint services auto-join `simpledeploy-public`; upstreams resolved by container IP. Ships in v\<next\>.

## Near term (next few releases)

- **More backup strategies**: ClickHouse, Elasticsearch, generic exec hooks for "run this command, capture stdout".
- **Backup target plugins**: GCS, Azure Blob, SFTP.
- **SSO**: OIDC for the dashboard so teams can drop password rotation.
- **Audit log shipping**: stream the audit log to syslog or an HTTP sink for compliance.
- **Better deploy diff**: show what's about to change before applying.
- **CLI quality of life**: tab completion improvements, structured `--output json` everywhere.

## Medium term

- **Multi-host federation**: a thin coordinator so two or more SimpleDeploy instances appear as one in the dashboard. Not full orchestration; just unified visibility and per-host targeted deploys.
- **Built-in load balancer driver**: Cloudflare and a generic "tell external LB to drain me" hook.
- **First-class secrets**: a per-app encrypted secret store that exposes values into the compose env without writing to disk in plaintext.
- **Plugin loader**: a stable interface for compiled-in extensions (custom Caddy modules, custom strategies/targets).
- **Docker install feature parity**: document one-shot container workflows (init, rotate-secret, db-backup via `docker compose run --rm`) as first-class, and add an E2E suite variant (`make e2e-docker`) so the container path stays green on every release.

## Long term

- **One-click upgrade**: dashboard-driven self-upgrade with automatic backup and rollback. Mainly a concern on the apt/tarball paths; on the Docker install, `docker compose pull && up -d` already covers this.
- **App templates marketplace**: curated compose files for common stacks.

## What we will not do

- Replace Kubernetes. SimpleDeploy is for one to a few hosts.
- Add a hosted control plane. Single-binary, self-host first.
- Lock-in to a specific cloud or registry.

Have a different priority? Vote on issues or open one. Maintainers steer by community signal.
