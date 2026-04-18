---
title: FAQ
description: "Common questions about SimpleDeploy: when to use it, how it compares to other tools, how data and crashes are handled, and hardware requirements."
---

## Is this Kubernetes?

No. SimpleDeploy is a single Go binary that runs `docker compose` for you, fronted by an embedded Caddy reverse proxy. There is no scheduler, no etcd, no overlay network. One host, one binary, your compose files.

## Why not just use docker compose directly?

You can. SimpleDeploy adds the things you build around compose anyway: TLS via Let's Encrypt, a dashboard, deploy history with rollback, scheduled backups to S3 or local disk, metrics, alert rules, user accounts with role-based access, and an API for CI.

## How is this different from Dokku, CapRover, Coolify?

- **Dokku** is git-push-to-deploy on top of buildpacks or Dockerfiles. SimpleDeploy is compose-file-first.
- **CapRover** is opinionated about app structure and uses Docker Swarm. SimpleDeploy uses plain compose, no Swarm.
- **Coolify** is a much larger system with broader scope (databases as services, multi-server, more integrations). SimpleDeploy aims smaller: one binary, one host, compose files you already understand.

Pick SimpleDeploy when you want compose with the production extras and nothing more.

## How is data backed up?

Per-app, per-strategy. The built-in strategies cover Postgres, MySQL, MongoDB, Redis, SQLite, and generic volumes. Targets are local disk or S3-compatible storage. Configure under `Backups` in the dashboard or via `simpledeploy.backup.*` labels in compose. The system DB itself is backed up nightly via `VACUUM INTO`.

## What happens if SimpleDeploy crashes?

Your apps keep running. Containers are managed by Docker, not by SimpleDeploy. The proxy stops accepting new connections only if the host process dies; on restart it picks up where it left off from the SQLite state on disk.

## Can I run multiple SimpleDeploy nodes?

Not yet. SimpleDeploy is single-host today. Multi-host is on the roadmap but not the next priority. For HA, run two hosts with shared DNS failover and replicate compose files via your config-management tool.

## Can I roll back a deploy?

Yes. Every deploy stores a snapshot of the compose file and the resolved image digests. From `Versions` in the app view, click any prior deploy and `Rollback`. The CLI: `simpledeploy rollback <app> --to <version>`.

## Is there a hosted option?

No. SimpleDeploy is self-hosted by design. Run it on a $5 VPS or on your laptop.

## Is it free?

Yes. MIT licensed. No paid tier, no telemetry, no upsell.

## What about logs? How long are they kept?

Container logs flow through Docker's default logger. Configure rotation in `/etc/docker/daemon.json` or per-service in compose. SimpleDeploy itself keeps a ring buffer of recent process logs (default 500 lines, configurable via `log_buffer_size`) for streaming to the UI. Request stats are kept in the metrics tables, rolled up after 7 days.

## How do I expose a non-HTTP service?

Set `simpledeploy.tcp.port` (or expose the port directly via the compose `ports:` field) and skip `simpledeploy.domain`. Caddy doesn't proxy raw TCP; bind the port on the host and point clients at the host directly. For TCP behind Caddy's TLS, look at Caddy's `layer4` module (advanced setup, not built-in).

## How do I run a one-off task?

Use `simpledeploy exec <app> <service> -- <command>`. It's a thin wrapper over `docker exec`. For migrations or seeds at deploy time, add a `command:` override in compose or use a sidecar service with `restart: no`.

## How big a server?

For 5 to 10 small apps, a 2 vCPU / 2 GB VPS is comfortable. SimpleDeploy itself uses around 60 MB RAM. The rest is Docker plus your apps.

## Does it work on a Raspberry Pi?

Yes. Linux/arm64 and linux/armv7 binaries ship with every release. A Pi 4 with 4 GB handles a handful of small services well.

## Where are secrets stored?

Registry credentials are AES-256-GCM encrypted with the `master_secret` from your config, stored in the SQLite DB. App env vars are stored as plain text in compose files on disk. Treat the host filesystem as your trust boundary: full-disk encryption, restricted SSH, and don't commit `master_secret` to git.
