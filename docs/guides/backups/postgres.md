---
title: PostgreSQL backups
description: Logical backups of PostgreSQL with pg_dump, gzip-compressed, working out of the box with the official postgres image.
---

The `postgres` strategy runs:

```
docker exec <container> sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
```

and gzip-compresses the output. The user and database are read from the container's env at dump time, so the strategy works with the stock `postgres` image (which would otherwise dump the empty default `postgres` database). Override via `opts.Credentials["POSTGRES_USER"]`. Restore pipes the gzipped dump into `psql` using the same user/db resolution.

Filename format: `{containerName}-{YYYYMMDD-HHMMSS}.sql.gz`

## Configure via compose labels

```yaml
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: myapp
      POSTGRES_PASSWORD: secret
    labels:
      simpledeploy.backup.strategy: "postgres"
      simpledeploy.backup.schedule: "0 2 * * *"
      simpledeploy.backup.target: "local"
      simpledeploy.backup.retention: "7"
```

Or configure via the UI Backup wizard.

See also: [Backups overview](/guides/backups/overview/), [Restore](/guides/backups/restore/), [Backup architecture](/architecture/backup/).
