---
title: Storage layer
description: SQLite with WAL, embedded migrations, VACUUM-based snapshots.
---

The `internal/store/` package owns all database access. There is one database for everything (apps, users, metrics, alerts, audit, etc.) so a single backup captures full state.

## Engine

SQLite with WAL mode. `SetMaxOpenConns(4)` allows concurrent readers while a single writer serializes through the mode lock.

### Connection settings

SQLite keeps some settings per connection and others in the database file. The two kinds are applied differently:

| Setting | Scope | How it is applied |
| --- | --- | --- |
| `busy_timeout=5000` | connection | DSN `_pragma=` parameter |
| `foreign_keys=1` | connection | DSN `_pragma=` parameter |
| `cache_size=2000` (pages) | connection | DSN `_pragma=` parameter |
| `journal_mode=WAL` | database (persistent) | once in `Open` |
| `auto_vacuum=INCREMENTAL` | database (persistent) | once in `Open`, new databases only |

Connection-scoped settings must reach every pooled connection, including ones opened later when `SetConnMaxLifetime` recycles a connection. A plain `db.Exec("PRAGMA ...")` only reaches whichever connection happened to run it, so these live in the DSN (`connPragmas` in `store.go`); modernc runs each `_pragma` on every connection it opens. `synchronous` is not set and stays at the SQLite default (`FULL`).

Foreign keys are enforced on every connection: `ON DELETE CASCADE` / `SET NULL` run, and inserts referencing a missing parent fail. Before this was fixed, enforcement was only on one pooled connection, so older databases may hold orphan rows. SQLite does not re-check existing rows, so they do not block startup or updates; only new writes are checked. Write paths that copy foreign key values out of existing rows or out of an in-memory buffer (metric and request metric inserts and rollups) skip rows whose app no longer exists, so a stale point cannot fail a whole batch.

### auto_vacuum

`auto_vacuum` can only be chosen before page 1 is written, which happens on the first table and also on the switch to WAL. `Open` therefore sets `auto_vacuum=INCREMENTAL` before `journal_mode=WAL`, and only when the file is empty (`page_count = 0`). The daily `IncrementalVacuum` then returns free pages to the filesystem.

Databases created before this fix have `auto_vacuum=NONE`, and `IncrementalVacuum` is a no-op on them. Converting needs a full `VACUUM`, which rewrites the whole file, needs free disk space roughly equal to the database size, and blocks writers while it runs, so `Open` never does it implicitly. The explicit vacuum (`POST /api/system/vacuum`, `Store.VacuumDB`) sets `auto_vacuum=INCREMENTAL` on its connection before `VACUUM`, so the first manual vacuum converts an old database at no extra cost.

## Migrations

SQL files live in `internal/store/migrations/` and are embedded with `go:embed`. They run in numeric order on startup.

| # | Topic |
| --- | --- |
| 001 | apps |
| 002 | app_labels |
| 003 | users, api_keys, user_app_access |
| 004 | metrics |
| 005 | request_stats |
| 006 | webhooks, alert_rules, alert_history |
| 007 | backup_configs, backup_runs |
| 008 | compose_hash |
| 009 | compose_versions, deploy_events |
| 010 | registries |
| 011 | db_backup_config, db_backup_runs |
| 012 | user profile (display_name, email) |
| 013 | metrics v2 |
| 014 | alert history rule snapshot columns |
| 015 | backups v2 |
| 016 | indexes (alert_history, backup_runs) |
| 017 | alert_history.rule_id nullable for gitsync conflict alerts |
| 018 | gitsync_config (DB-authoritative git sync configuration) |
| 019 | apps_status_unstable (transient status tracking) |
| 020 | audit_log (unified activity feed) + audit_config (retention) |
| 021 | recipe_pulls (community recipe popularity) |
| 022 | apps.archived_at (archive instead of delete) |
| 023 | system_meta (key/value markers) |
| 024 | backup_configs uuid |
| 025 | role `admin` renamed to `manage` |
| 026 | users.token_version (session invalidation) |
| 027 | api_keys.last_used_at |
| 028 | alert_history.rule_id `ON DELETE SET NULL` (history kept when a rule is deleted) |

Migrations are forward-only. Adding a column with a default is safe; renames and drops are forbidden in published migrations to keep rollback to a previous binary version possible.

All pending migrations run on one dedicated connection with `PRAGMA foreign_keys=OFF` (set outside the per-migration transaction, since the pragma is a no-op inside one), and the connection gets `foreign_keys=ON` back before it returns to the pool. This follows SQLite's documented procedure for table rebuilds (create new table, copy, `DROP` old, rename), which is the only way to change a `CHECK` or column type. With foreign keys on, `DROP TABLE` runs an implicit `DELETE` that fires `ON DELETE CASCADE` on every child table, so rebuilding `apps` (019) or `users` (025) would wipe labels, grants, API keys, versions, backups and metrics. A side effect: a migration that deletes parent rows does not cascade, so it must delete the children itself.

## Backup of the DB itself

The system DB backup uses SQLite's `VACUUM INTO`, which writes a consistent snapshot to a target file without blocking writers. "Compact" mode strips `metrics` and `request_stats` rows before vacuuming so the snapshot is small enough to ship off-site cheaply. See [the system DB backup guide](/guides/backups/system-db-backup/).

## Connection ownership

The store exposes typed methods (e.g., `UpsertApp`, `ListAlertRules`) rather than raw SQL outside the package. This keeps query patterns in one file per resource and makes the API package easy to test.

## File layout

`{data_dir}/simpledeploy.db` plus its WAL and SHM siblings. Permissions are 0600.
