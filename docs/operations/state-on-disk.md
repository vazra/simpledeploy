---
title: State on disk
description: How SimpleDeploy stores configuration on the filesystem, which files are authoritative, where secrets live, and how to recover from a corrupted database.
---

import { Aside } from '@astrojs/starlight/components';

SimpleDeploy treats the filesystem as the source of truth for all portable configuration. The SQLite database is a cache that can be rebuilt from disk at any time. This page describes the on-disk layout, the authority boundary, the secrets boundary, and recovery procedures.

## File layout

Per app, under `<apps_dir>/<slug>/`:

```
<apps_dir>/<slug>/
  docker-compose.yml          # mode 0644, git-tracked
  simpledeploy.yml            # mode 0644, git-tracked, declarative non-secret
  simpledeploy.secrets.yml    # mode 0600, gitignored, encrypted blobs
```

Global, under `{data_dir}/`:

```
{data_dir}/
  config.yml                  # mode 0644, git-tracked
  secrets.yml                 # mode 0600, gitignored
  archive/<slug>.yml          # tombstones (see archived-apps.md)
  simpledeploy.db             # DB cache (regenerable from FS)
```

## What's authoritative

| Resource | Authority | Where |
|---|---|---|
| compose | FS | `<apps_dir>/<slug>/docker-compose.yml` |
| app meta (display name) | FS | `simpledeploy.yml` |
| alert rules | FS | `simpledeploy.yml` |
| backup configs (non-secret) | FS | `simpledeploy.yml` |
| backup configs (encrypted target) | FS | `simpledeploy.secrets.yml` |
| per-app access | FS | `simpledeploy.yml` |
| users (non-secret) | FS | `config.yml` |
| users (password hash) | FS | `secrets.yml` |
| api keys (key hash) | FS | `secrets.yml` |
| registries (id, name, url) | FS | `config.yml` |
| registries (encrypted creds) | FS | `secrets.yml` |
| webhooks (name, type) | FS | `config.yml` |
| webhooks (url, headers, template) | FS | `secrets.yml` |
| db backup (schedule, target type) | FS | `config.yml` |
| db backup (encrypted target config) | FS | `secrets.yml` |
| metrics, request stats, audit log, deploy events, backup runs, alert history, compose versions, gitsync state | DB | (runtime state, not portable) |
| `master_secret`, bootstrap config | `/etc/simpledeploy/config.yaml` | unchanged |

## Secrets boundary

- `*.secrets.yml` files are mode 0600 and gitignored.
- `auth.Encrypt`-encrypted fields: registry credentials, backup target configs.
- Hashed (irreversible): `password_hash`, `api_key.key_hash`.
- Plaintext but file-perm-protected: webhook URLs, headers, template overrides.
- `master_secret` (in bootstrap config or `SIMPLEDEPLOY_MASTER_SECRET` env) is required to decrypt the encrypted fields.

Threat model: filesystem read access on the host is equivalent to DB read access. Don't commit secrets files; don't ship them in backups intended for git.

## Editing files by hand

The reconciler watcher detects edits and reapplies after a short debounce (~1s).

Recommended workflow: edit, save, watch logs for `[fs-auth] apply <slug>` to confirm.

Permissions: keep secrets files at 0600. The seed step enforces this on first boot. Editors that rewrite-then-rename atomically preserve perms; some don't. Verify with `ls -l`.

Bad edit recovery: `git checkout` the offending file (if tracked) or restore from backup. The watcher reapplies; the DB returns to a consistent state on the next reload.

## GitOps with gitsync

Commit `simpledeploy.yml`, `config.yml`, `docker-compose.yml`. Never `*.secrets.yml`. The seed step injects appropriate `.gitignore` entries on first boot.

Pulling new content updates files on disk; the watcher re-applies changes to the DB cache.

## Eventual-consistency contract

SimpleDeploy uses path B (eventual consistency) for FS writes:

- API mutation responds 200 after the DB commits.
- The store mutation hook then schedules a debounced FS write.
- If the FS write fails, the watcher reapplies on the next file edit. In normal operation, FS reflects the change within a few seconds of the response.

Trade-off: a process crash between DB commit and FS write means the next boot's `ReconcileDBFromFS` will reapply the FS state. If the writer's debounce hadn't fired yet, FS is stale and the DB row will be reverted to FS state at next reload.

Mitigation: the debounce is short; production crashes mid-debounce are rare; most edits are made via API anyway. If atomicity is required, future work can promote the writer to synchronous.

## Recovery from a corrupted DB

1. Stop the server.
2. Move `{data_dir}/simpledeploy.db` aside (rename, don't delete until you've verified recovery).
3. Restart. The first-boot seed marker (`system_meta`) is in the DB, so the server runs the FS-to-DB reconcile via `ReconcileDBFromFS`. All FS-authoritative state rebuilds.

Caveat: runtime state (metrics, audit log, deploy history, archived-app rows that have no tombstone) is lost.

## Troubleshooting

- Logs prefixed `[fs-auth]` describe seed, reload, and apply operations.
- Force a reload: `touch <apps_dir>/<slug>/simpledeploy.yml`. The watcher fires and reapplies.
- File permissions: secrets files must be 0600 to satisfy the writer. If you change perms by hand, the next write resets them.

## Caveat: macOS Docker Desktop dev mode

The `make dev-docker` flow bind-mounts `${HOME}/.simpledeploy-local-dev` into the container. fsnotify on bind mounts is reliable for writes originating inside the container. Writes from the host that originate outside the bind-mount path may be missed by fsnotify.

If you edit sidecar files from the host while the dev container is running, restart the dev container after a batch of edits, or trigger a reload by touching the file inside the container.
