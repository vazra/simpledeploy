---
title: Desired-state reconciler
description: How the reconciler converges Docker state to match the apps directory and SQLite store.
---

import { Aside } from '@astrojs/starlight/components';

SimpleDeploy is declarative. You describe what you want (compose files in `apps_dir/`, rows in SQLite), and the reconciler keeps Docker matching. Same model as Kubernetes, smaller scope.

## Two states

**Desired.** Whatever exists on disk under `apps_dir/<slug>/docker-compose.yml`, plus DB rows for registries, backup configs, alert rules. The user controls this.

**Current.** Containers Docker is actually running, and metadata in the `apps` table (last seen status, compose hash, primary domain). The reconciler controls this.

The job: diff the two on every event, apply the smallest change that closes the gap.

## What triggers a reconcile

Two sources, both via [/internal/reconciler/watcher.go](https://github.com/vazra/simpledeploy/blob/main/internal/reconciler/watcher.go):

- An fsnotify event on `apps_dir/` (file created, modified, deleted, renamed). Debounced 1 second so a flurry of writes coalesces.
- An initial reconcile at startup. There is no periodic timer in the watcher itself; events drive it. The metrics collector separately syncs container status every interval.

## What it does

For each app on disk:

- Not in the store yet? Deploy.
- In the store but the file's SHA-256 changed since the last deploy? Redeploy.
- Otherwise: leave it alone.

For each app in the store but not on disk: tear down with `docker compose down --remove-orphans` and delete the DB row.

After all that: rebuild the route table from every app's endpoints and call `caddy.Load()` once.

## Concurrency

The reconciler runs deploys in goroutines but caps concurrency at **3 parallel deploys** via a semaphore (see `Reconcile` in [/internal/reconciler/reconciler.go](https://github.com/vazra/simpledeploy/blob/main/internal/reconciler/reconciler.go)). Image pulls are slow; running a hundred at once would saturate the box.

Teardowns run serially after deploys.

<Aside type="note">
There is no built-in error backoff. A failing deploy records a `deploy_failed` event in the `deploy_events` table and updates the app status to `error`. The next file change triggers another attempt. If you push a broken compose file in a loop, you will hammer Docker.
</Aside>

## Hash-based change detection

To avoid redeploying on every fsnotify event (editors trigger many writes), the reconciler stores a SHA-256 of the compose file in the `apps.compose_hash` column (migration 008). On reconcile, the new hash is compared. If they match, `docker compose up` is skipped entirely. This is also why direct edits to running containers are unsafe: the hash will not change so the reconciler will not re-converge.

Read the architecture page for the deploy event lifecycle, version history, and rollback flow: [Reconciler architecture](/simpledeploy/architecture/reconciler/).
