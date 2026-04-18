---
title: Local target
description: Store backups on the host filesystem under {data_dir}/backups/. Owner-only file permissions and path-traversal validation.
---

The local target writes backup files to `{data_dir}/backups/` on the host filesystem. Files are created with mode `0600` and the directory uses `0700`. Filenames are validated against path traversal (`..`, absolute paths).

## When to use

- Single-server deploys where off-host backup is handled separately (e.g., disk snapshots)
- Development and testing
- Air-gapped environments without S3-compatible storage

## When not to use

If you only have local backups and the server fails, you lose your data. For production, use [S3 target](/guides/backups/s3-target/) or replicate the backups directory off-server.

See also: [Backups overview](/guides/backups/overview/).
