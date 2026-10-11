---
title: Config sidecars and sidecar-based recovery
description: How SimpleDeploy mirrors every user-editable setting to YAML files on disk, and how to recover a wiped database from those files.
---

To sync sidecar files to a remote git repository automatically, see [Git sync](/operations/git-sync/).

SimpleDeploy continuously mirrors every user-editable setting to plain YAML files called **config sidecars**. If the SQLite database is ever wiped, the server automatically imports the sidecars on the next start. No backup job needed for configuration; the live copy is always on disk.

Historical data (metrics, deploy events, alert history, backup run records) is not mirrored and will be lost on a DB wipe. See [What is and isn't mirrored](#what-is-and-isnt-mirrored).

## File layout

```
{data_dir}/
  simpledeploy.db          # SQLite database
  simpledeploy.db-wal      # WAL journal (normal)
  .configsync_backfill_v1  # sentinel written after first-boot backfill
  config.yml               # global sidecar (users, api keys, registries, webhooks, db backup config), no secrets
  secrets.yml              # password hashes, api key hashes, encrypted registry creds, webhook URLs
  archive/<slug>.yml       # tombstones of archived apps (see Archived apps)

{apps_dir}/
  _global.yml              # redacted copy of config.yml for git sync
  my-app/
    docker-compose.yml        # your compose file
    simpledeploy.yml          # per-app sidecar (alert rules, backup configs, access grants)
    simpledeploy.secrets.yml  # encrypted backup target configs
  another-app/
    docker-compose.yml
    simpledeploy.yml
    simpledeploy.secrets.yml
```

All sidecar files use YAML `version: 1`. `config.yml` and `simpledeploy.yml` hold no secrets and are written with file mode `0644`; `secrets.yml` and `simpledeploy.secrets.yml` are written with mode `0600`. `master_secret` is not stored here: it lives in your server config file (`/etc/simpledeploy/config.yaml` by default).

## What is and isn't mirrored

| Data | Mirrored | Notes |
|------|----------|-------|
| Users (role, display name, email) | Yes | `config.yml`; bcrypt hash in `secrets.yml` |
| API keys (name, expiry) | Yes | `config.yml`; key hash in `secrets.yml` |
| Registries (name, URL) | Yes | `config.yml`; encrypted credentials in `secrets.yml` |
| Webhooks (name, type) | Yes | `config.yml`; URL, headers, template in `secrets.yml` |
| DB backup config | Yes | `config.yml`; encrypted target config in `secrets.yml` |
| Alert rules (per app) | Yes | `simpledeploy.yml` |
| Backup configs (per app) | Yes | `simpledeploy.yml`; encrypted target config in `simpledeploy.secrets.yml` |
| User access grants (per app) | Yes | `simpledeploy.yml` |
| Metrics / request stats | **No** | High-volume, by design |
| Deploy events | **No** | Historical, by design |
| Alert history | **No** | Historical, by design |
| Backup run records | **No** | Historical, by design |
| Compose versions | **No** | Historical, by design |

## How writes work

After every mutation (API call that changes a mirrored setting), the server schedules a sidecar write with a 500 ms debounce. Burst changes collapse into one write. The files are replaced atomically.

## How recovery works

**Global sidecar import**: on startup, if the `users` table is empty and `{data_dir}/config.yml` exists, the global sidecar (with `secrets.yml`) is imported automatically. A log line confirms:

```
[configsync] imported global sidecar into empty DB
```

If `config.yml` cannot be imported (for example malformed YAML after a hand edit), the server does not replace it with the empty DB's contents: startup stops with an error naming the file, so you can fix it and start again.

**Per-app sidecar import**: the reconciler re-adds every app folder that has a `docker-compose.yml` but no DB row, then imports that app's `simpledeploy.yml` (and `simpledeploy.secrets.yml`) if the app has no alert rules, backup configs or access grants yet. Log line:

```
[configsync] imported app sidecar for my-app
```

Until then, the boot-time reload skips the app with `[fs-auth] my-app: no app row yet; its sidecar is imported once the reconciler adds the app`.

**Archived apps** come back from their tombstones in `{data_dir}/archive/`, still archived.

These two imports never overwrite existing rows: they run only for an empty `users` table, or for an app with no alert rules, backup configs or access grants yet.

**Orphan cleanup never removes live config.** On startup, a `simpledeploy.yml` is deleted only from a folder that is no longer an app: no `docker-compose.yml`, no DB row (active or archived) and no tombstone. Nothing is deleted while the DB has no apps at all, so a wiped DB never costs you app sidecars. Removals are logged as `[configsync] pruned orphan sidecars for: [...]`.

With [git sync](/operations/git-sync/) enabled, access grants are managed in the dashboard only: the `access` list in a pulled or hand-edited `simpledeploy.yml` is never applied, and the server rewrites a pulled file's `access` list back to the dashboard's grants. Disaster-recovery imports into an empty DB from sidecars already on disk still restore grants. Sidecar and data-dir files must be regular files; symlinks are refused.

## Recovery procedure

<Aside type="caution">
This procedure recovers configuration only. Historical data (metrics, deploy events, alert history) cannot be recovered from sidecars. For a full recovery including app volumes, follow [Disaster recovery](/operations/disaster-recovery/) alongside this procedure.
</Aside>

1. **Preserve `master_secret`, `{data_dir}` and `apps_dir`.** `master_secret` is set via `master_secret:` in your server config file (`/etc/simpledeploy/config.yaml` by default). Without it, encrypted blobs (registry credentials, S3 backup target configs) are unreadable and API keys stop working, even though the sidecar files still exist. Users can still log in because their passwords are stored as bcrypt hashes.

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
   Watch for the import log lines. Within seconds of startup you should see the global sidecar import, then per-app imports as the reconciler re-adds each app (it runs `docker compose up` for each one; unchanged containers keep running).

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

**Losing `master_secret` with a wiped DB**: registry credentials and S3 backup target configs are unrecoverable, and API keys must be re-created (their hashes are keyed with `master_secret`). Users, alert rules, webhooks and access grants are still restored. Keep `master_secret` in a password manager separate from the host.

**Secrets files contain sensitive data**: `secrets.yml` and `simpledeploy.secrets.yml` hold bcrypt password hashes, API key hashes, encrypted credential blobs and webhook URLs, with file mode `0600`. Do not commit them to a public repo. [Git sync](/operations/git-sync/) commits the app folders (its `.gitignore` excludes `*.secrets.yml`) and the redacted `_global.yml`, never anything in `{data_dir}`.

**Hand-editing sidecars**: supported. The file watcher applies edits within about a second; otherwise they apply on the next start. A malformed `simpledeploy.yml` is skipped with a log line and leaves the DB unchanged; a malformed `config.yml` stops the server from starting until it is fixed.

**Files win over the DB**: on every start, and whenever a sidecar changes, the server applies the files to the DB (see [State on disk](/operations/state-on-disk/)). For an app the DB does not have yet, its sidecar is applied once the reconciler re-adds the app.
