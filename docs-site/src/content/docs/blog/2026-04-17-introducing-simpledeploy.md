---
title: Introducing SimpleDeploy
date: 2026-04-17
authors:
  - name: SimpleDeploy maintainers
    title: Project team
    picture: https://github.com/vazra.png
    url: https://github.com/vazra/simpledeploy
excerpt: A single binary that turns any VPS into a production-grade Docker Compose host. HTTPS, backups, alerts, metrics. No Kubernetes.
tags:
  - announcement
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
