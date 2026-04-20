---
title: Apps and services
description: The SimpleDeploy mental model: an app is a Compose project, a service is a container in that project.
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

## App status values

The Status column on the dashboard and app detail page reflects the most recent reconciler outcome:

| Status | Meaning |
| --- | --- |
| `running` | Compose succeeded and the post-deploy stabilization check confirmed every container is up (and healthy when a healthcheck is defined). |
| `unstable` | Compose succeeded but at least one container was still restarting, unhealthy, or exited at the end of the 30-second stabilization window. The app may still be partly usable. Open Logs for the failing service to see the underlying error, then redeploy after fixing. |
| `degraded` | A previously running app has lost a service since the last reconcile. |
| `error` | Compose itself failed (bad YAML, missing image, registry auth, port conflict). The deploy did not start. |
| `stopped` | App is intentionally stopped via the Stop action. |

The deploy wizard reports the same three terminal states as `success`, `unstable`, and `failed`, plus a per-service breakdown when stabilization runs. See [Troubleshooting -> Deploy reported "Unstable"](/operations/troubleshooting/#deploy-reported-unstable) for what to do when you see it.

## Healthchecks

Every template SimpleDeploy ships with includes a `healthcheck` for each service. When a service has a healthcheck, the post-deploy stabilization check waits for `healthy` (up to 30s) before reporting success; without one, it just verifies the container is running and not restart-looping. If you write your own compose file, add a healthcheck for any service that takes more than a second or two to be ready, otherwise stabilization may report `unstable` while the service is still warming up. A reasonable default for an HTTP app:

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 30s
```
