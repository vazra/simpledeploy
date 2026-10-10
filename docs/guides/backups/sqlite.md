---
title: SQLite backups
description: Use sqlite3 .backup to take a consistent snapshot of a SQLite database file from inside its container.
---

The `sqlite` strategy is detected via the `simpledeploy.backup.strategy=sqlite` label. `Backup()` runs:

```
docker exec <c> sqlite3 <path> .backup /tmp/...
```

using the explicit path from the backup config (`paths: ["/data/app.db"]`). Auto-detect returns the mounted volume directory but not the DB filename, so configs must specify the concrete `.db` file path.

The path must be absolute and written in plain form (no `//`, `.` or `..` parts). It names the database file, so it must not end with `/` (`/data/app.db`, not `/data/app.db/`). Spaces and characters such as `@` are fine; quotes (`'`, `"`), backslashes, backticks, `$`, control characters and path parts starting with `-` are refused.

Filename format: `{containerName}-{YYYYMMDD-HHMMSS}.db.gz`.

## Configure via compose labels

```yaml
services:
  app:
    image: myapp:latest
    volumes:
      - data:/data
    labels:
      simpledeploy.backup.strategy: "sqlite"
      simpledeploy.backup.schedule: "0 2 * * *"
      simpledeploy.backup.target: "local"
      simpledeploy.backup.retention: "7"
```

Then in the backup config, set `paths: ["/data/app.db"]`.

See also: [Backups overview](/guides/backups/overview/), [Restore](/guides/backups/restore/).
