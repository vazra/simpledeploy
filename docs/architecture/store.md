---
title: Storage layer
description: SQLite with WAL, embedded migrations, VACUUM-based snapshots.
---

The `internal/store/` package owns all database access. There is one database for everything (apps, users, metrics, alerts, audit, etc.) so a single backup captures full state.

## Engine

SQLite with WAL mode. `SetMaxOpenConns(4)` allows concurrent readers while a single writer serializes through the mode lock. Foreign keys are enabled. Busy timeout is set so transient lock contention retries automatically.

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

Migrations are forward-only. Adding a column with a default is safe; renames and drops are forbidden in published migrations to keep rollback to a previous binary version possible.

## Backup of the DB itself

The system DB backup uses SQLite's `VACUUM INTO`, which writes a consistent snapshot to a target file without blocking writers. "Compact" mode strips `metrics` and `request_stats` rows before vacuuming so the snapshot is small enough to ship off-site cheaply. See [the system DB backup guide](/guides/backups/system-db-backup/).

## Connection ownership

The store exposes typed methods (e.g., `UpsertApp`, `ListAlertRules`) rather than raw SQL outside the package. This keeps query patterns in one file per resource and makes the API package easy to test.

## File layout

`{data_dir}/simpledeploy.db` plus its WAL and SHM siblings. Permissions are 0600.
