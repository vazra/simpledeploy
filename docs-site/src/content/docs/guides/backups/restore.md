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

See also: [Backups overview](/guides/backups/overview/), [Disaster recovery](/operations/disaster-recovery/).
