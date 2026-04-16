# Backup System v2 Design

Complete rewrite of SimpleDeploy's backup system. Pipeline architecture (source -> transform -> target), 5 DB strategies, hooks, notifications, config editing, download/upload, checksum verification, compose version history.

Pre-production - breaking changes to existing backup tables/code are acceptable.

## Architecture: Pipeline Model

Every backup flows through a pipeline:

```
Source (Strategy) -> Pre-hooks -> Backup -> Transform (compress, checksum) -> Target (upload) -> Post-hooks -> Record
```

Every restore flows through:

```
Target (download) -> Verify (checksum) -> Transform (decompress) -> Pre-hooks -> Restore (Strategy) -> Post-hooks -> Record
```

### Core Interfaces

```go
// Strategy handles backup/restore for a specific data type.
// Each strategy owns its own detection logic.
type Strategy interface {
    // Type returns the strategy identifier (e.g., "postgres", "mysql").
    Type() string

    // Detect scans a parsed compose config and returns detected services
    // that this strategy can back up.
    Detect(cfg *compose.Config, appName string) []DetectedService

    // Backup produces a data stream from the given container/path.
    Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error)

    // Restore applies data to the given container/path.
    Restore(ctx context.Context, opts RestoreOpts) error
}

type DetectedService struct {
    ServiceName   string   // compose service name
    ContainerName string   // full container name
    Label         string   // human-readable label for UI
    Paths         []string // relevant paths (volume mounts, DB files)
    Metadata      map[string]string // extra info (DB version, etc.)
}

type BackupOpts struct {
    ContainerName string
    Paths         []string // for volume/sqlite: which paths to back up
    Credentials   map[string]string // extracted from container env
}

type BackupResult struct {
    Reader   io.ReadCloser
    Filename string
    Size     int64 // -1 if unknown (streaming)
}

type RestoreOpts struct {
    ContainerName string
    Paths         []string
    Credentials   map[string]string
    Reader        io.ReadCloser
}

// Target handles storage of backup artifacts.
type Target interface {
    Type() string
    Upload(ctx context.Context, filename string, r io.Reader) (path string, size int64, err error)
    Download(ctx context.Context, path string) (io.ReadCloser, error)
    Delete(ctx context.Context, path string) error
    Test(ctx context.Context) error // connection validation
}

// TargetFactory creates a Target from JSON config.
type TargetFactory func(configJSON string) (Target, error)
```

### Pipeline Processor

```go
type Pipeline struct {
    strategy    Strategy
    target      Target
    hooks       *HookRunner
    checksummer *Checksummer
    recorder    *RunRecorder
}

// Run executes the full backup pipeline.
func (p *Pipeline) Backup(ctx context.Context, cfg *BackupConfig) error {
    // 1. Run pre-hooks
    // 2. Call strategy.Backup()
    // 3. Tee stream through SHA-256 hasher + gzip
    // 4. Upload to target
    // 5. Optional: verify upload (re-download + compare hash)
    // 6. Run post-hooks
    // 7. Record result (success/fail, size, checksum, file path)
    // 8. Prune old runs
}

// Restore executes the full restore pipeline.
func (p *Pipeline) Restore(ctx context.Context, run *BackupRun, cfg *BackupConfig) error {
    // 1. Download from target
    // 2. Verify checksum
    // 3. Decompress
    // 4. Run pre-hooks
    // 5. Call strategy.Restore()
    // 6. Run post-hooks
    // 7. Record result
}
```

## Strategies

### PostgreSQL (existing, enhanced)

- Backup: `docker exec {container} pg_dump -U {user} | gzip`
- Restore: `gunzip | docker exec -i {container} psql -U {user}`
- Detection: image contains `postgres` or `postgis` or `timescale` or `pgvector` or `supabase`
- Credentials: `POSTGRES_USER` env (default `postgres`), `POSTGRES_PASSWORD`
- Label override: `simpledeploy.backup.strategy=postgres`

### MySQL/MariaDB (new)

- Backup: `docker exec {container} mysqldump --all-databases -u root -p{password} | gzip`
- Restore: `gunzip | docker exec -i {container} mysql -u root -p{password}`
- Detection: image contains `mysql` or `mariadb` or `percona`
- Credentials: `MYSQL_ROOT_PASSWORD` env from container inspect
- Label override: `simpledeploy.backup.strategy=mysql`

### MongoDB (new)

- Backup: `docker exec {container} mongodump --archive --gzip`
- Restore: `docker exec -i {container} mongorestore --archive --gzip`
- Detection: image contains `mongo`
- Credentials: try unauthenticated first, fall back to `MONGO_INITDB_ROOT_USERNAME` / `MONGO_INITDB_ROOT_PASSWORD`
- Label override: `simpledeploy.backup.strategy=mongo`

### Redis (new)

- Backup: `docker exec {container} redis-cli BGSAVE`, wait for completion, `docker cp {container}:/data/dump.rdb` + gzip
- Restore: stop container, `docker cp` rdb file in, start container
- Detection: image contains `redis` or `valkey` or `dragonfly`
- Label override: `simpledeploy.backup.strategy=redis`

### SQLite (new)

- Backup: `docker exec {container} sqlite3 {path} ".backup '/tmp/sd-backup.db'"`, then `docker cp` out + gzip
- Restore: stop container, `docker cp` file in, start container
- Detection: scan volume mounts for `.db`, `.sqlite`, `.sqlite3` files via `docker exec find`
- User picks which DB files to back up in wizard (all pre-selected)
- Label override: `simpledeploy.backup.strategy=sqlite`

### Volume (existing, enhanced)

- Backup: `docker exec {container} tar -czf - {paths}` (multi-path support)
- Restore: `docker exec -i {container} tar -xzf - -C /`
- Detection: collect all volume mounts (exclude `/var/run/docker.sock`)
- User picks which mounts to back up (all pre-selected by default)
- Label override: `simpledeploy.backup.strategy=volume`

### Label Override (all strategies)

- `simpledeploy.backup.strategy=postgres|mysql|mongo|redis|sqlite|volume` on any compose service
- Label takes priority over image-name auto-detection
- Allows tagging derivative images (timescaledb, keydb, etc.)

## Backup Hooks

### Predefined Actions

| Action | Type | Behavior |
|--------|------|----------|
| Stop container | `stop` | `docker stop {container}` before backup, auto-start after |
| Start container | `start` | `docker start {container}` |
| Flush Redis | `flush_redis` | `redis-cli BGSAVE` + wait for completion |
| Lock MySQL | `flush_mysql` | `FLUSH TABLES WITH READ LOCK` (released post-backup) |

### Custom Docker Exec

- User provides: service name + command string
- Executed via `docker exec {container} sh -c "{command}"`
- Runs inside the container (no host access)

### Execution Model

```go
type Hook struct {
    Type    string `json:"type"`    // "stop"|"start"|"flush_redis"|"flush_mysql"|"exec"
    Service string `json:"service"` // target service name
    Command string `json:"command"` // only for "exec" type
    Timeout int    `json:"timeout"` // seconds, default 60
}
```

- Stored as JSON arrays: `pre_hooks` and `post_hooks` columns on `backup_configs`
- Pre-hooks run sequentially. If any fails, backup aborts, marked failed.
- Post-hooks run sequentially after backup (success or fail). Post-hook failure logged as warning, doesn't change backup status.
- Default timeout: 60s per hook.

### Wizard UX

- Optional step between Schedule and Retention
- Smart suggestions based on strategy:
  - Volume strategy: "Stop container during backup for consistency?" toggle
  - Redis strategy: "Flush to disk before backup?" toggle
- Collapsible "Advanced: Custom command" section
- Warning text: "Custom commands run inside the container as root"

## Retention & Cleanup

### Two Modes (user picks one per config)

- **Count-based**: keep last N successful backups. Default 7.
- **Time-based**: keep backups newer than N days. Default 30.

### Pruning Logic

- Runs after each successful backup
- Count mode: delete oldest successful runs beyond N
- Time mode: delete successful runs older than N days
- Failed runs never auto-deleted (preserved for investigation)
- Cleanup order: delete target file first, then remove DB record
- If target deletion fails, log warning but still remove DB record (prevents orphaned records blocking future prunes)

### Schema

- `retention_mode` TEXT CHECK `'count'|'time'`, default `'count'`
- `retention_days` INTEGER, nullable (used when mode is `time`)
- Existing `retention_count` used when mode is `count`

## Config Editing

- `PUT /api/backups/configs/{id}` endpoint
- All fields editable: strategy, target, schedule, retention, hooks
- When schedule changes, reschedule cron job immediately (hot-reload)
- Wizard reused in edit mode, pre-populated with existing config
- UI: edit icon on each config row in BackupsTab

## Backup Download

- `GET /api/backups/runs/{id}/download`
- Local target: stream file from disk with `Content-Disposition: attachment`
- S3 target: generate pre-signed URL (15min TTL), 302 redirect
- UI: download button on each successful run row

## Upload & Restore from File

- `POST /api/apps/{slug}/backups/upload-restore` (multipart form)
- Accepted formats:
  - Postgres/MySQL: `.sql`, `.sql.gz`
  - MongoDB: `.archive`, `.archive.gz`
  - Redis: `.rdb`
  - SQLite: `.db`, `.sqlite`, `.sqlite3`
  - Volume: `.tar.gz`
- Request body: file + `strategy` type + `container` name
- Validation: extension must match strategy, max file size 5GB (configurable)
- Restore runs async (same pattern as existing restore)
- UI: "Restore from file" button on BackupsTab opens modal
  - Pick strategy type, pick target service/container, upload file, confirm

## Checksum Verification

### Flow

1. During backup: tee data stream through SHA-256 hasher
2. Store hash in `backup_runs.checksum` column
3. Post-upload verify (optional): re-download, re-hash, compare
4. Pre-restore verify (default on): check hash before restoring

### Config

- `verify_upload` BOOLEAN on `backup_configs`, default false
- Pre-restore verification always on (no config needed)

### UI

- Checksum shown on run details (truncated with copy button)
- "Verify after upload" toggle in wizard retention step
- Restore modal shows checksum status (match/mismatch/unavailable)

## Notifications via Existing Alerts

### New Event Types

- `backup_failed` - any backup run fails
- `backup_missed` - scheduled backup didn't execute within 2x its cron interval

### Implementation

- Scheduler calls alert evaluator after backup completion with event context
- Missed detection: background goroutine every 5 min checks last run time vs expected schedule
- Alert payload: app name, config ID, strategy type, error message (if failed), last successful backup time

### UX

- No new notification UI. Users configure via existing Alerts page.
- Backup events appear as new options when creating alert rules.
- Failures appear in existing alert history.

## Compose Version History Enhancement

### Schema Changes to `compose_versions`

- Add `name` TEXT nullable - user-provided label
- Add `notes` TEXT nullable - user-provided description
- Add `env_snapshot` TEXT nullable - JSON snapshot of env vars

### Auto-capture with Backups

- Every backup run stores `compose_version_id` (new FK column on `backup_runs`)
- Compose file + env vars snapshotted automatically when backup executes
- During restore: "This backup was made with compose version X" with option to restore config too

### API

- `PUT /api/apps/{slug}/versions/{id}` - update name and notes
- `GET /api/apps/{slug}/versions/{id}/download` - download compose file as `.yml`
- `POST /api/apps/{slug}/versions/{id}/restore` - redeploy with that version's compose file

### UI

- Version history as timeline in app detail view
- Each entry: timestamp, name (editable inline), notes (editable), change indicator
- Actions: download, restore, edit name/notes
- Side-by-side diff between any two versions
- During restore from backup: shows associated compose version with "also restore config?" option

## Volume Path Selection

### Per-config path configuration (replaces hardcoded `/data`)

- Wizard shows all detected volume mounts (from compose file), all pre-selected
- User can deselect specific mounts (e.g., cache dirs)
- Selected paths stored in backup config (JSON array)
- Volume strategy backs up only selected paths

## S3 Credential Encryption

- `target_config_json` encrypted using existing `auth.Encrypt` (AES-256-GCM with master_secret)
- Decrypted on read in scheduler/API
- Migration encrypts existing plaintext configs

## Database Migration

Single migration (012) covering all backup v2 changes:

```sql
-- Drop old tables
DROP TABLE IF EXISTS backup_runs;
DROP TABLE IF EXISTS backup_configs;

-- New backup_configs
CREATE TABLE backup_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    strategy TEXT NOT NULL CHECK(strategy IN ('postgres','mysql','mongo','redis','sqlite','volume')),
    target TEXT NOT NULL CHECK(target IN ('s3','local')),
    schedule_cron TEXT NOT NULL,
    target_config_json TEXT NOT NULL, -- encrypted
    retention_mode TEXT NOT NULL DEFAULT 'count' CHECK(retention_mode IN ('count','time')),
    retention_count INTEGER NOT NULL DEFAULT 7,
    retention_days INTEGER,
    verify_upload BOOLEAN NOT NULL DEFAULT 0,
    pre_hooks TEXT, -- JSON array of Hook objects
    post_hooks TEXT, -- JSON array of Hook objects
    paths TEXT, -- JSON array of paths (for volume/sqlite strategies)
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- New backup_runs
CREATE TABLE backup_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_config_id INTEGER NOT NULL REFERENCES backup_configs(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK(status IN ('running','success','failed')) DEFAULT 'running',
    size_bytes INTEGER,
    checksum TEXT, -- SHA-256 hex
    file_path TEXT,
    compose_version_id INTEGER REFERENCES compose_versions(id), -- auto-linked at backup time
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME,
    error_msg TEXT
);

CREATE INDEX idx_backup_configs_app ON backup_configs(app_id);
CREATE INDEX idx_backup_runs_config ON backup_runs(backup_config_id);
CREATE INDEX idx_backup_runs_status ON backup_runs(status);

-- Extend compose_versions
ALTER TABLE compose_versions ADD COLUMN name TEXT;
ALTER TABLE compose_versions ADD COLUMN notes TEXT;
ALTER TABLE compose_versions ADD COLUMN env_snapshot TEXT;
```

## API Endpoints (Complete)

### Backup Config CRUD
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/apps/{slug}/backups/configs` | List configs for app |
| POST | `/api/apps/{slug}/backups/configs` | Create config |
| PUT | `/api/backups/configs/{id}` | Update config |
| DELETE | `/api/backups/configs/{id}` | Delete config |

### Backup Execution
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/apps/{slug}/backups/run` | Trigger all configs for app |
| POST | `/api/backups/configs/{id}/run` | Trigger specific config |
| POST | `/api/backups/restore/{id}` | Restore from run |
| POST | `/api/apps/{slug}/backups/upload-restore` | Upload file and restore |

### Backup Runs
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/apps/{slug}/backups/runs` | List runs for app |
| GET | `/api/backups/runs/{id}/download` | Download backup file |

### Detection & Testing
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/apps/{slug}/backups/detect` | Auto-detect strategies |
| POST | `/api/backups/test-s3` | Test S3 connection |

### Global
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/backups/summary` | Cross-app backup health |

### Compose Versions
| Method | Path | Description |
|--------|------|-------------|
| PUT | `/api/apps/{slug}/versions/{id}` | Update name/notes |
| GET | `/api/apps/{slug}/versions/{id}/download` | Download compose file |
| POST | `/api/apps/{slug}/versions/{id}/restore` | Restore compose version |

## UI Changes

### Backup Wizard (rewritten)

5-step flow (6 with hooks):
1. **What to back up** - auto-detected strategies with label override info, service selection
2. **Where to store** - local/S3 with test connection
3. **Schedule** - existing ScheduleBuilder
4. **Hooks** (optional) - predefined suggestions + custom exec
5. **Retention & verification** - count/time toggle, verify upload toggle
6. **Summary** - review all settings, create/update

Edit mode: same wizard, pre-populated.

### BackupsTab (enhanced)

- Config table: adds edit button, shows hooks indicator, retention mode
- Run history: adds download button, checksum indicator, compose version link
- New "Restore from file" button
- Status header: unchanged

### Backups Dashboard (enhanced)

- Existing stat cards + new: "Missed Backups (24h)"
- Health cards: add strategy icons, retention info
- Recent activity: add download links

### Compose Versions UI (new section)

- Timeline view in app detail
- Inline editing for name/notes
- Download/restore actions
- Side-by-side diff viewer

## File Structure

```
internal/backup/
    pipeline.go         -- Pipeline orchestrator
    strategy.go         -- Strategy interface + DetectedService
    target.go           -- Target interface + TargetFactory
    hooks.go            -- HookRunner, predefined actions
    checksum.go         -- SHA-256 streaming hasher + verification
    detect.go           -- Detection coordinator (label override logic)
    postgres.go         -- PostgreSQL strategy (enhanced)
    mysql.go            -- MySQL/MariaDB strategy (new)
    mongo.go            -- MongoDB strategy (new)
    redis.go            -- Redis strategy (new)
    sqlite.go           -- SQLite strategy (new)
    volume.go           -- Volume strategy (enhanced, multi-path)
    local.go            -- Local filesystem target (unchanged)
    s3.go               -- S3 target (enhanced, pre-signed URLs)
    scheduler.go        -- Cron scheduler (enhanced, hot-reload, missed detection)
    *_test.go           -- Tests for each file
```
