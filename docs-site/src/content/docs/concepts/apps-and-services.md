---
title: Apps and services
description: "The SimpleDeploy mental model: an app is a Compose project, a service is a container in that project."
---

import { Aside } from '@astrojs/starlight/components';

## Definitions

**App.** A Compose project managed by SimpleDeploy. Lives at `apps_dir/<slug>/docker-compose.yml`. Has exactly one slug, one directory, one project name.

**Service.** A `services:` entry inside that compose file. A service produces one or more containers (usually one, more if you scale it). A service has its own image, environment, ports, and labels.

One app, many services. The app is the unit of deployment, scheduling, backup, and access control. The service is what actually runs.

## Naming

The slug is the directory name under `apps_dir/`. It must match `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`. Path-traversal characters and leading dots are rejected. The slug is used everywhere: API paths (`/api/apps/<slug>/...`), Docker project name, log buffer keys, container name lookups, backup file naming.

The Docker Compose project name is always `simpledeploy-<slug>`. SimpleDeploy passes `-p simpledeploy-<slug>` to every `docker compose` invocation in [/internal/deployer/deployer.go](https://github.com/vazra/simpledeploy/blob/main/internal/deployer/deployer.go). This means container names look like `simpledeploy-myapp-web-1`, and the `com.docker.compose.project` label on every managed container starts with `simpledeploy-`. The metrics collector uses that prefix to filter.

<Aside type="note">
Do not run `docker compose up` from a shell with a custom `-p` flag against an apps directory. The reconciler will not find your project and will treat the app as missing.
</Aside>

## Service-level labels

All app config is in `simpledeploy.*` labels. Endpoints, backup strategy hints, rate limits, IP allowlists, and registry overrides are all label keys. The reconciler aggregates labels across all services in the app: the first occurrence of a key wins, so put app-wide settings on any service (typically the primary one).

Example slice:

```yaml
services:
  web:
    image: ghcr.io/example/web:1.4
    ports: ["8080"]
    labels:
      simpledeploy.endpoints.0.domain: "app.example.com"
      simpledeploy.endpoints.0.service: "web"
      simpledeploy.endpoints.0.port: "8080"
  redis:
    image: redis:7
```

Two services, one endpoint, one app named after the directory.

## Lifecycle

Created when you (or the API) writes the compose file into `apps_dir/<slug>/`. Updated when the file content changes (SHA-256 hash compare). Removed when the directory disappears: the next reconcile pass calls `docker compose down --remove-orphans`. Deploy history is recorded in the `compose_versions` and `deploy_events` tables (migration 009).
