---
title: MongoDB backups
description: Logical backups via mongodump with --archive --gzip, authenticated against the admin database with credentials from container env.
---

The `mongo` strategy runs:

```
docker exec <c> sh -c 'mongodump --archive --gzip --authenticationDatabase admin -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD"'
```

Credentials are read from the container env. Restore uses `mongorestore --drop` so it overwrites existing collections. Override via `opts.Credentials["MONGO_INITDB_ROOT_USERNAME"]` / `["MONGO_INITDB_ROOT_PASSWORD"]`.

Filename format: `{containerName}-{YYYYMMDD-HHMMSS}.archive.gz`.

## Configure via compose labels

```yaml
services:
  db:
    image: mongo:7
    environment:
      MONGO_INITDB_ROOT_USERNAME: root
      MONGO_INITDB_ROOT_PASSWORD: secret
    labels:
      simpledeploy.backup.strategy: "mongo"
      simpledeploy.backup.schedule: "0 2 * * *"
      simpledeploy.backup.target: "local"
      simpledeploy.backup.retention: "7"
```

See also: [Backups overview](/guides/backups/overview/), [Restore](/guides/backups/restore/).
