---
title: Writing a backup strategy
description: "Add a new backup Strategy implementation: interface, registration, detection, tests, docs."
---

A "strategy" knows how to dump and restore one kind of data store (Postgres, MySQL, Redis, raw volumes, ...). To add one, implement the `Strategy` interface in `internal/backup/` and register it.

## Interface

```go
type Strategy interface {
    Name() string                                       // unique key, e.g. "postgres"
    Detect(svc compose.Service) Confidence              // 0..1: should we suggest this?
    Backup(ctx context.Context, run *Run) error         // produce an artifact
    Restore(ctx context.Context, run *Run, src io.Reader) error
}
```

`Run` carries the app slug, target writer, working dir, and labels. The strategy is responsible for streaming the dump into `run.Out` and reporting bytes written.

## Reference: Postgres

`internal/backup/postgres.go`:

1. **Detect**: returns `0.95` if the service image starts with `postgres:` or `bitnami/postgresql`; `0` otherwise.
2. **Backup**: shells `pg_dump --format=custom --compress=6` inside the container via `docker exec`, captures stdout into `run.Out`. Reads `POSTGRES_USER`/`POSTGRES_DB` from the service env.
3. **Restore**: `pg_restore --clean --if-exists`.

Use it as the canonical template.

## Steps to add a new strategy

1. Create `internal/backup/<name>.go` implementing `Strategy`.
2. Register the strategy in the factory (search for where Postgres is registered; add yours next to it).
3. Add a unit test that exercises `Detect` against a few compose services.
4. Add an integration test that runs `Backup` and `Restore` against a real container fixture (use the same pattern as `internal/backup/postgres_test.go`).
5. Add a row to the strategies table in [Compose labels](/reference/compose-labels/) and a guide page under `docs/guides/backups/<name>.md`.
6. Mention the new strategy in `docs/architecture/backup.md`.

## Constraints

- Network: only reach the target via the project's docker network. Do not expose ports.
- Credentials: read from compose env vars or `*_FILE` secrets; never log them.
- Output: stream into `run.Out`. Do not buffer entire dumps in memory.
- Idempotency: a failed backup must be safe to retry.
- Cancellation: honor `ctx`; long-running dumps must exit when cancelled.

## Submit

Open a PR with the strategy, tests, and docs. Tag it `feat(backup): <name> strategy`.
