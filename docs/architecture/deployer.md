---
title: Deployer
description: docker compose CLI wrapper, deploy events, versioning, rollback.
---

The `internal/deployer/` package wraps the `docker compose` CLI. It owns the lifecycle of every deployed app.

## CommandRunner interface

All shell-outs go through `CommandRunner.Run(ctx, name, args, opts)`. The default implementation is `os/exec`. Tests inject a `MockRunner` that records calls. This lets the package unit-test the deploy state machine without Docker present.

## Project naming

Each app's compose project is named `simpledeploy-<app-slug>`. This isolates apps from each other and from manually-managed compose stacks on the same host.

## Deploy flow

1. The caller (API handler or reconciler) has written `docker-compose.yml` to `apps_dir/<slug>/`. API handlers validate the content before writing it.
2. Re-read and validate the file (`checkComposeFile`, see below).
3. If the app references registries, write their credentials to a temporary Docker config (0600, removed afterwards) and pass it with `docker --config <tmp>`.
4. Run `docker compose up -d --remove-orphans` (missing images are pulled; output streamed to the deploy log).
5. The reconciler records a `deploy_events` row (action, output) and, on success, the compose hash and a `compose_versions` row (the 10 most recent are kept).

The whole sequence is wrapped in a tracker that the API surfaces as live deploy logs over WebSocket and as a "deploying" flag on the app.

## Compose check before `up`

`Deploy`, `RollbackDeploy`, `Pull`, `Scale` and `Cancel` (which runs a follow-up `up`) all call `checkComposeFile` before running `docker compose`. It re-reads `docker-compose.yml` from disk with `compose.ParseFile`, which interpolates `${VARS}` from the app's `.env` and the server environment the way `docker compose` does and refuses a symlinked compose file or `.env`, then runs `compose.ValidateComposeSecurity`. A file changed after the caller parsed it (git sync, version restore, manual edit) is therefore still checked right before it runs. `Restart` (`compose restart`) and `Stop`/`Start` do not create containers and skip the check.

On refusal nothing is run. Tracked operations (deploy, rollback, pull) write `Stopped before starting: ...` plus one line per violation to the deploy log (stderr stream), mark the tracker `deploy_failed` or `pull_failed`, and return a `DeployResult` with status `failed` and a `*compose.ViolationError` (or the parse error) in `Err`. `Scale` and `Cancel` return the error. The reconciler treats a refusal as "never ran": it keeps the stored compose hash and records no `compose_versions` row (see [Reconciler](/architecture/reconciler/)). API handlers validate before writing the file too and answer `400` with `{error, violations}`.

## Cancellation

Each in-flight deploy holds a context. The `cancel` API cancels the context, which kills the underlying `docker compose` process group. Cleanup of partially-pulled images is left to the next prune.

## Lifecycle commands

`stop`, `start`, `restart`, `pull`, `scale`, and `remove` all run through the same runner. `remove` also tears down the project network and (optionally) volumes.

## Rollback

Rollback re-applies a prior `compose_versions` row. The deploy flow is identical; only the YAML differs. Volumes are not touched, so stateful apps behave correctly across rollbacks (subject to the new version being able to read the on-disk state).

## Concurrency

A semaphore caps concurrent deploys (default 3). Beyond the cap, deploys queue.
