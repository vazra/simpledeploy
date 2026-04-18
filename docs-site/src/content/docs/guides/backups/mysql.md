---
title: MySQL / MariaDB backups
description: Logical backups via mysqldump with --all-databases, working out of the box with the official mysql and mariadb images.
---

The `mysql` strategy runs:

```
docker exec <c> sh -c 'mysqldump --all-databases -u root -p"$MYSQL_ROOT_PASSWORD"'
```

The root password is read from the container env at dump time (same idea as postgres), so the stock `mysql:8` / `mariadb` images work out of the box. Restore pipes the gzipped SQL into `mysql -u root -p$MYSQL_ROOT_PASSWORD`. Override via `opts.Credentials["MYSQL_ROOT_PASSWORD"]`.

Filename format: `{containerName}-{YYYYMMDD-HHMMSS}.sql.gz`.

## Configure via compose labels

```yaml
services:
  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: secret
    labels:
      simpledeploy.backup.strategy: "mysql"
      simpledeploy.backup.schedule: "0 2 * * *"
      simpledeploy.backup.target: "local"
      simpledeploy.backup.retention: "7"
```

See also: [Backups overview](/guides/backups/overview/), [Restore](/guides/backups/restore/).
