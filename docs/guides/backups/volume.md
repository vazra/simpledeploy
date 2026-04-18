---
title: Volume snapshots
description: Tar and gzip configured paths inside a container. Good for files and a fallback for databases when no DB-specific strategy applies.
---

The `volume` strategy runs:

```
docker exec <container> tar -czf - <paths...>
```

against the paths configured on the backup config. Tar strips leading `/` so the archive contents are relative (e.g. `var/lib/postgresql/data/...`). Restore extracts with `tar -xzf - -C /`, which recreates the absolute paths.

Filename format: `{containerName}-{YYYYMMDD-HHMMSS}.tar.gz`

## Caveat for running databases

Backing up a live postgres data directory produces a crash-consistent snapshot, but a volume *restore* over the same directory is racy because pg keeps files open. For any DB-backed volume restore, either stop the service via `pre_hooks: [stop]` + `post_hooks: [start]`, or use the dedicated DB strategy (postgres/mysql/mongo/redis) that speaks the protocol.

## Configure via compose labels

```yaml
services:
  app:
    image: myapp:latest
    volumes:
      - uploads:/var/uploads
    labels:
      simpledeploy.backup.strategy: "volume"
      simpledeploy.backup.schedule: "0 3 * * *"
      simpledeploy.backup.target: "s3"
      simpledeploy.backup.retention: "30"
```

Then in the backup config, set `paths: ["/var/uploads"]`.

See also: [Backups overview](/guides/backups/overview/), [Restore](/guides/backups/restore/).
