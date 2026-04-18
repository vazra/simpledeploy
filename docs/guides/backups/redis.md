---
title: Redis backups
description: Snapshot Redis via BGSAVE and stream the resulting RDB file out of the container, gzipped.
---

The `redis` strategy captures the pre-BGSAVE `LASTSAVE` timestamp, triggers `BGSAVE`, polls until the timestamp changes, then `docker cp`s `/data/dump.rdb` out and gzips it. Restore stops the container, `docker cp`s the decompressed RDB back into `/data/`, and restarts.

Filename format: `{containerName}-{YYYYMMDD-HHMMSS}.rdb.gz`.

## Configure via compose labels

```yaml
services:
  cache:
    image: redis:7
    labels:
      simpledeploy.backup.strategy: "redis"
      simpledeploy.backup.schedule: "0 */6 * * *"
      simpledeploy.backup.target: "local"
      simpledeploy.backup.retention: "12"
```

See also: [Backups overview](/guides/backups/overview/), [Restore](/guides/backups/restore/).
