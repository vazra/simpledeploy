---
title: Config sidecars and sidecar-based recovery
description: How SimpleDeploy mirrors every user-editable setting to YAML files on disk, and how to recover a wiped database from those files.
---

import { Aside } from '@astrojs/starlight/components';

SimpleDeploy continuously mirrors every user-editable setting to plain YAML files called **config sidecars**. If the SQLite database is ever wiped, the server automatically imports the sidecars on the next start. No backup job needed for configuration; the live copy is always on disk.

Historical data (metrics, deploy events, alert history, backup run records) is not mirrored and will be lost on a DB wipe. See [What is and isn't mirrored](#what-is-and-isnt-mirrored).

## File layout

```
{data_dir}/
  simpledeploy.db          # SQLite database
  simpledeploy.db-wal      # WAL journal (normal)
  .secret                  # master_secret (keep this safe)
  .configsync_backfill_v1  # sentinel written after first-boot backfill
  config.yml               # global sidecar (users, api keys, registries, webhooks, db backup config)

{apps_dir}/
  my-app/
    compose.yaml           # your compose file
    simpledeploy.yml       # per-app sidecar (alert rules, backup configs, access grants)
  another-app/
    compose.yaml
    simpledeploy.yml
```

Both sidecar files use YAML `version: 1` and are written with file mode `0600`.

## What is and isn't mirrored

| Data | Mirrored | Notes |
|------|----------|-------|
| Users (display name, email, bcrypt hash) | Yes | `config.yml` |
| API keys (hashed) | Yes | `config.yml` |
| Registries (encrypted credentials) | Yes | `config.yml` |
| Webhooks | Yes | `config.yml` |
| DB backup config | Yes | `config.yml` |
| Alert rules (per app) | Yes | `simpledeploy.yml` |
| Backup configs (per app, encrypted) | Yes | `simpledeploy.yml` |
| User access grants (per app) | Yes | `simpledeploy.yml` |
| Metrics / request stats | **No** | High-volume, by design |
| Deploy events | **No** | Historical, by design |
| Alert history | **No** | Historical, by design |
| Backup run records | **No** | Historical, by design |
| Compose versions | **No** | Historical, by design |

## How writes work

After every mutation (API call that changes a mirrored setting), the server schedules a sidecar write with a 500 ms debounce. Burst changes collapse into one write. The files are replaced atomically.

## How recovery works

**Global sidecar import** — on startup, if the `users` table is empty and `{data_dir}/config.yml` exists, the global sidecar is imported automatically. A log line confirms:

```
[configsync] imported global sidecar into empty DB
```

**Per-app sidecar import** — during each reconcile pass, any app whose compose file is present on disk but has no DB-side config (no alert rules, no backup configs, no access grants) has its `simpledeploy.yml` imported. Log line:

```
[configsync] imported sidecar for my-app
```

Neither import overwrites a healthy DB. If the DB already has data for a given app or user, it is left untouched.

## Recovery procedure

<Aside type="caution">
This procedure recovers configuration only. Historical data (metrics, deploy events, alert history) cannot be recovered from sidecars. For a full recovery including app volumes, follow [Disaster recovery](/operations/disaster-recovery/) alongside this procedure.
</Aside>

1. **Preserve `master_secret` and `apps_dir`.** The `master_secret` lives in `{data_dir}/.secret` (or is set via `master_secret:` in your config YAML). Without it, encrypted blobs (registry credentials, S3 backup target configs) are unreadable even though the sidecar files still exist. Users and API keys are still restored because only their bcrypt hashes are stored.

2. **Stop SimpleDeploy.**
   ```bash
   sudo systemctl stop simpledeploy
   ```

3. **Delete the corrupt or missing DB** (keep everything else).
   ```bash
   sudo rm -f /var/lib/simpledeploy/simpledeploy.db \
              /var/lib/simpledeploy/simpledeploy.db-wal \
              /var/lib/simpledeploy/simpledeploy.db-shm
   ```

4. **Start SimpleDeploy.**
   ```bash
   sudo systemctl start simpledeploy
   journalctl -u simpledeploy -f
   ```
   Watch for the import log lines. Within seconds of startup you should see the global sidecar import, then per-app imports as the reconciler runs.

5. **Log in with your existing admin credentials.** They are restored from the sidecar.

6. **Verify.** Open Settings and confirm users, registries, webhooks, and alert rules are present. Open each app and check backup configs and access grants.

## First-boot backfill

When an existing install upgrades to a version that adds sidecar support, the first `serve` start writes all sidecars from the current DB state automatically. A sentinel file `{data_dir}/.configsync_backfill_v1` is written afterward. Subsequent boots skip the backfill. No action is needed.

## CLI tools

Force a full sidecar write from the current DB (useful to verify content or backfill after a manual DB edit):

```bash
simpledeploy config export
```

Recover into a non-empty DB (truncates config tables, then imports sidecars). This is destructive and usually unnecessary because startup auto-recovery runs when the DB is empty:

```bash
simpledeploy config import --force --wipe
```

## Cautions

**Losing `master_secret` with a wiped DB** — registry credentials and S3 backup target configs are unrecoverable. Users, API keys, alert rules, and webhooks are still restored. Keep `master_secret` in a password manager separate from the host.

**Sidecar files contain sensitive data** — bcrypt password hashes, encrypted credential blobs. File mode is `0600` by default. Do not commit them to a public repo. A forthcoming Git sync feature will handle automated sync with redaction.

**Hand-editing sidecars** — supported. Edits take effect on the next startup or reconcile pass. Malformed YAML fails the import with a log line and leaves the DB unchanged.

**DB wins over sidecar** — if the same app or user already exists in the DB, the sidecar is ignored for that record. Sidecars are authoritative only when the DB is empty for that entity.
