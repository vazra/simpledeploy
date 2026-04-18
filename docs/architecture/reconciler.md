---
title: Reconciler
description: Tick loop, hash-based change detection, concurrency cap, and deploy event lifecycle.
---

import { Aside } from '@astrojs/starlight/components';

The reconciler is the only component allowed to mutate Docker state. Everything else writes a file or a row and lets the loop converge. Source: [/internal/reconciler/reconciler.go](https://github.com/vazra/simpledeploy/blob/main/internal/reconciler/reconciler.go) and [/internal/reconciler/watcher.go](https://github.com/vazra/simpledeploy/blob/main/internal/reconciler/watcher.go).

## Trigger model

The watcher (`Watch`) registers an fsnotify watch on `apps_dir` (non-recursive: subdirectory creates trigger only the dir-level event, the per-file events fire on the create-then-edit pattern most editors use). Events are coalesced into a single `Reconcile()` call via a 1-second debounce timer. The first `Reconcile()` runs unconditionally at startup.

There is no internal periodic tick. If you want a heartbeat sweep, restart the process (cheap) or trigger any file event manually. The metrics collector independently re-syncs the `apps.status` column every collection interval, so drift in that column is corrected even without a reconcile.

## Diff loop

`Reconcile()` does this in order:

1. `scanAppsDir()` walks `apps_dir`, parses every `docker-compose.yml`, returns a map keyed by slug. Hidden directories (leading `.`) and directories without a compose file are skipped silently.
2. `store.ListApps()` returns currently-known apps from SQLite.
3. For each desired app: if not in the store, deploy. If in the store but the file's SHA-256 differs from `apps.compose_hash`, redeploy.
4. For each store app not in the desired set: tear down and delete the row.
5. Recompute the route table from all desired apps and call `proxy.SetRoutes()`.

Steps 3 are run as goroutines under a `chan struct{}` semaphore of capacity **3**, so at most 3 deploys execute concurrently. Step 4 runs serially after the wait.

## Hash detection

Migration 008 added `apps.compose_hash` as a `TEXT` column. After every successful deploy, `deployApp()` writes the SHA-256 of the compose file. On the next reconcile, file hash is recomputed and compared. Match -> skip. Mismatch -> deploy. Failure to read -> log and treat as not-yet-known (will deploy).

This keeps reconciles cheap. fsnotify fires a lot during git pulls, file syncs, and editor saves. Without the hash check the system would call `docker compose up` repeatedly and produce noisy `deploy_events`.

## Deploy event lifecycle

Every deploy attempt writes one row to `deploy_events` (migration 009). The action column distinguishes outcomes:

| Action | When |
|--------|------|
| `deploy` | initial successful deploy or successful redeploy |
| `deploy_failed` | `docker compose up` returned nonzero |
| `restart` / `restart_failed` | manual restart via API/UI |
| `pull` / `pull_failed` | manual image pull (pull then up) |
| `rollback` | rollback to a saved compose version |

The output column captures combined stdout+stderr from the docker invocation, truncated only by the deploy log buffer ring.

After every deploy (success or fail), `apps.status` is updated to `running` or `error`. The metrics collector later refines this to `degraded` if some services are down or `error` if zero containers are up.

## Compose versions

Migration 009 also added `compose_versions`. After every successful deploy, the current compose file content is inserted with the same hash. The UI's "Versions" tab lists these, and `RollbackOne(slug, versionID)` writes the saved content back to disk and triggers a redeploy. Versions are not pruned automatically.

## Registry credential resolution

If `app.registries` is set (via the `simpledeploy.registries` label), or default `registries:` is configured globally, `resolveRegistries()` reads the `registries` table, decrypts username/password with the master secret (AES-256-GCM, see [/internal/auth/crypto.go](https://github.com/vazra/simpledeploy/blob/main/internal/auth/crypto.go)), and passes them to the deployer as `RegistryAuth` records. The deployer writes a temporary `~/.docker/config.json` and uses `docker --config <tmp>` so credentials never touch the host's real Docker config.

<Aside type="caution">
There is no error backoff. A failing deploy will be retried on the next file event. If you push a broken compose file in a git-sync loop, you will spam Docker. The fix is upstream: validate before commit, or pause the file source.
</Aside>

## Cancel and scale

Long-running deploys (slow image pulls, big builds) can be cancelled. `CancelOne()` invokes `Tracker.Cancel(slug)` which calls the per-deploy `cancel()` saved in the tracker; the deployer's exec-context aborts. After cancel, a follow-up `docker compose up -d` is run to leave the project in a consistent state (whatever was already pulled gets started or skipped).

Scaling does not redeploy. `ScaleOne()` uses `docker compose up -d --no-recreate --scale svc=N`, which only adds or removes container replicas.
