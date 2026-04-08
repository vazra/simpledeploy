# Phase 8: Backup System - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scheduled backups for Postgres (pg_dump) and generic volumes (tar+gzip). Upload to S3-compatible storage or local directory. Retention management. Restore via CLI/API.

**Architecture:** A scheduler goroutine reads backup_configs from SQLite and runs backups on cron schedules. Strategy interface for backup/restore (Postgres, Volume). Target interface for upload/download (S3, Local). Streaming pipeline: strategy output -> gzip -> target upload. No temp files for large backups.

**Tech Stack:** Docker exec (pg_dump/pg_restore), AWS SDK v2 (S3), archive/tar + compress/gzip, robfig/cron/v3

---

## File Structure

```
internal/backup/strategy.go          - Strategy interface + registry
internal/backup/postgres.go          - PostgresStrategy (pg_dump via docker exec)
internal/backup/postgres_test.go
internal/backup/volume.go            - VolumeStrategy (tar+gzip volume)
internal/backup/volume_test.go
internal/backup/target.go            - Target interface
internal/backup/s3.go                - S3Target
internal/backup/s3_test.go
internal/backup/local.go             - LocalTarget
internal/backup/local_test.go
internal/backup/scheduler.go         - Cron scheduler
internal/backup/scheduler_test.go

internal/store/backups.go            - backup_configs, backup_runs CRUD
internal/store/backups_test.go
internal/store/migrations/007_backups.sql

internal/api/backups.go              - Backup management endpoints
internal/api/backups_test.go

cmd/simpledeploy/main.go             - Wire scheduler, add CLI commands
```

---

### Task 1: Backup Store

**Files:**
- Create: `internal/store/migrations/007_backups.sql`
- Create: `internal/store/backups.go`
- Create: `internal/store/backups_test.go`

#### Migration:
```sql
CREATE TABLE IF NOT EXISTS backup_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    strategy TEXT NOT NULL CHECK(strategy IN ('postgres', 'volume')),
    target TEXT NOT NULL CHECK(target IN ('s3', 'local')),
    schedule_cron TEXT NOT NULL,
    target_config_json TEXT NOT NULL DEFAULT '{}',
    retention_count INTEGER NOT NULL DEFAULT 7,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS backup_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_config_id INTEGER NOT NULL REFERENCES backup_configs(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running', 'success', 'failed')),
    size_bytes INTEGER,
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME,
    error_msg TEXT,
    file_path TEXT
);

CREATE INDEX IF NOT EXISTS idx_backup_configs_app ON backup_configs(app_id);
CREATE INDEX IF NOT EXISTS idx_backup_runs_config ON backup_runs(backup_config_id);
```

#### Types:
```go
type BackupConfig struct {
    ID              int64
    AppID           int64
    Strategy        string
    Target          string
    ScheduleCron    string
    TargetConfigJSON string
    RetentionCount  int
    CreatedAt       time.Time
}

type BackupRun struct {
    ID             int64
    BackupConfigID int64
    Status         string
    SizeBytes      *int64
    StartedAt      time.Time
    FinishedAt     *time.Time
    ErrorMsg       string
    FilePath       string
}
```

#### Methods:
- `CreateBackupConfig(cfg *BackupConfig) error`
- `ListBackupConfigs(appID *int64) ([]BackupConfig, error)`
- `GetBackupConfig(id int64) (*BackupConfig, error)`
- `DeleteBackupConfig(id int64) error`
- `CreateBackupRun(configID int64) (*BackupRun, error)`
- `UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath string) error`
- `UpdateBackupRunFailed(id int64, errMsg string) error`
- `ListBackupRuns(configID int64) ([]BackupRun, error)`
- `GetBackupRun(id int64) (*BackupRun, error)`
- `ListOldBackupRuns(configID int64, keepCount int) ([]BackupRun, error)` - returns runs beyond retention count

#### Tests:
- TestBackupConfigCRUD
- TestBackupRunLifecycle (create -> success/fail)
- TestListOldBackupRuns

- [ ] Commit: `git commit -m "add backup store: configs and runs"`

---

### Task 2: Backup Strategies + Targets

**Files:**
- Create: `internal/backup/strategy.go`
- Create: `internal/backup/postgres.go` + `postgres_test.go`
- Create: `internal/backup/volume.go` + `volume_test.go`
- Create: `internal/backup/target.go`
- Create: `internal/backup/local.go` + `local_test.go`
- Create: `internal/backup/s3.go` + `s3_test.go`

#### Strategy interface:
```go
type Strategy interface {
    Backup(ctx context.Context, containerName string) (io.ReadCloser, string, error)
    // Returns: data stream, suggested filename, error
    Restore(ctx context.Context, containerName string, data io.Reader) error
}
```

#### Target interface:
```go
type Target interface {
    Upload(ctx context.Context, filename string, data io.Reader) (int64, error)
    // Returns: bytes written, error
    Download(ctx context.Context, filename string) (io.ReadCloser, error)
    Delete(ctx context.Context, filename string) error
}
```

#### PostgresStrategy:
- Backup: `docker exec {container} pg_dump -U postgres | gzip`
- Uses Docker exec API to run pg_dump inside the container
- Returns gzipped stream + filename like `{appslug}-{timestamp}.sql.gz`
- Restore: `gunzip | docker exec -i {container} psql -U postgres`
- Needs docker.Client interface for exec

#### VolumeStrategy:
- Backup: tar+gzip the volume mount path
- Uses Docker exec to tar the volume inside container
- Returns gzipped tar stream + filename like `{appslug}-{timestamp}.tar.gz`
- Restore: stops container, extracts tar, restarts

#### LocalTarget:
- Upload: write file to `{basePath}/{filename}`
- Download: open file
- Delete: os.Remove
- Config: `{"path": "/var/lib/simpledeploy/backups"}`

#### S3Target:
- Upload: PutObject (or multipart for large files)
- Download: GetObject
- Delete: DeleteObject
- Config: `{"endpoint": "...", "bucket": "...", "prefix": "...", "access_key": "...", "secret_key": "...", "region": "..."}`
- Uses AWS SDK v2 with custom endpoint for S3-compatible storage

#### Tests:
- TestLocalTargetUploadDownload - write, read back, verify content
- TestLocalTargetDelete
- TestPostgresStrategy (mock docker exec - just test the filename generation and interface conformance)
- TestVolumeStrategy (mock docker exec - test filename and interface)
- TestS3Target - skip if no credentials, or use mock

**Dependencies:**
```bash
go get github.com/aws/aws-sdk-go-v2@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/service/s3@latest
go get github.com/aws/aws-sdk-go-v2/credentials@latest
```

- [ ] Commit: `git commit -m "add backup strategies and targets"`

---

### Task 3: Backup Scheduler

**Files:**
- Create: `internal/backup/scheduler.go`
- Create: `internal/backup/scheduler_test.go`

#### Scheduler:
```go
type BackupStore interface {
    ListBackupConfigs(appID *int64) ([]store.BackupConfig, error)
    CreateBackupRun(configID int64) (*store.BackupRun, error)
    UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath string) error
    UpdateBackupRunFailed(id int64, errMsg string) error
    ListOldBackupRuns(configID int64, keepCount int) ([]store.BackupRun, error)
    GetAppByID(id int64) (*store.App, error)
}

type Scheduler struct {
    store      BackupStore
    strategies map[string]Strategy
    targets    map[string]Target
    cron       *cron.Cron
}

func NewScheduler(st BackupStore) *Scheduler

func (s *Scheduler) RegisterStrategy(name string, strategy Strategy)
func (s *Scheduler) RegisterTarget(name string, target Target)

func (s *Scheduler) Start() error
// Reads all backup configs, creates cron entries for each

func (s *Scheduler) Stop()

func (s *Scheduler) RunBackup(ctx context.Context, cfg store.BackupConfig) error
// 1. Create backup_run (status=running)
// 2. Get strategy and target
// 3. Get app name for the container name
// 4. Call strategy.Backup -> get stream
// 5. Upload stream to target
// 6. On success: update run with size+path, prune old runs
// 7. On failure: update run with error

func (s *Scheduler) RunRestore(ctx context.Context, run store.BackupRun, cfg store.BackupConfig) error
// 1. Download from target
// 2. Call strategy.Restore
```

**Dependencies:**
```bash
go get github.com/robfig/cron/v3@latest
```

#### Tests:
- TestSchedulerRunBackup - mock strategy+target, verify create run, upload called, success updated
- TestSchedulerRunBackupFailure - strategy returns error, verify failed status
- TestSchedulerRetention - verify old runs listed for pruning

- [ ] Commit: `git commit -m "add backup scheduler with cron support"`

---

### Task 4: Backup API + CLI + Wire

**Files:**
- Create: `internal/api/backups.go` + `backups_test.go`
- Modify: `internal/api/server.go`
- Modify: `cmd/simpledeploy/main.go`

#### API Endpoints:
```
GET    /api/apps/{slug}/backups/configs     - list backup configs for app
POST   /api/apps/{slug}/backups/configs     - create backup config
DELETE /api/backups/configs/{id}            - delete config

GET    /api/apps/{slug}/backups/runs        - list backup runs
POST   /api/apps/{slug}/backups/run         - trigger backup now
POST   /api/backups/restore/{id}            - restore from run
```

#### CLI commands:
```
simpledeploy backup run --app myapp         - trigger backup
simpledeploy backup list --app myapp        - list runs
simpledeploy restore --app myapp --id 42    - restore
```

#### Wiring in main.go:

```go
import "github.com/vazra/simpledeploy/internal/backup"

// in runServe:
backupScheduler := backup.NewScheduler(db)
backupScheduler.RegisterStrategy("postgres", backup.NewPostgresStrategy(dc))
backupScheduler.RegisterStrategy("volume", backup.NewVolumeStrategy(dc))
backupScheduler.RegisterTarget("local", backup.NewLocalTarget("/var/lib/simpledeploy/backups"))
// S3 target registered per-config when needed
backupScheduler.Start()
defer backupScheduler.Stop()
```

#### Tests:
- TestCreateBackupConfig
- TestListBackupRuns
- TestTriggerBackup (mock)

- [ ] Run full test suite, tidy, build
- [ ] Commit: `git commit -m "add backup API, CLI commands, and wire scheduler"`

---

## Verification Checklist

- [ ] backup_configs and backup_runs tables
- [ ] PostgresStrategy: pg_dump via docker exec
- [ ] VolumeStrategy: tar+gzip via docker exec
- [ ] LocalTarget: file-based backup storage
- [ ] S3Target: S3-compatible upload/download
- [ ] Cron scheduler runs backups on schedule
- [ ] Retention: prunes old runs beyond count
- [ ] Trigger backup via CLI and API
- [ ] Restore via CLI and API
- [ ] All tests pass
