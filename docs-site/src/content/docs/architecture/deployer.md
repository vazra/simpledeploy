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

1. Persist the new compose YAML as a `compose_versions` row (sha256-keyed; identical content does not insert a new version).
2. Write `docker-compose.yml` to `apps_dir/<slug>/`.
3. Build a Docker config file with credentials for any registries the app references; injected via `DOCKER_CONFIG` env var so credentials never touch disk in plaintext.
4. Run `docker compose pull` (output streamed to a `logbuf.Buffer`).
5. Run `docker compose up -d --remove-orphans`.
6. Record the result as a `deploy_events` row (action, status, output tail).

The whole sequence is wrapped in a tracker that the API surfaces as live deploy logs over WebSocket and as a "deploying" flag on the app.

## Cancellation

Each in-flight deploy holds a context. The `cancel` API cancels the context, which kills the underlying `docker compose` process group. Cleanup of partially-pulled images is left to the next prune.

## Lifecycle commands

`stop`, `start`, `restart`, `pull`, `scale`, and `remove` all run through the same runner. `remove` also tears down the project network and (optionally) volumes.

## Rollback

Rollback re-applies a prior `compose_versions` row. The deploy flow is identical; only the YAML differs. Volumes are not touched, so stateful apps behave correctly across rollbacks (subject to the new version being able to read the on-disk state).

## Concurrency

A semaphore caps concurrent deploys (default 3). Beyond the cap, deploys queue.
