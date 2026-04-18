---
title: S3 target
description: Store backups in any S3-compatible bucket (AWS, MinIO, R2, Backblaze B2, DigitalOcean Spaces) with streamed uploads.
---

The S3 target stores backups in any S3-compatible service (AWS, MinIO, DigitalOcean Spaces, Backblaze B2, Cloudflare R2).

## Config

```json
{
  "endpoint": "s3.amazonaws.com",
  "bucket": "my-backups",
  "prefix": "simpledeploy/myapp",
  "access_key": "AKIA...",
  "secret_key": "...",
  "region": "us-east-1"
}
```

| Field | Notes |
|-------|-------|
| `endpoint` | Empty for AWS S3. Set for MinIO/R2/B2/Spaces. |
| `bucket` | Bucket name (must already exist) |
| `prefix` | Optional key prefix |
| `access_key` / `secret_key` | Credentials (encrypted at rest with master_secret) |
| `region` | Defaults to `us-east-1` |

## Notes

- Uses AWS SDK v2 with the `feature/s3/manager` Uploader for streamed `PutObject`. The manager handles non-seekable readers from `pg_dump`/`tar` stdout.
- Path-style addressing is enabled when a custom `endpoint` is set so MinIO, DigitalOcean Spaces, and Backblaze B2 all work.
- Credentials are stored encrypted with AES-256-GCM using `master_secret` (PBKDF2 key derivation).
- Use the "Test S3" button in the UI wizard, or `POST /api/backups/test-s3`, to validate credentials before saving.

See also: [Backups overview](/guides/backups/overview/), [Local target](/guides/backups/local-target/).
