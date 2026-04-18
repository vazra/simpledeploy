---
title: Write a docker-compose.yml
description: Walk-through of writing a Compose file with simpledeploy.* labels for routing, TLS, and multi-service apps. Uses whoami as the example.
---

import { Aside, Steps, FileTree } from '@astrojs/starlight/components';

A SimpleDeploy app is a regular `docker-compose.yml` with a few extra labels under the `simpledeploy.*` namespace. Existing compose files work as-is once you add labels for routing.

## Minimal example

The classic test image, [`traefik/whoami`](https://hub.docker.com/r/traefik/whoami), prints the request it received. Perfect for verifying the proxy.

```yaml
# /etc/simpledeploy/apps/whoami/docker-compose.yml
services:
  web:
    image: traefik/whoami
    ports:
      - "80"
    labels:
      simpledeploy.domain: "whoami.example.com"
      simpledeploy.port: "80"
      simpledeploy.tls: "auto"
    restart: unless-stopped
```

Three labels do the work:

| Label | What it does |
|---|---|
| `simpledeploy.domain` | Hostname Caddy routes to this container. |
| `simpledeploy.port` | Container port to forward to. Defaults to first mapped port. |
| `simpledeploy.tls` | `auto` (Let's Encrypt), `custom`, or `off`. Defaults to `auto`. |

Drop the file in `/etc/simpledeploy/apps/whoami/`, the reconciler picks it up within a few seconds.

## Multi-service app

Apps with a database, a worker, and a web tier all live in one compose file. Place routing labels on the public service. Place backup labels on the database service.

```yaml
# /etc/simpledeploy/apps/myapp/docker-compose.yml
services:
  web:
    image: ghcr.io/me/myapp:latest
    ports:
      - "3000:3000"
    environment:
      DATABASE_URL: postgres://app:secret@db:5432/myapp
    labels:
      simpledeploy.domain: "myapp.example.com"
      simpledeploy.port: "3000"
    depends_on:
      - db
    restart: unless-stopped

  db:
    image: postgres:16
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: app
      POSTGRES_PASSWORD: secret
    volumes:
      - pgdata:/var/lib/postgresql/data
    labels:
      simpledeploy.backup.strategy: "postgres"
      simpledeploy.backup.schedule: "0 2 * * *"
      simpledeploy.backup.target: "local"
      simpledeploy.backup.retention: "7"
    restart: unless-stopped

volumes:
  pgdata:
```

This deploys the app at `https://myapp.example.com`, with daily Postgres backups at 2 AM, keeping 7.

## Directory layout

Each app lives in its own subdirectory. The directory name becomes the app slug (used in URLs, logs, CLI).

<FileTree>
- /etc/simpledeploy/apps
  - whoami
    - docker-compose.yml
  - myapp
    - docker-compose.yml
    - .env
  - api-service
    - docker-compose.yml
</FileTree>

<Aside type="tip">
Try the [Compose playground](/playground/) to draft and validate label combinations before saving the file.
</Aside>

## Common labels

A short reference. See [Compose labels](/reference/compose-labels/) for the full list.

| Label | Example | Notes |
|---|---|---|
| `simpledeploy.domain` | `app.example.com` | Required for proxy routing. |
| `simpledeploy.port` | `3000` | Container port. |
| `simpledeploy.tls` | `auto` | `auto`, `custom`, `off`. |
| `simpledeploy.backup.strategy` | `postgres` | Auto-detected from image where possible. |
| `simpledeploy.backup.schedule` | `0 2 * * *` | Cron, 5-field. |
| `simpledeploy.alerts.cpu` | `>80,5m` | Trigger above 80% for 5 min. |
| `simpledeploy.ratelimit.requests` | `100` | Override global rate limit. |
| `simpledeploy.access.allow` | `10.0.0.0/8` | IP allowlist. |

Next: deploy it [via the UI](/first-deploy/ui/) or [via the CLI](/first-deploy/cli/).
