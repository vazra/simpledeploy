---
title: State and storage
description: What SimpleDeploy stores, where it lives on disk, and how to back it up.
---

import { Aside } from '@astrojs/starlight/components';

SimpleDeploy keeps state in three places: a SQLite database, an apps directory, and Caddy's own storage for ACME certs. Backups go anywhere you point them.

## Layout

```
data_dir/                       (default /var/lib/simpledeploy)
  simpledeploy.db               SQLite, WAL mode, 0600
  simpledeploy.db-wal           WAL journal
  simpledeploy.db-shm           shared memory
  caddy/                        Caddy data + ACME cert storage
  backups/                      local backup target output
  audit-log.jsonl               (if file audit sink configured)

apps_dir/                       (default /etc/simpledeploy/apps)
  myapp/
    docker-compose.yml          desired state for myapp
    .env                        optional
    certs/                      custom TLS certs (if tls.mode=custom)
      app.example.com.crt
      app.example.com.key
  otherapp/
    docker-compose.yml
```

Everything in `apps_dir/` is owned by you, the operator. Edit files freely; the reconciler picks them up. Everything in `data_dir/` is owned by SimpleDeploy; do not edit by hand while the process is running.

## What SQLite holds

One file, all tables. WAL mode lets concurrent reads (API, dashboard) proceed against a single writer (reconciler, metrics writer, audit log). See [/internal/store/](https://github.com/vazra/simpledeploy/tree/main/internal/store) and the 17 migrations under `migrations/`. High-level groups:

- App metadata: `apps`, `app_labels`, `compose_hash`, `compose_versions`, `deploy_events`
- Auth: `users`, `api_keys`, `user_app_access`
- Observability: `metrics` (5 tier rollup), `request_stats` (per-route), `alert_rules`, `alert_history`, `webhooks`
- Backups: `backup_configs`, `backup_runs` (for app data) and `db_backup_config`, `db_backup_runs` (for the SimpleDeploy DB itself)
- Registries: `registries` (encrypted credentials)

## What does not live in SQLite

- Compose files: on disk, source of truth.
- TLS certs: managed by Caddy under `data_dir/caddy/`.
- Container state: in Docker.
- Process logs: in-memory ring buffer ([/internal/logbuf/](https://github.com/vazra/simpledeploy/tree/main/internal/logbuf)), lost on restart unless you redirect stderr.

## Backing up the database

The DB is itself a backup target via the System page. SimpleDeploy uses SQLite `VACUUM INTO` for atomic, WAL-safe snapshots without stopping writes. A "compact" mode strips the metrics and request_stats tables before download, since those dominate size. Schedules are stored in `db_backup_config` (migration 011) and managed alongside app backups.

<Aside type="caution">
Copying the raw `simpledeploy.db` file while the process is running is unsafe. WAL pages may not yet be checkpointed. Use the System backup endpoint (which calls `VACUUM INTO`) or stop the process first.
</Aside>

For backing up your apps' data (postgres volumes, redis dumps, app files), see [Backups](/simpledeploy/concepts/backups/) and [Backup architecture](/simpledeploy/architecture/backup/).
