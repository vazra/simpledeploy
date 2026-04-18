---
title: Backups overview
description: "Concepts behind SimpleDeploy backups: strategies, targets, schedules, retention, and where to configure each."
---

SimpleDeploy backups have three pieces:

- **Strategy** what to back up: a database dump (postgres, mysql, mongo, redis, sqlite) or a volume tarball.
- **Target** where to store it: local filesystem or any S3-compatible bucket.
- **Schedule + retention** when it runs (cron) and how many copies to keep.

Configure backups via the UI Backup wizard (recommended) or the REST API.

## Quick example

### Local backups

No extra config needed. Backups stored at `{data_dir}/backups/`.

### S3 backups

Configure via the API or UI. The S3 target config:

```json
{
  "endpoint": "s3.amazonaws.com",
  "bucket": "my-backups",
  "prefix": "simpledeploy/",
  "access_key": "AKIA...",
  "secret_key": "...",
  "region": "us-east-1"
}
```

Works with AWS S3, MinIO, Cloudflare R2, and any S3-compatible storage.

## Per-strategy guides

- [PostgreSQL](/guides/backups/postgres/)
- [MySQL / MariaDB](/guides/backups/mysql/)
- [MongoDB](/guides/backups/mongo/)
- [Redis](/guides/backups/redis/)
- [SQLite](/guides/backups/sqlite/)
- [Volume snapshots](/guides/backups/volume/)

## Targets

- [S3 target](/guides/backups/s3-target/)
- [Local target](/guides/backups/local-target/)

## Operations

- [Retention and scheduling](/guides/backups/retention/)
- [Restore](/guides/backups/restore/)
- [System database backup](/guides/backups/system-db-backup/)

## Architecture

For a developer-oriented walkthrough of strategies and targets, see [Backup architecture](/architecture/backup/).
