---
title: Restore
description: Restore an app from a previous backup run via the UI or POST /api/backups/restore/{id}. Strategy-specific behavior.
---

Restore from the UI Backups page, or via the API:

```
POST /api/backups/restore/{run_id}
```

Returns 202 Accepted and runs asynchronously.

## Strategy-specific notes

- **postgres** pipes the gzipped dump into `psql` using the same user/db resolution as the backup.
- **mysql** pipes the gzipped SQL into `mysql -u root -p$MYSQL_ROOT_PASSWORD`.
- **mongo** uses `mongorestore --drop` so it overwrites existing collections.
- **redis** stops the container, copies the decompressed RDB into `/data/`, and restarts.
- **sqlite** runs `sqlite3 .restore` against the configured path.
- **volume** extracts the archive with `tar -xzf - -C /`. Not safe over a running database; use a DB-native strategy or stop the service first.

For **volume**, **sqlite** and **redis**, the archive is checked (no absolute paths, `..`, links or device files; size limit) before anything is extracted.

Backups are staged in a private temp file under `{data_dir}/tmp` (owner-only) while their checksum and contents are checked, and the file is deleted when the restore finishes. This holds for the server and for the `simpledeploy restore` CLI command. Make sure the data disk has free space for the backup file.

## Size limits

- Restoring a backup that SimpleDeploy made itself (from the backup history, the API above, or `simpledeploy restore`) has no size limit by default.
- A backup file uploaded with **Restore from File** may be up to 32 MiB and may expand to at most **8 GiB** once decompressed.
- To choose your own limit, set `SIMPLEDEPLOY_RESTORE_MAX_GB` to a whole number of GiB (for example `50`) where SimpleDeploy runs. It then applies to both kinds of restore. For CLI restores, set it in the shell too. See [Environment variables](/reference/env-vars/).

A restore over the limit fails with "backup archive exceeds the restore size limit". For volume, sqlite and redis this happens before anything is extracted. postgres and mysql restores stop when the limit is reached, so part of the dump may already have been applied; restore again with a higher limit.

The decompressed size limit does not apply to **mongo** restores: `mongorestore` reads the gzipped archive itself, so SimpleDeploy passes it through without decompressing it. The 32 MiB limit on uploaded files still applies.

## Concurrent restores

At most 4 restores run at once, counting uploaded files and restores from the backup history together. Further requests get `429 Too Many Requests` with "too many restores in progress, try again later". Wait for a running restore to finish and try again.

See also: [Backups overview](/guides/backups/overview/), [Disaster recovery](/operations/disaster-recovery/).
