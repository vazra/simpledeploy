---
title: Introducing SimpleDeploy
date: 2026-04-08
authors:
  - name: SimpleDeploy maintainers
    title: Project team
    picture: https://github.com/vazra.png
    url: https://github.com/vazra/simpledeploy
excerpt: A single binary that turns any VPS into a production-grade Docker Compose host. HTTPS, backups, alerts, metrics. No Kubernetes.
tags:
  - announcement
  - release
---

We have been quietly building SimpleDeploy: a single Go binary that runs Docker Compose apps on a VPS the way you actually want them to run in production.

## The problem

You write a `docker-compose.yml`, `scp` it to a VPS, run `docker compose up -d`, then start gluing on the missing pieces:

- nginx or Caddy for TLS.
- certbot or some script for renewals.
- A cron for `pg_dump` and `aws s3 cp`.
- A second cron for log rotation.
- Some monitoring (Prometheus? Netdata?) and an alert webhook.
- A dashboard for the team.
- A way to do this from your laptop.

It works, but every step is its own decision. Now repeat for every project.

## What SimpleDeploy gives you

One binary that ships all of the above as defaults you can override:

- **Docker Compose deploys** with safety: every change is versioned, every deploy is auditable, every app can roll back.
- **Automatic HTTPS** via embedded Caddy. Let's Encrypt out of the box. Custom certs supported.
- **Built-in backups** for Postgres, MySQL, MongoDB, Redis, SQLite, and raw volumes. Local or S3 targets. Retention and scheduling per app.
- **Alerts** with rules like "CPU above 80% for 5 minutes" and webhook delivery to Slack, Discord, PagerDuty.
- **Metrics and request stats** with tiered rollups so the database stays small forever.
- **Multi-user RBAC** with per-app access. API keys for CI.
- **Remote CLI** with kubectl-style contexts. Deploy from your laptop or a GitHub Actions job.
- **A dashboard** for everything above.

All of it ships in one process. About 60 MB resident for a small fleet. SQLite for state. No external dependencies beyond Docker.

## Who it is for

- Solo developers who want production-quality deployments without spending Saturdays on YAML.
- Small teams running a handful of services on one or two VPSes.
- Agencies hosting client apps cheaply.
- Anyone who looked at Kubernetes for a side project and thought "no thanks".

If you need multi-host orchestration, GPU scheduling, or an autoscaler, you want something else. SimpleDeploy is for the long tail of "this app needs to run reliably on a $20 VPS".

## What's in the 1.0 box

- **CLI + API server.** `simpledeploy serve` runs the daemon; the CLI talks to it locally or remotely with context switching (`simpledeploy context`).
- **Reconciler.** Drop a `compose.yml` in the apps directory and SimpleDeploy applies it. A directory watcher with debounce handles edits.
- **Embedded Caddy.** Programmatic config (no Caddyfile), with custom modules for per-domain rate limiting and request metrics.
- **SQLite + WAL store.** Apps, deploys, users, API keys, app access, metrics, request stats, alerts, webhooks, backups, all in one local file.
- **Auth.** Passwords (bcrypt), JWT sessions, API keys with scopes, per-app access middleware, login rate limiting.
- **Metrics.** System and container stats collector, buffered batch writer, tiered rollup and pruning, query API.
- **Request stats.** Caddy module records every request; tiered rollup powers the dashboard charts.
- **Backups.** Strategies and targets with a scheduler, configs and run history in the store, CLI commands.
- **Alerts.** Rule evaluator, webhook dispatch with built-in templates, history.
- **Svelte dashboard.** Embedded in the Go binary. Login, app list, app detail with charts and live logs, deploy/remove flows, backups page, alerts page, user management.
- **Log streaming.** Process stdout/stderr through a ring buffer, exposed live over WebSocket and the CLI.

1.0.0 also shipped two same-day patch releases (1.1.0, 1.2.0) tightening the goreleaser pipeline so Linux ARM64 and macOS binaries publish cleanly. 1.2.0 is the recommended 1.x baseline until [1.3.0](/blog/2026-04-30-v1-3-0/).

## Try it

Five minutes from zero to a HTTPS-served app:

```bash
brew install vazra/tap/simpledeploy        # or apt, or curl
simpledeploy init --config /etc/simpledeploy/config.yaml
sudo systemctl enable --now simpledeploy
simpledeploy users create --username admin --role super_admin
# drop a docker-compose.yml with simpledeploy.endpoints.0.domain label
simpledeploy apply -f docker-compose.yml --name myapp
```

The full walk-through is in the [quickstart](/start/quickstart/).

## What's next

- A growing library of [backup strategies](/guides/backups/overview/) and [target backends](/guides/backups/s3-target/).
- Multi-host federation for unified visibility across instances.
- SSO via OIDC.
- A community marketplace of compose templates.

The full direction is on the [roadmap](/community/roadmap/). The repo is at [github.com/vazra/simpledeploy](https://github.com/vazra/simpledeploy). Issues and discussions are open.

If you build something on SimpleDeploy, tell us. We will list it on the [showcase](/community/showcase/).
