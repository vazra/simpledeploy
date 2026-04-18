---
title: Writing a backup target
description: Add a new backup Target for a different storage backend.
---

A "target" knows how to read, write, list, and delete blobs in a storage backend (S3, local disk, GCS, Azure Blob, ...). To add one, implement the `Target` interface in `internal/backup/` and register it.

## Interface

```go
type Target interface {
    Name() string                                                      // unique key, e.g. "s3"
    Put(ctx context.Context, key string, body io.Reader) (size int64, err error)
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    List(ctx context.Context, prefix string) ([]Object, error)
    Delete(ctx context.Context, key string) error
    TestConnection(ctx context.Context) error                          // used by the UI test button
}
```

`Object` is `{Key, Size, ModTime}`. Keys are forward-slash paths; targets must accept any printable ASCII.

## Reference targets

- **Local** (`internal/backup/target_local.go`): writes to `{data_dir}/backups/local/`. Useful for development and one-host setups.
- **S3** (`internal/backup/target_s3.go`): uses the AWS SDK. Supports custom endpoints (R2, Wasabi, MinIO) via `endpoint`. Credentials are stored encrypted in `registries`-style rows.

## Steps to add a target

1. Create `internal/backup/target_<name>.go` implementing `Target`.
2. Add credential storage if needed. Reuse the encrypted-credential pattern from S3 (encrypt with `master_secret`).
3. Register the target in the factory.
4. Add unit tests with a fake remote (testcontainers, MinIO, or custom interface mock).
5. Add a guide page under `docs/guides/backups/<name>-target.md`.
6. Update `docs/architecture/backup.md` to mention the new target.

## Constraints

- **Streaming**: never buffer a backup body in memory. `Put` should accept `io.Reader` and stream.
- **Idempotency**: `Put` with the same key must overwrite cleanly; `Delete` must be a no-op when the key is gone.
- **Cancellation**: honor `ctx` for all network operations.
- **Credentials**: encrypt at rest with `auth.Encrypt`. Decrypt only at use time.
- **Errors**: classify retryable vs permanent; the scheduler retries only retryable ones.

## Submit

Open a PR with the target, tests, and docs. Tag it `feat(backup): <name> target`.
