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
- Custom endpoints on private, loopback or link-local addresses (for example MinIO on the same server, LAN or Tailscale: `127.0.0.1`, `10.x`, `192.168.x`, `100.64.x`-`100.127.x`) are refused by default so a backup config cannot be used to reach internal services. To allow them, start SimpleDeploy with `SIMPLEDEPLOY_ALLOW_PRIVATE_S3=1` (and set it in the shell for CLI `simpledeploy backup run` / `simpledeploy restore`). Public S3 services (AWS, R2, B2, Spaces) need nothing extra. When S3 is reached through `HTTP_PROXY`/`HTTPS_PROXY`, the proxy makes the connection, so SimpleDeploy checks the endpoint's address each time it sets up the target instead.
- Credentials are stored encrypted with AES-256-GCM using `master_secret` (PBKDF2 key derivation).
- Use the "Test S3" button in the UI wizard, or `POST /api/backups/test-s3`, to validate credentials before saving.

See also: [Backups overview](/guides/backups/overview/), [Local target](/guides/backups/local-target/).
