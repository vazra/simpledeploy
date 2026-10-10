---
title: Reconciler
description: Tick loop, hash-based change detection, concurrency cap, and deploy event lifecycle.
---

The reconciler is the only component allowed to mutate Docker state. Everything else writes a file or a row and lets the loop converge. Source: [/internal/reconciler/reconciler.go](https://github.com/vazra/simpledeploy/blob/main/internal/reconciler/reconciler.go) and [/internal/reconciler/watcher.go](https://github.com/vazra/simpledeploy/blob/main/internal/reconciler/watcher.go).

## Trigger model

The watcher (`Watch`) registers an fsnotify watch on `apps_dir` (non-recursive: subdirectory creates trigger only the dir-level event, the per-file events fire on the create-then-edit pattern most editors use). Events are coalesced into a single `Reconcile()` call via a 1-second debounce timer. The first `Reconcile()` runs unconditionally at startup.

There is no internal periodic tick. If you want a heartbeat sweep, restart the process (cheap) or trigger any file event manually. The metrics collector independently re-syncs the `apps.status` column every collection interval, so drift in that column is corrected even without a reconcile.

## Diff loop

`Reconcile()` does this in order:

1. `scanAppsDir()` walks `apps_dir`, parses and validates every `docker-compose.yml`, and returns a `scanResult`: `desired` (apps that pass) and `refused` (apps on disk that fail compose security rules, have a symlinked compose file, or reference files outside the app folder). Hidden directories (leading `.`) and directories without a compose file are skipped silently.
2. `store.ListApps()` returns currently-known apps from SQLite.
3. For each desired app: if not in the store, deploy. If in the store but the file's SHA-256 differs from `apps.compose_hash`, redeploy.
4. For each store app neither desired nor refused, whose compose file is gone: archive it (teardown, tombstone, row marked archived; see [Archived apps](/operations/archived-apps/)).
5. Recompute the route table from desired apps plus route-only refused apps (below), tag every route with its app's store ID (`Route.AppID`, `math.MaxInt64` for apps not in the store yet), and call `proxy.SetRoutes()`. The proxy uses `AppID` to give a domain claimed by several apps to the one created first (see [Proxy](/architecture/proxy/)).

Steps 3 are run as goroutines under a `chan struct{}` semaphore of capacity **3**, so at most 3 deploys execute concurrently. Step 4 runs serially after the wait.

## Hash detection

Migration 008 added `apps.compose_hash` as a `TEXT` column. After every successful deploy, `deployApp()` writes the SHA-256 of the compose file. On the next reconcile, file hash is recomputed and compared. Match -> skip. Mismatch -> deploy. Failure to read -> log and skip. Hashing reads through `fsutil.ReadRegularFile`, so a symlinked file never hashes.

When the deployer refuses a file (`*compose.ViolationError` or `compose.ErrDotEnv`), `deployApp()` and `RollbackOne()` keep the previously stored hash and record no `compose_versions` row: a refused file never ran, so it must not look like the last deployed one.

This keeps reconciles cheap. fsnotify fires a lot during git pulls, file syncs, and editor saves. Without the hash check the system would call `docker compose up` repeatedly and produce noisy `deploy_events`.

## Refused apps and route-only retention

Refused apps are never deployed, and a refused app that is still on disk is never archived. `withRouteOnlyApps()` keeps routing a refused app only when it is in the store, not archived, and its compose file still has the same SHA-256 as `apps.compose_hash` (the last successful deploy). An app deployed before the rules tightened therefore stays reachable while its file is unchanged, and logs `keeping routes for the running app; fix its compose file before the next deploy`. Any edit to the file drops it from routing until it passes validation. Redeploys, pulls and scaling are refused by the deployer's own check (see [Deployer](/architecture/deployer/#compose-check-before-up)).

For routing, refused apps are parsed as leniently as possible: a symlinked `docker-compose.yml` is read through the link (and its hash compared the way older versions computed it), a file-reference refusal (`include`, `label_file` or `extends` outside the app folder) uses `compose.ParseForRoutes()`, which strips those references and ignores `.env`. These parses are only ever used for routes. An unusable `.env` (symlink or syntax error) does not refuse the app by itself: the scan parses the compose file without `.env` and validates it as usual, since the deployer re-parses strictly before every `up` and refuses to start anything from it.

`New()` registers the folders the validator protects: `compose.SetProtectedPaths(data_dir, apps_dir)` (no app may bind-mount SimpleDeploy's data or another app's folder) and `compose.SetAllowedHostPaths(allowed_bind_paths)` (operator-approved folders under protected system paths).

## Deploy event lifecycle

Every deploy attempt writes one row to `deploy_events` (migration 009). The action column distinguishes outcomes:

| Action | When |
|--------|------|
| `deploy` | initial successful deploy or successful redeploy |
| `deploy_failed` | `docker compose up` returned nonzero, or the deployer refused the compose file (output lists the violations) |
| `restart` / `restart_failed` | manual restart via API/UI |
| `pull` / `pull_failed` | manual image pull (pull then up) |
| `rollback` | rollback to a saved compose version |

The output column captures combined stdout+stderr from the docker invocation, truncated only by the deploy log buffer ring.

After every deploy (success or fail), `apps.status` is updated to `running` or `error`. The metrics collector later refines this to `degraded` if some services are down or `error` if zero containers are up.

## Compose versions

Migration 009 also added `compose_versions`. After every successful deploy, the current compose file content is inserted with the same hash; the 10 most recent per app are kept. The UI's "Versions" tab lists these, and `RollbackOne(slug, versionID)` writes the saved content back to disk and triggers a redeploy. The version must belong to the app (`ErrVersionNotFound` otherwise), and it is checked with the app's current `.env` against today's rules before it replaces the file, so a refused rollback leaves the current file in place.

## Registry credential resolution

If `app.registries` is set (via the `simpledeploy.registries` label), or default `registries:` is configured globally, `resolveRegistries()` reads the `registries` table, decrypts username/password with the master secret (AES-256-GCM, see [/internal/auth/crypto.go](https://github.com/vazra/simpledeploy/blob/main/internal/auth/crypto.go)), and passes them to the deployer as `RegistryAuth` records. The deployer writes a temporary `~/.docker/config.json` and uses `docker --config <tmp>` so credentials never touch the host's real Docker config.

<Aside type="caution">
There is no error backoff. A failing deploy will be retried on the next file event. If you push a broken compose file in a git-sync loop, you will spam Docker. The fix is upstream: validate before commit, or pause the file source.
</Aside>

## Cancel and scale

Long-running deploys (slow image pulls, big builds) can be cancelled. `CancelOne()` invokes `Tracker.Cancel(slug)` which calls the per-deploy `cancel()` saved in the tracker; the deployer's exec-context aborts. After cancel, a follow-up `docker compose up -d` is run to leave the project in a consistent state (whatever was already pulled gets started or skipped).

Scaling does not redeploy. `ScaleOne()` uses `docker compose up -d --no-recreate --scale svc=N`, which only adds or removes container replicas.
