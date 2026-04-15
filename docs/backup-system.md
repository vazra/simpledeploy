# Backup System

Developer guide for SimpleDeploy's backup module. Covers architecture, data flow, extension points, and known limitations.

## Architecture Overview

The backup system has three layers:

```
UI (Svelte)  -->  API (Go handlers)  -->  Scheduler (cron + orchestration)
                                              |
                                     Strategy + Target (pluggable)
                                              |
                                        Store (SQLite)
```

**Strategy** defines *what* to back up (postgres dump, volume tar). **Target** defines *where* to store it (local filesystem, S3). **Scheduler** wires them together, runs cron jobs, handles retention. The API exposes CRUD for configs/runs plus manual triggers. The UI provides a wizard for config creation and a dashboard for monitoring.

## Core Interfaces

Both are in `internal/backup/`.

```go
// Strategy defines how to back up and restore data.
type Strategy interface {
    Backup(ctx context.Context, containerName string) (io.ReadCloser, string, error)
    Restore(ctx context.Context, containerName string, data io.Reader) error
}

// Target defines where backup data is stored.
type Target interface {
    Upload(ctx context.Context, filename string, data io.Reader) (int64, error)
    Download(ctx context.Context, filename string) (io.ReadCloser, error)
    Delete(ctx context.Context, filename string) error
}
```

Strategies produce a data stream + filename. Targets move that stream to/from storage. This separation means any strategy works with any target.

## Strategies

### PostgreSQL (`internal/backup/postgres.go`)

Runs `docker exec <container> pg_dump -U postgres` and gzip-compresses the output. Restore decompresses and pipes to `docker exec -i <container> psql -U postgres`.

Filename: `{containerName}-{YYYYMMDD-HHMMSS}.sql.gz`

### Volume (`internal/backup/volume.go`)

Runs `docker exec <container> tar -czf - -C {volumePath} .` to archive a container directory. Restore extracts the tar back to the same path.

Constructor takes a `volumePath` (default `/data`). Filename: `{containerName}-{YYYYMMDD-HHMMSS}.tar.gz`

## Targets

### Local (`internal/backup/local.go`)

Writes files to `{dataDir}/backups/` on the host filesystem. Files created with mode 0600. Validates filenames against path traversal (`..`, absolute paths).

### S3 (`internal/backup/s3.go`)

Stores in any S3-compatible service (AWS, MinIO, DigitalOcean Spaces, Backblaze B2). Config:

```go
type S3Config struct {
    Endpoint  string // empty for AWS S3
    Bucket    string
    Prefix    string // optional key prefix (e.g. "backups/myapp")
    AccessKey string
    SecretKey string
    Region    string // defaults to "us-east-1"
}
```

Uses AWS SDK v2. Enables path-style addressing when a custom endpoint is set (for S3-compatible services).

## Scheduler (`internal/backup/scheduler.go`)

Orchestrates everything. Created at startup, receives registered strategies and target factories.

```go
sched := backup.NewScheduler(db)
sched.RegisterStrategy("postgres", backup.NewPostgresStrategy())
sched.RegisterStrategy("volume", backup.NewVolumeStrategy("/data"))
sched.RegisterTargetFactory("local", func(configJSON string) (backup.Target, error) { ... })
sched.RegisterTargetFactory("s3", func(configJSON string) (backup.Target, error) { ... })
sched.Start() // loads configs, schedules cron jobs
```

### Backup Flow (`RunBackup`)

1. Fetch config from DB
2. Create run record (status=`running`)
3. Look up strategy and target factory by name
4. Instantiate target from `TargetConfigJSON`
5. Get app name for container naming
6. Call `strategy.Backup()` to get data stream
7. Call `target.Upload()` to store it
8. Update run to `success` with size and file path
9. Prune old runs beyond `RetentionCount`
10. On any error: update run to `failed` with error message

### Restore Flow (`RunRestore`)

1. Fetch run and its config from DB
2. Get strategy and target
3. Call `target.Download()` to retrieve data
4. Call `strategy.Restore()` to apply it

Both backup and restore run asynchronously (fired via `go` in API handlers).

## Database Schema

### `backup_configs` (migration 007)

| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | Auto-increment |
| app_id | INTEGER | FK to apps |
| strategy | TEXT | `postgres` or `volume` |
| target | TEXT | `s3` or `local` |
| schedule_cron | TEXT | 5-field cron expression |
| target_config_json | TEXT | JSON config for target (S3 creds, etc.) |
| retention_count | INTEGER | Keep last N successful backups (default 7) |
| created_at | DATETIME | Auto-set |

### `backup_runs` (migration 007)

| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | Auto-increment |
| backup_config_id | INTEGER | FK to backup_configs |
| status | TEXT | `running`, `success`, or `failed` |
| size_bytes | INTEGER | Nullable, set on success |
| started_at | DATETIME | Auto-set |
| finished_at | DATETIME | Nullable, set on completion |
| error_msg | TEXT | Nullable, set on failure |
| file_path | TEXT | Nullable, relative path/key of backup file |

## API Endpoints

All require auth via `authMiddleware`.

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/api/apps/{slug}/backups/configs` | `handleListBackupConfigs` | List configs for app |
| POST | `/api/apps/{slug}/backups/configs` | `handleCreateBackupConfig` | Create config |
| DELETE | `/api/backups/configs/{id}` | `handleDeleteBackupConfig` | Delete config |
| GET | `/api/apps/{slug}/backups/runs` | `handleListBackupRuns` | List runs across all app configs |
| POST | `/api/apps/{slug}/backups/run` | `handleTriggerBackup` | Trigger backup (first config) |
| POST | `/api/backups/configs/{id}/run` | `handleTriggerBackupConfig` | Trigger backup for specific config |
| POST | `/api/backups/restore/{id}` | `handleRestore` | Restore from a run (202 async) |
| GET | `/api/backups/summary` | `handleBackupSummary` | Cross-app dashboard data |
| GET | `/api/apps/{slug}/backups/detect` | `handleDetectStrategies` | Auto-detect available strategies |
| POST | `/api/backups/test-s3` | `handleTestS3` | Validate S3 credentials |

### Detection Endpoint

`/api/apps/{slug}/backups/detect` parses the app's compose file and inspects services:
- Checks image names for `postgres` to detect database containers
- Collects volume mounts (excluding `/var/run/docker.sock`)
- Returns strategy availability with container/volume details

## UI Components

### `/backups` Dashboard (`ui/src/routes/Backups.svelte`)

Cross-app overview. Calls `GET /api/backups/summary` which returns:
- Per-app health: config count, strategies, last run status, storage used, 24h success/fail counts
- Recent runs across all apps with app name and strategy

Renders summary stat cards, per-app health cards (BackupHealthCard), and a filterable activity feed.

### App Detail > Backups Tab (`ui/src/components/BackupsTab.svelte`)

Per-app backup management. Three states:
1. **Empty**: no configs, shows CTA to configure
2. **Has configs**: status header + config table + run history
3. **Loading**: skeleton placeholders

Config table shows friendly labels (e.g. "Database (PostgreSQL)" not "postgres", "Daily at 02:00" not "0 2 * * *").

### Backup Wizard (`ui/src/components/BackupWizard.svelte`)

4-step FormModal:
1. **What to back up**: auto-detects strategies from compose, shows availability
2. **Where to store**: local or S3 with inline config form + connection test
3. **Schedule**: visual cron builder (ScheduleBuilder component)
4. **Retention**: count input + summary of all selections

### Schedule Builder (`ui/src/components/ScheduleBuilder.svelte`)

Visual cron builder with 4 modes: daily (time picker), weekly (day chips + time), monthly (day-of-month + time), custom (raw cron input). Shows human-readable preview and generated cron expression.

## Adding a New Strategy

1. Create `internal/backup/mystrategy.go`:
```go
type MyStrategy struct{}

func NewMyStrategy() *MyStrategy { return &MyStrategy{} }

func (s *MyStrategy) Backup(ctx context.Context, containerName string) (io.ReadCloser, string, error) {
    // exec into container, produce data stream
    filename := fmt.Sprintf("%s-%s.ext", containerName, time.Now().Format("20060102-150405"))
    return stream, filename, nil
}

func (s *MyStrategy) Restore(ctx context.Context, containerName string, data io.Reader) error {
    // exec into container, apply data
    return nil
}
```

2. Register in `cmd/simpledeploy/main.go` (`newBackupScheduler`):
```go
sched.RegisterStrategy("mystrategy", backup.NewMyStrategy())
```

3. Add to migration (ALTER or new migration) to allow the new strategy name in the CHECK constraint.

4. Update the detect endpoint in `internal/api/backups.go` (`handleDetectStrategies`) to detect containers for the new strategy.

5. Update the UI wizard step 1 labels and descriptions.

## Adding a New Target

1. Create `internal/backup/mytarget.go` implementing the `Target` interface.

2. Register factory in `cmd/simpledeploy/main.go`:
```go
sched.RegisterTargetFactory("mytarget", func(configJSON string) (backup.Target, error) {
    var cfg MyTargetConfig
    json.Unmarshal([]byte(configJSON), &cfg)
    return NewMyTarget(cfg)
})
```

3. Add to migration CHECK constraint.

4. Update wizard step 2 with UI for the new target's config fields.

## System Database Backups

Separate from app backups. Lives in `internal/store/db_backup.go` and `internal/api/system.go`. Uses SQLite `VACUUM INTO` for atomic, consistent copies. Supports a "compact" mode that strips metrics/request_stats tables to reduce size. Managed from the System page, not the Backups page.

## Known Limitations

- **No edit for configs**: must delete and recreate to change schedule/target/retention
- **Volume strategy hardcodes `/data`**: the volume path is set at startup, not per-config
- **No backup download**: app backups can't be downloaded via UI (only DB backups can)
- **No backup verification**: no checksum or integrity check after upload
- **Single postgres user**: pg_dump always uses `-U postgres`
- **Trigger backup uses first config**: the per-app trigger (`POST /apps/{slug}/backups/run`) only runs the first config; use config-specific trigger instead
- **No notification on failure**: failed backups are only visible in the UI, no webhook/alert integration

## File Index

```
internal/backup/
  strategy.go       Strategy interface
  target.go         Target interface
  scheduler.go      Scheduler (cron, RunBackup, RunRestore)
  postgres.go       PostgreSQL strategy (pg_dump/psql)
  volume.go         Volume strategy (tar)
  local.go          Local filesystem target
  s3.go             S3-compatible target

internal/store/
  backups.go         BackupConfig/BackupRun CRUD + summary queries
  db_backup.go       System DB backup operations
  migrations/
    007_backups.sql       backup_configs + backup_runs tables
    011_db_backup.sql     db_backup_config + db_backup_runs tables

internal/api/
  backups.go         All backup API handlers
  server.go          Route registration (lines 227-241)

cmd/simpledeploy/
  main.go            Scheduler init (newBackupScheduler), CLI commands

ui/src/
  routes/Backups.svelte                 Cross-app dashboard
  components/BackupsTab.svelte          Per-app backup management
  components/BackupWizard.svelte        4-step config creation wizard
  components/BackupHealthCard.svelte    Dashboard health card
  components/ScheduleBuilder.svelte     Visual cron builder
  lib/api.js                            API client methods
```
