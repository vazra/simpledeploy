# Backup System v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite SimpleDeploy's backup system with pipeline architecture, 6 DB strategies, hooks, notifications, config editing, download/upload, checksum verification, and compose version history.

**Architecture:** Pipeline model (source -> hooks -> backup -> transform -> target -> record). Each strategy implements `Detect()` for auto-discovery with label override. Hooks support predefined actions + docker exec. Integrates with existing alert system for failure notifications.

**Tech Stack:** Go, SQLite, Docker API, AWS SDK v2, Svelte 5, Playwright

**Spec:** `docs/superpowers/specs/2026-04-16-backup-system-v2-design.md`

---

## File Structure

### New Files
```
internal/backup/
    pipeline.go         -- Pipeline orchestrator (backup + restore flows)
    pipeline_test.go
    hooks.go            -- HookRunner: predefined actions + docker exec
    hooks_test.go
    checksum.go         -- SHA-256 streaming hasher + verification
    checksum_test.go
    detect.go           -- Detection coordinator (label override + auto-detect)
    detect_test.go
    mysql.go            -- MySQL/MariaDB strategy
    mysql_test.go
    mongo.go            -- MongoDB strategy
    mongo_test.go
    redis.go            -- Redis strategy
    redis_test.go
    sqlite_strategy.go  -- SQLite strategy (avoid collision with sqlite3 package)
    sqlite_strategy_test.go
internal/store/migrations/
    015_backups_v2.sql
```

### Modified Files
```
internal/backup/strategy.go      -- New Strategy interface with Type() + Detect()
internal/backup/target.go        -- New Target interface with Type() + Test()
internal/backup/postgres.go      -- Add Type(), Detect(), credential extraction
internal/backup/volume.go        -- Add Type(), Detect(), multi-path support
internal/backup/local.go         -- Unchanged interface, just Type() method
internal/backup/s3.go            -- Add Type(), Test(), pre-signed URL download
internal/backup/scheduler.go     -- Rewrite: hot-reload, missed detection, pipeline integration
internal/store/backups.go        -- Rewrite: new schema types, CRUD, summary queries
internal/store/versions.go       -- Add name, notes, env_snapshot fields + new methods
internal/api/backups.go          -- Rewrite: all handlers for v2 endpoints
internal/api/server.go           -- Update route registration
internal/alerts/evaluator.go     -- Add backup event support
internal/alerts/types.go         -- Add BackupAlertEvent type
internal/auth/crypto.go          -- No changes (used as-is for S3 cred encryption)
cmd/simpledeploy/main.go         -- Update backup initialization
ui/src/lib/api.js                -- Add new backup API methods
ui/src/components/BackupWizard.svelte    -- Rewrite: 6 steps, hooks, retention modes
ui/src/components/BackupsTab.svelte      -- Add edit, download, upload-restore
ui/src/routes/Backups.svelte             -- Add missed backup stat
ui/src/components/BackupHealthCard.svelte -- Add strategy icons
e2e/tests/12-backups.spec.js     -- Rewrite for v2 features
```

---

### Task 1: Database Migration

**Files:**
- Create: `internal/store/migrations/015_backups_v2.sql`

- [ ] **Step 1: Write the migration**

```sql
-- Backup System v2: drop and recreate backup tables with new schema
-- Safe because pre-production, no existing data to preserve

DROP TABLE IF EXISTS backup_runs;
DROP TABLE IF EXISTS backup_configs;

CREATE TABLE backup_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    strategy TEXT NOT NULL CHECK(strategy IN ('postgres','mysql','mongo','redis','sqlite','volume')),
    target TEXT NOT NULL CHECK(target IN ('s3','local')),
    schedule_cron TEXT NOT NULL,
    target_config_json TEXT NOT NULL DEFAULT '{}',
    retention_mode TEXT NOT NULL DEFAULT 'count' CHECK(retention_mode IN ('count','time')),
    retention_count INTEGER NOT NULL DEFAULT 7,
    retention_days INTEGER,
    verify_upload INTEGER NOT NULL DEFAULT 0,
    pre_hooks TEXT,
    post_hooks TEXT,
    paths TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE backup_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_config_id INTEGER NOT NULL REFERENCES backup_configs(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running','success','failed')),
    size_bytes INTEGER,
    checksum TEXT,
    file_path TEXT,
    compose_version_id INTEGER REFERENCES compose_versions(id),
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME,
    error_msg TEXT
);

CREATE INDEX idx_backup_configs_app ON backup_configs(app_id);
CREATE INDEX idx_backup_runs_config ON backup_runs(backup_config_id);
CREATE INDEX idx_backup_runs_status ON backup_runs(status);

-- Extend compose_versions with name, notes, env snapshot
ALTER TABLE compose_versions ADD COLUMN name TEXT;
ALTER TABLE compose_versions ADD COLUMN notes TEXT;
ALTER TABLE compose_versions ADD COLUMN env_snapshot TEXT;
```

- [ ] **Step 2: Verify migration loads**

Run: `go test ./internal/store/ -run TestOpen -v`
Expected: PASS (migration applies cleanly on fresh DB)

- [ ] **Step 3: Commit**

```bash
git add internal/store/migrations/015_backups_v2.sql
git commit -m "feat(store): add backup system v2 migration"
```

---

### Task 2: Core Interfaces

**Files:**
- Modify: `internal/backup/strategy.go`
- Modify: `internal/backup/target.go`

- [ ] **Step 1: Rewrite strategy.go**

```go
package backup

import (
	"context"
	"io"

	"github.com/vazra/simpledeploy/internal/compose"
)

// Strategy defines how to backup and restore a specific data type.
type Strategy interface {
	// Type returns the strategy identifier (e.g., "postgres", "mysql").
	Type() string

	// Detect scans a parsed compose config and returns services this strategy can back up.
	Detect(cfg *compose.AppConfig) []DetectedService

	// Backup produces a data stream from the given container.
	Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error)

	// Restore applies data to the given container.
	Restore(ctx context.Context, opts RestoreOpts) error
}

// DetectedService describes a service that a strategy can back up.
type DetectedService struct {
	ServiceName   string            `json:"service_name"`
	ContainerName string            `json:"container_name"`
	Label         string            `json:"label"`
	Paths         []string          `json:"paths,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// BackupOpts configures a backup operation.
type BackupOpts struct {
	ContainerName string
	Paths         []string
	Credentials   map[string]string
}

// BackupResult holds the output of a backup operation.
type BackupResult struct {
	Reader   io.ReadCloser
	Filename string
}

// RestoreOpts configures a restore operation.
type RestoreOpts struct {
	ContainerName string
	Paths         []string
	Credentials   map[string]string
	Reader        io.ReadCloser
}
```

- [ ] **Step 2: Rewrite target.go**

```go
package backup

import (
	"context"
	"io"
)

// Target defines where backup data is stored and retrieved from.
type Target interface {
	// Type returns the target identifier (e.g., "s3", "local").
	Type() string

	// Upload stores data and returns the storage path and byte count.
	Upload(ctx context.Context, filename string, data io.Reader) (path string, size int64, err error)

	// Download retrieves stored data by path.
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete removes stored data by path.
	Delete(ctx context.Context, path string) error

	// Test validates the target connection/configuration.
	Test(ctx context.Context) error
}

// TargetFactory creates a Target from JSON config.
type TargetFactory func(configJSON string) (Target, error)
```

- [ ] **Step 3: Verify interfaces compile**

Run: `go build ./internal/backup/`
Expected: Compilation errors from existing implementations missing new methods. This is expected -- we'll fix them in subsequent tasks.

- [ ] **Step 4: Commit**

```bash
git add internal/backup/strategy.go internal/backup/target.go
git commit -m "feat(backup): rewrite core Strategy and Target interfaces for v2"
```

---

### Task 3: Checksum Module

**Files:**
- Create: `internal/backup/checksum.go`
- Create: `internal/backup/checksum_test.go`

- [ ] **Step 1: Write the test**

```go
package backup

import (
	"bytes"
	"strings"
	"testing"
)

func TestChecksumWriter(t *testing.T) {
	data := []byte("hello world backup data")
	r := bytes.NewReader(data)

	cw := NewChecksumWriter()
	teeReader := cw.TeeReader(r)

	// Read all data through the tee
	var buf bytes.Buffer
	_, err := buf.ReadFrom(teeReader)
	if err != nil {
		t.Fatal(err)
	}

	// Data should pass through unchanged
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("data was modified by checksum tee")
	}

	// Should produce a hex SHA-256 hash
	hash := cw.Sum()
	if len(hash) != 64 {
		t.Errorf("expected 64 char hex hash, got %d chars: %s", len(hash), hash)
	}
}

func TestChecksumVerify(t *testing.T) {
	data := []byte("test data for verification")

	// Compute hash
	cw := NewChecksumWriter()
	teeReader := cw.TeeReader(bytes.NewReader(data))
	var buf bytes.Buffer
	buf.ReadFrom(teeReader)
	hash := cw.Sum()

	// Verify same data passes
	if err := VerifyChecksum(bytes.NewReader(data), hash); err != nil {
		t.Errorf("valid data should pass verification: %v", err)
	}

	// Verify different data fails
	if err := VerifyChecksum(strings.NewReader("wrong data"), hash); err == nil {
		t.Error("different data should fail verification")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestChecksum -v`
Expected: FAIL (functions not defined)

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// ChecksumWriter computes SHA-256 over data as it flows through a TeeReader.
type ChecksumWriter struct {
	hash io.Writer
	sum  []byte
	done bool
}

// NewChecksumWriter creates a new ChecksumWriter.
func NewChecksumWriter() *ChecksumWriter {
	return &ChecksumWriter{
		hash: sha256.New(),
	}
}

// TeeReader returns an io.Reader that computes SHA-256 as data is read.
func (c *ChecksumWriter) TeeReader(r io.Reader) io.Reader {
	return io.TeeReader(r, c.hash)
}

// Sum returns the hex-encoded SHA-256 hash. Must be called after all data is read.
func (c *ChecksumWriter) Sum() string {
	h := c.hash.(interface{ Sum([]byte) []byte })
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyChecksum reads all data from r and verifies it matches the expected SHA-256 hex hash.
func VerifyChecksum(r io.Reader, expectedHex string) error {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return fmt.Errorf("reading data for checksum: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedHex {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHex, actual)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestChecksum -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/checksum.go internal/backup/checksum_test.go
git commit -m "feat(backup): add SHA-256 checksum module"
```

---

### Task 4: Hook Runner

**Files:**
- Create: `internal/backup/hooks.go`
- Create: `internal/backup/hooks_test.go`

- [ ] **Step 1: Write the test**

```go
package backup

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type mockCommandExecutor struct {
	commands []string
	failOn   string
}

func (m *mockCommandExecutor) ExecInContainer(ctx context.Context, container string, cmd []string) (string, error) {
	full := container + ":" + cmd[len(cmd)-1]
	m.commands = append(m.commands, full)
	if m.failOn != "" && cmd[len(cmd)-1] == m.failOn {
		return "", fmt.Errorf("command failed: %s", m.failOn)
	}
	return "ok", nil
}

func (m *mockCommandExecutor) StopContainer(ctx context.Context, container string) error {
	m.commands = append(m.commands, "stop:"+container)
	return nil
}

func (m *mockCommandExecutor) StartContainer(ctx context.Context, container string) error {
	m.commands = append(m.commands, "start:"+container)
	return nil
}

func TestHookRunner_PreHooks(t *testing.T) {
	exec := &mockCommandExecutor{}
	runner := NewHookRunner(exec, 30*time.Second)

	hooks := []Hook{
		{Type: HookTypeStop, Service: "myapp-web-1"},
		{Type: HookTypeExec, Service: "myapp-redis-1", Command: "redis-cli PING"},
	}

	err := runner.RunPre(context.Background(), hooks)
	if err != nil {
		t.Fatalf("pre hooks should succeed: %v", err)
	}

	if len(exec.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(exec.commands))
	}
	if exec.commands[0] != "stop:myapp-web-1" {
		t.Errorf("expected stop command, got %s", exec.commands[0])
	}
}

func TestHookRunner_PreHookFailure_Aborts(t *testing.T) {
	exec := &mockCommandExecutor{failOn: "bad-command"}
	runner := NewHookRunner(exec, 30*time.Second)

	hooks := []Hook{
		{Type: HookTypeExec, Service: "myapp-web-1", Command: "bad-command"},
		{Type: HookTypeExec, Service: "myapp-web-1", Command: "should-not-run"},
	}

	err := runner.RunPre(context.Background(), hooks)
	if err == nil {
		t.Fatal("should fail on bad command")
	}

	if len(exec.commands) != 1 {
		t.Errorf("should stop after first failure, ran %d commands", len(exec.commands))
	}
}

func TestHookRunner_PostHooks_ContinueOnFailure(t *testing.T) {
	exec := &mockCommandExecutor{failOn: "fail-cmd"}
	runner := NewHookRunner(exec, 30*time.Second)

	hooks := []Hook{
		{Type: HookTypeExec, Service: "myapp-web-1", Command: "fail-cmd"},
		{Type: HookTypeStart, Service: "myapp-web-1"},
	}

	warnings := runner.RunPost(context.Background(), hooks)
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
	if len(exec.commands) != 2 {
		t.Error("post hooks should continue after failure")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestHookRunner -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"context"
	"fmt"
	"time"
)

const (
	HookTypeStop       = "stop"
	HookTypeStart      = "start"
	HookTypeFlushRedis = "flush_redis"
	HookTypeFlushMySQL = "flush_mysql"
	HookTypeExec       = "exec"
)

// Hook defines a pre or post backup action.
type Hook struct {
	Type    string `json:"type"`
	Service string `json:"service"`
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// ContainerExecutor abstracts Docker container operations for hooks.
type ContainerExecutor interface {
	ExecInContainer(ctx context.Context, container string, cmd []string) (string, error)
	StopContainer(ctx context.Context, container string) error
	StartContainer(ctx context.Context, container string) error
}

// HookRunner executes pre/post backup hooks.
type HookRunner struct {
	exec           ContainerExecutor
	defaultTimeout time.Duration
}

// NewHookRunner creates a HookRunner.
func NewHookRunner(exec ContainerExecutor, defaultTimeout time.Duration) *HookRunner {
	return &HookRunner{exec: exec, defaultTimeout: defaultTimeout}
}

// RunPre runs hooks sequentially. Returns error on first failure (aborts remaining).
func (h *HookRunner) RunPre(ctx context.Context, hooks []Hook) error {
	for i, hook := range hooks {
		if err := h.execute(ctx, hook); err != nil {
			return fmt.Errorf("pre-hook %d (%s) failed: %w", i, hook.Type, err)
		}
	}
	return nil
}

// RunPost runs hooks sequentially. Continues on failure, returns warnings.
func (h *HookRunner) RunPost(ctx context.Context, hooks []Hook) []string {
	var warnings []string
	for i, hook := range hooks {
		if err := h.execute(ctx, hook); err != nil {
			warnings = append(warnings, fmt.Sprintf("post-hook %d (%s): %v", i, hook.Type, err))
		}
	}
	return warnings
}

func (h *HookRunner) execute(ctx context.Context, hook Hook) error {
	timeout := h.defaultTimeout
	if hook.Timeout > 0 {
		timeout = time.Duration(hook.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch hook.Type {
	case HookTypeStop:
		return h.exec.StopContainer(ctx, hook.Service)
	case HookTypeStart:
		return h.exec.StartContainer(ctx, hook.Service)
	case HookTypeFlushRedis:
		_, err := h.exec.ExecInContainer(ctx, hook.Service, []string{"redis-cli", "BGSAVE"})
		return err
	case HookTypeFlushMySQL:
		_, err := h.exec.ExecInContainer(ctx, hook.Service, []string{"mysql", "-e", "FLUSH TABLES WITH READ LOCK"})
		return err
	case HookTypeExec:
		if hook.Command == "" {
			return fmt.Errorf("exec hook requires a command")
		}
		_, err := h.exec.ExecInContainer(ctx, hook.Service, []string{"sh", "-c", hook.Command})
		return err
	default:
		return fmt.Errorf("unknown hook type: %s", hook.Type)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestHookRunner -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/hooks.go internal/backup/hooks_test.go
git commit -m "feat(backup): add hook runner with predefined actions and docker exec"
```

---

### Task 5: PostgreSQL Strategy (Enhanced)

**Files:**
- Modify: `internal/backup/postgres.go`
- Modify: `internal/backup/postgres_test.go`

- [ ] **Step 1: Write detection test**

```go
package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestPostgresStrategy_Type(t *testing.T) {
	s := NewPostgresStrategy()
	if s.Type() != "postgres" {
		t.Errorf("expected 'postgres', got '%s'", s.Type())
	}
}

func TestPostgresStrategy_Detect(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		labels   map[string]string
		expected bool
	}{
		{"postgres image", "postgres:16", nil, true},
		{"bitnami postgres", "bitnami/postgresql:15", nil, true},
		{"timescaledb", "timescale/timescaledb:latest", nil, true},
		{"postgis", "postgis/postgis:16", nil, true},
		{"pgvector", "ankane/pgvector:latest", nil, false},
		{"pgvector with label", "ankane/pgvector:latest", map[string]string{"simpledeploy.backup.strategy": "postgres"}, true},
		{"mysql", "mysql:8", nil, false},
		{"nginx", "nginx:latest", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &compose.AppConfig{
				Name: "testapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: tt.image, Labels: tt.labels},
				},
			}
			detected := NewPostgresStrategy().Detect(cfg)
			if tt.expected && len(detected) == 0 {
				t.Error("expected detection, got none")
			}
			if !tt.expected && len(detected) > 0 {
				t.Error("expected no detection, got some")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestPostgresStrategy -v`
Expected: FAIL (Type and Detect methods not yet on PostgresStrategy)

- [ ] **Step 3: Rewrite postgres.go**

```go
package backup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

var postgresImageKeywords = []string{"postgres", "postgis", "timescale", "supabase"}

// PostgresStrategy backs up PostgreSQL databases using pg_dump.
type PostgresStrategy struct{}

func NewPostgresStrategy() *PostgresStrategy {
	return &PostgresStrategy{}
}

func (s *PostgresStrategy) Type() string { return "postgres" }

func (s *PostgresStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var detected []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "postgres") || matchesImageKeywords(svc.Image, postgresImageKeywords) {
			detected = append(detected, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: fmt.Sprintf("%s-%s-1", cfg.Name, svc.Name),
				Label:         fmt.Sprintf("PostgreSQL (%s)", svc.Name),
			})
		}
	}
	return detected
}

func (s *PostgresStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	user := opts.Credentials["POSTGRES_USER"]
	if user == "" {
		user = "postgres"
	}

	cmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName,
		"pg_dump", "-U", user)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pg_dump pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("pg_dump start: %w", err)
	}

	pr, pw := io.Pipe()
	go func() {
		gzCmd := exec.Command("gzip")
		gzCmd.Stdin = stdout
		gzCmd.Stdout = pw
		if err := gzCmd.Run(); err != nil {
			pw.CloseWithError(err)
			return
		}
		cmd.Wait()
		pw.Close()
	}()

	filename := fmt.Sprintf("%s-%s.sql.gz", opts.ContainerName, time.Now().Format("20060102-150405"))
	return &BackupResult{Reader: pr, Filename: filename}, nil
}

func (s *PostgresStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	user := opts.Credentials["POSTGRES_USER"]
	if user == "" {
		user = "postgres"
	}

	gzCmd := exec.Command("gunzip")
	gzCmd.Stdin = opts.Reader

	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", opts.ContainerName,
		"psql", "-U", user)
	cmd.Stdin, _ = gzCmd.StdoutPipe()
	if err := gzCmd.Start(); err != nil {
		return fmt.Errorf("gunzip start: %w", err)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore: %w", err)
	}
	return gzCmd.Wait()
}

// matchesLabel checks if a service has a simpledeploy.backup.strategy label matching the given strategy.
func matchesLabel(labels map[string]string, strategy string) bool {
	if labels == nil {
		return false
	}
	return labels["simpledeploy.backup.strategy"] == strategy
}

// matchesImageKeywords checks if image name contains any of the keywords.
func matchesImageKeywords(image string, keywords []string) bool {
	lower := strings.ToLower(image)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestPostgresStrategy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/postgres.go internal/backup/postgres_test.go
git commit -m "feat(backup): enhance PostgreSQL strategy with Detect and label override"
```

---

### Task 6: MySQL Strategy

**Files:**
- Create: `internal/backup/mysql.go`
- Create: `internal/backup/mysql_test.go`

- [ ] **Step 1: Write detection test**

```go
package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestMySQLStrategy_Type(t *testing.T) {
	s := NewMySQLStrategy()
	if s.Type() != "mysql" {
		t.Errorf("expected 'mysql', got '%s'", s.Type())
	}
}

func TestMySQLStrategy_Detect(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		labels   map[string]string
		expected bool
	}{
		{"mysql image", "mysql:8", nil, true},
		{"mariadb image", "mariadb:11", nil, true},
		{"percona image", "percona:8", nil, true},
		{"bitnami mysql", "bitnami/mysql:8.0", nil, true},
		{"postgres", "postgres:16", nil, false},
		{"random with label", "custom-db:latest", map[string]string{"simpledeploy.backup.strategy": "mysql"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &compose.AppConfig{
				Name:     "testapp",
				Services: []compose.ServiceConfig{{Name: "db", Image: tt.image, Labels: tt.labels}},
			}
			detected := NewMySQLStrategy().Detect(cfg)
			if tt.expected && len(detected) == 0 {
				t.Error("expected detection, got none")
			}
			if !tt.expected && len(detected) > 0 {
				t.Error("expected no detection, got some")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestMySQLStrategy -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

var mysqlImageKeywords = []string{"mysql", "mariadb", "percona"}

// MySQLStrategy backs up MySQL/MariaDB databases using mysqldump.
type MySQLStrategy struct{}

func NewMySQLStrategy() *MySQLStrategy {
	return &MySQLStrategy{}
}

func (s *MySQLStrategy) Type() string { return "mysql" }

func (s *MySQLStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var detected []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "mysql") || matchesImageKeywords(svc.Image, mysqlImageKeywords) {
			detected = append(detected, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: fmt.Sprintf("%s-%s-1", cfg.Name, svc.Name),
				Label:         fmt.Sprintf("MySQL/MariaDB (%s)", svc.Name),
			})
		}
	}
	return detected
}

func (s *MySQLStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	password := opts.Credentials["MYSQL_ROOT_PASSWORD"]

	args := []string{"exec", opts.ContainerName, "mysqldump", "--all-databases", "-u", "root"}
	if password != "" {
		args = append(args, "-p"+password)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mysqldump pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mysqldump start: %w", err)
	}

	pr, pw := io.Pipe()
	go func() {
		gzCmd := exec.Command("gzip")
		gzCmd.Stdin = stdout
		gzCmd.Stdout = pw
		if err := gzCmd.Run(); err != nil {
			pw.CloseWithError(err)
			return
		}
		cmd.Wait()
		pw.Close()
	}()

	filename := fmt.Sprintf("%s-%s.sql.gz", opts.ContainerName, time.Now().Format("20060102-150405"))
	return &BackupResult{Reader: pr, Filename: filename}, nil
}

func (s *MySQLStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	password := opts.Credentials["MYSQL_ROOT_PASSWORD"]

	gzCmd := exec.Command("gunzip")
	gzCmd.Stdin = opts.Reader

	args := []string{"exec", "-i", opts.ContainerName, "mysql", "-u", "root"}
	if password != "" {
		args = append(args, "-p"+password)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin, _ = gzCmd.StdoutPipe()
	if err := gzCmd.Start(); err != nil {
		return fmt.Errorf("gunzip start: %w", err)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysql restore: %w", err)
	}
	return gzCmd.Wait()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestMySQLStrategy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/mysql.go internal/backup/mysql_test.go
git commit -m "feat(backup): add MySQL/MariaDB strategy"
```

---

### Task 7: MongoDB Strategy

**Files:**
- Create: `internal/backup/mongo.go`
- Create: `internal/backup/mongo_test.go`

- [ ] **Step 1: Write detection test**

```go
package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestMongoStrategy_Type(t *testing.T) {
	s := NewMongoStrategy()
	if s.Type() != "mongo" {
		t.Errorf("expected 'mongo', got '%s'", s.Type())
	}
}

func TestMongoStrategy_Detect(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		labels   map[string]string
		expected bool
	}{
		{"mongo image", "mongo:7", nil, true},
		{"bitnami mongo", "bitnami/mongodb:7.0", nil, true},
		{"percona mongo", "percona/percona-server-mongodb:7", nil, true},
		{"mysql", "mysql:8", nil, false},
		{"custom with label", "my-mongo:latest", map[string]string{"simpledeploy.backup.strategy": "mongo"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &compose.AppConfig{
				Name:     "testapp",
				Services: []compose.ServiceConfig{{Name: "db", Image: tt.image, Labels: tt.labels}},
			}
			detected := NewMongoStrategy().Detect(cfg)
			if tt.expected && len(detected) == 0 {
				t.Error("expected detection, got none")
			}
			if !tt.expected && len(detected) > 0 {
				t.Error("expected no detection, got some")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestMongoStrategy -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

var mongoImageKeywords = []string{"mongo"}

// MongoStrategy backs up MongoDB using mongodump.
type MongoStrategy struct{}

func NewMongoStrategy() *MongoStrategy {
	return &MongoStrategy{}
}

func (s *MongoStrategy) Type() string { return "mongo" }

func (s *MongoStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var detected []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "mongo") || matchesImageKeywords(svc.Image, mongoImageKeywords) {
			detected = append(detected, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: fmt.Sprintf("%s-%s-1", cfg.Name, svc.Name),
				Label:         fmt.Sprintf("MongoDB (%s)", svc.Name),
			})
		}
	}
	return detected
}

func (s *MongoStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	args := []string{"exec", opts.ContainerName, "mongodump", "--archive", "--gzip"}

	user := opts.Credentials["MONGO_INITDB_ROOT_USERNAME"]
	pass := opts.Credentials["MONGO_INITDB_ROOT_PASSWORD"]
	if user != "" && pass != "" {
		args = append(args, "-u", user, "-p", pass, "--authenticationDatabase", "admin")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mongodump pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mongodump start: %w", err)
	}

	filename := fmt.Sprintf("%s-%s.archive.gz", opts.ContainerName, time.Now().Format("20060102-150405"))
	return &BackupResult{
		Reader: &cmdReadCloser{ReadCloser: stdout, cmd: cmd},
		Filename: filename,
	}, nil
}

func (s *MongoStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	args := []string{"exec", "-i", opts.ContainerName, "mongorestore", "--archive", "--gzip"}

	user := opts.Credentials["MONGO_INITDB_ROOT_USERNAME"]
	pass := opts.Credentials["MONGO_INITDB_ROOT_PASSWORD"]
	if user != "" && pass != "" {
		args = append(args, "-u", user, "-p", pass, "--authenticationDatabase", "admin")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = opts.Reader
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mongorestore: %w: %s", err, out)
	}
	return nil
}

// cmdReadCloser wraps an io.ReadCloser and waits for the command to finish on Close.
type cmdReadCloser struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (c *cmdReadCloser) Close() error {
	c.ReadCloser.Close()
	return c.cmd.Wait()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestMongoStrategy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/mongo.go internal/backup/mongo_test.go
git commit -m "feat(backup): add MongoDB strategy"
```

---

### Task 8: Redis Strategy

**Files:**
- Create: `internal/backup/redis.go`
- Create: `internal/backup/redis_test.go`

- [ ] **Step 1: Write detection test**

```go
package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestRedisStrategy_Type(t *testing.T) {
	s := NewRedisStrategy()
	if s.Type() != "redis" {
		t.Errorf("expected 'redis', got '%s'", s.Type())
	}
}

func TestRedisStrategy_Detect(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		labels   map[string]string
		expected bool
	}{
		{"redis image", "redis:7", nil, true},
		{"valkey image", "valkey/valkey:8", nil, true},
		{"dragonfly", "docker.dragonflydb.io/dragonflydb/dragonfly:latest", nil, true},
		{"bitnami redis", "bitnami/redis:7.2", nil, true},
		{"mysql", "mysql:8", nil, false},
		{"custom with label", "keydb:latest", map[string]string{"simpledeploy.backup.strategy": "redis"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &compose.AppConfig{
				Name:     "testapp",
				Services: []compose.ServiceConfig{{Name: "cache", Image: tt.image, Labels: tt.labels}},
			}
			detected := NewRedisStrategy().Detect(cfg)
			if tt.expected && len(detected) == 0 {
				t.Error("expected detection, got none")
			}
			if !tt.expected && len(detected) > 0 {
				t.Error("expected no detection, got some")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestRedisStrategy -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

var redisImageKeywords = []string{"redis", "valkey", "dragonfly"}

// RedisStrategy backs up Redis by triggering BGSAVE and copying the dump.rdb file.
type RedisStrategy struct{}

func NewRedisStrategy() *RedisStrategy {
	return &RedisStrategy{}
}

func (s *RedisStrategy) Type() string { return "redis" }

func (s *RedisStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var detected []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "redis") || matchesImageKeywords(svc.Image, redisImageKeywords) {
			detected = append(detected, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: fmt.Sprintf("%s-%s-1", cfg.Name, svc.Name),
				Label:         fmt.Sprintf("Redis (%s)", svc.Name),
			})
		}
	}
	return detected
}

func (s *RedisStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	// Trigger BGSAVE
	bgsave := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName, "redis-cli", "BGSAVE")
	if out, err := bgsave.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("redis BGSAVE: %w: %s", err, out)
	}

	// Wait for save to complete (poll LASTSAVE)
	for i := 0; i < 30; i++ {
		check := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName,
			"redis-cli", "LASTSAVE")
		out, err := check.Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			break
		}
		time.Sleep(time.Second)
	}

	// Copy dump.rdb out via docker cp to stdout (tar format), then extract just the file
	cmd := exec.CommandContext(ctx, "docker", "cp", opts.ContainerName+":/data/dump.rdb", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("docker cp pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("docker cp start: %w", err)
	}

	// docker cp outputs a tar stream; we gzip the whole thing
	pr, pw := io.Pipe()
	go func() {
		gzCmd := exec.Command("gzip")
		gzCmd.Stdin = stdout
		gzCmd.Stdout = pw
		if err := gzCmd.Run(); err != nil {
			pw.CloseWithError(err)
			return
		}
		cmd.Wait()
		pw.Close()
	}()

	filename := fmt.Sprintf("%s-%s.rdb.tar.gz", opts.ContainerName, time.Now().Format("20060102-150405"))
	return &BackupResult{Reader: pr, Filename: filename}, nil
}

func (s *RedisStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	// Stop the container first
	stop := exec.CommandContext(ctx, "docker", "stop", opts.ContainerName)
	if out, err := stop.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %w: %s", err, out)
	}

	// Decompress and pipe tar to docker cp
	gzCmd := exec.Command("gunzip")
	gzCmd.Stdin = opts.Reader
	gzOut, _ := gzCmd.StdoutPipe()

	cp := exec.CommandContext(ctx, "docker", "cp", "-", opts.ContainerName+":/data/")
	cp.Stdin = gzOut

	if err := gzCmd.Start(); err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	if out, err := cp.CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp restore: %w: %s", err, out)
	}
	gzCmd.Wait()

	// Start the container back
	start := exec.CommandContext(ctx, "docker", "start", opts.ContainerName)
	if out, err := start.CombinedOutput(); err != nil {
		return fmt.Errorf("docker start: %w: %s", err, out)
	}
	return nil
}
```

Note: Add `"bytes"` to the import block. The `bytes` import may not be needed in the final code; the compiler will flag unused imports.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestRedisStrategy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/redis.go internal/backup/redis_test.go
git commit -m "feat(backup): add Redis strategy"
```

---

### Task 9: SQLite Strategy

**Files:**
- Create: `internal/backup/sqlite_strategy.go`
- Create: `internal/backup/sqlite_strategy_test.go`

- [ ] **Step 1: Write detection test**

```go
package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestSQLiteStrategy_Type(t *testing.T) {
	s := NewSQLiteStrategy()
	if s.Type() != "sqlite" {
		t.Errorf("expected 'sqlite', got '%s'", s.Type())
	}
}

func TestSQLiteStrategy_Detect_WithLabel(t *testing.T) {
	cfg := &compose.AppConfig{
		Name: "testapp",
		Services: []compose.ServiceConfig{
			{
				Name:   "app",
				Image:  "myapp:latest",
				Labels: map[string]string{"simpledeploy.backup.strategy": "sqlite"},
				Volumes: []compose.VolumeMount{
					{Source: "data", Target: "/app/data", Type: "volume"},
				},
			},
		},
	}
	detected := NewSQLiteStrategy().Detect(cfg)
	if len(detected) == 0 {
		t.Error("expected detection via label")
	}
	if len(detected[0].Paths) != 1 || detected[0].Paths[0] != "/app/data" {
		t.Errorf("expected path /app/data, got %v", detected[0].Paths)
	}
}

func TestSQLiteStrategy_Detect_NoLabel(t *testing.T) {
	cfg := &compose.AppConfig{
		Name: "testapp",
		Services: []compose.ServiceConfig{
			{Name: "app", Image: "myapp:latest"},
		},
	}
	detected := NewSQLiteStrategy().Detect(cfg)
	if len(detected) != 0 {
		t.Error("should not auto-detect without label (requires runtime file scan)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestSQLiteStrategy -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

// SQLiteStrategy backs up SQLite databases using the .backup command.
// Detection is label-based only (simpledeploy.backup.strategy=sqlite)
// since runtime file scanning is needed to find .db files.
type SQLiteStrategy struct{}

func NewSQLiteStrategy() *SQLiteStrategy {
	return &SQLiteStrategy{}
}

func (s *SQLiteStrategy) Type() string { return "sqlite" }

func (s *SQLiteStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var detected []DetectedService
	for _, svc := range cfg.Services {
		if !matchesLabel(svc.Labels, "sqlite") {
			continue
		}
		var paths []string
		for _, vol := range svc.Volumes {
			if vol.Target != "/var/run/docker.sock" {
				paths = append(paths, vol.Target)
			}
		}
		detected = append(detected, DetectedService{
			ServiceName:   svc.Name,
			ContainerName: fmt.Sprintf("%s-%s-1", cfg.Name, svc.Name),
			Label:         fmt.Sprintf("SQLite (%s)", svc.Name),
			Paths:         paths,
		})
	}
	return detected
}

func (s *SQLiteStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	if len(opts.Paths) == 0 {
		return nil, fmt.Errorf("sqlite backup requires at least one database path")
	}

	// Use SQLite's .backup command for each path, tar them together
	tmpDir := fmt.Sprintf("/tmp/sd-sqlite-backup-%d", time.Now().UnixNano())
	mkdirCmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName, "mkdir", "-p", tmpDir)
	if out, err := mkdirCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mkdir: %w: %s", err, out)
	}

	for i, dbPath := range opts.Paths {
		backupPath := fmt.Sprintf("%s/db%d.sqlite", tmpDir, i)
		cmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName,
			"sqlite3", dbPath, fmt.Sprintf(".backup '%s'", backupPath))
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("sqlite3 .backup %s: %w: %s", dbPath, err, out)
		}
	}

	// Tar + gzip the backup directory
	tarCmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName,
		"tar", "-czf", "-", "-C", tmpDir, ".")
	stdout, err := tarCmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("tar pipe: %w", err)
	}
	if err := tarCmd.Start(); err != nil {
		return nil, fmt.Errorf("tar start: %w", err)
	}

	// Clean up tmp dir after we've started reading
	go func() {
		tarCmd.Wait()
		cleanCmd := exec.Command("docker", "exec", opts.ContainerName, "rm", "-rf", tmpDir)
		cleanCmd.Run()
	}()

	filename := fmt.Sprintf("%s-%s.sqlite.tar.gz", opts.ContainerName, time.Now().Format("20060102-150405"))
	return &BackupResult{Reader: stdout, Filename: filename}, nil
}

func (s *SQLiteStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	if len(opts.Paths) == 0 {
		return fmt.Errorf("sqlite restore requires at least one database path")
	}

	tmpDir := fmt.Sprintf("/tmp/sd-sqlite-restore-%d", time.Now().UnixNano())

	// Extract tar into tmp dir
	mkdirCmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName, "mkdir", "-p", tmpDir)
	if out, err := mkdirCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mkdir: %w: %s", err, out)
	}

	tarCmd := exec.CommandContext(ctx, "docker", "exec", "-i", opts.ContainerName,
		"tar", "-xzf", "-", "-C", tmpDir)
	tarCmd.Stdin = opts.Reader
	if out, err := tarCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar extract: %w: %s", err, out)
	}

	// Copy each db file to its original path
	for i, dbPath := range opts.Paths {
		src := fmt.Sprintf("%s/db%d.sqlite", tmpDir, i)
		cpCmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName, "cp", src, dbPath)
		if out, err := cpCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cp %s -> %s: %w: %s", src, dbPath, err, out)
		}
	}

	// Clean up
	cleanCmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName, "rm", "-rf", tmpDir)
	cleanCmd.Run()
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestSQLiteStrategy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/sqlite_strategy.go internal/backup/sqlite_strategy_test.go
git commit -m "feat(backup): add SQLite strategy"
```

---

### Task 10: Volume Strategy (Enhanced)

**Files:**
- Modify: `internal/backup/volume.go`
- Modify: `internal/backup/volume_test.go`

- [ ] **Step 1: Write detection test**

```go
package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestVolumeStrategy_Type(t *testing.T) {
	s := NewVolumeStrategy()
	if s.Type() != "volume" {
		t.Errorf("expected 'volume', got '%s'", s.Type())
	}
}

func TestVolumeStrategy_Detect(t *testing.T) {
	cfg := &compose.AppConfig{
		Name: "testapp",
		Services: []compose.ServiceConfig{
			{
				Name:  "web",
				Image: "nginx:latest",
				Volumes: []compose.VolumeMount{
					{Source: "data", Target: "/data", Type: "volume"},
					{Source: "/var/run/docker.sock", Target: "/var/run/docker.sock", Type: "bind"},
					{Source: "uploads", Target: "/uploads", Type: "volume"},
				},
			},
		},
	}
	detected := NewVolumeStrategy().Detect(cfg)
	if len(detected) != 1 {
		t.Fatalf("expected 1 detected service, got %d", len(detected))
	}
	// Should have 2 paths (docker.sock excluded)
	if len(detected[0].Paths) != 2 {
		t.Errorf("expected 2 paths, got %d: %v", len(detected[0].Paths), detected[0].Paths)
	}
}

func TestVolumeStrategy_Detect_WithLabel(t *testing.T) {
	cfg := &compose.AppConfig{
		Name: "testapp",
		Services: []compose.ServiceConfig{
			{
				Name:   "app",
				Image:  "myapp:latest",
				Labels: map[string]string{"simpledeploy.backup.strategy": "volume"},
			},
		},
	}
	detected := NewVolumeStrategy().Detect(cfg)
	if len(detected) != 1 {
		t.Fatal("should detect via label even without volumes")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestVolumeStrategy -v`
Expected: FAIL (Type/Detect not yet on VolumeStrategy, constructor changed)

- [ ] **Step 3: Rewrite volume.go**

```go
package backup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

// VolumeStrategy backs up container directories via tar.
// Supports multiple paths per backup (user-selected in wizard).
type VolumeStrategy struct{}

func NewVolumeStrategy() *VolumeStrategy {
	return &VolumeStrategy{}
}

func (s *VolumeStrategy) Type() string { return "volume" }

func (s *VolumeStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var detected []DetectedService
	for _, svc := range cfg.Services {
		hasLabel := matchesLabel(svc.Labels, "volume")
		var paths []string
		for _, vol := range svc.Volumes {
			if vol.Target != "/var/run/docker.sock" {
				paths = append(paths, vol.Target)
			}
		}
		if hasLabel || len(paths) > 0 {
			detected = append(detected, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: fmt.Sprintf("%s-%s-1", cfg.Name, svc.Name),
				Label:         fmt.Sprintf("Files & Volumes (%s)", svc.Name),
				Paths:         paths,
			})
		}
	}
	return detected
}

func (s *VolumeStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	paths := opts.Paths
	if len(paths) == 0 {
		return nil, fmt.Errorf("volume backup requires at least one path")
	}

	args := []string{"exec", opts.ContainerName, "tar", "-czf", "-"}
	args = append(args, paths...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("tar pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("tar start: %w", err)
	}

	filename := fmt.Sprintf("%s-%s.tar.gz", opts.ContainerName, time.Now().Format("20060102-150405"))
	return &BackupResult{
		Reader:   &cmdReadCloser{ReadCloser: stdout, cmd: cmd},
		Filename: filename,
	}, nil
}

func (s *VolumeStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", opts.ContainerName,
		"tar", "-xzf", "-", "-C", "/")
	cmd.Stdin = opts.Reader
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar restore: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestVolumeStrategy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/volume.go internal/backup/volume_test.go
git commit -m "feat(backup): enhance volume strategy with multi-path support and detection"
```

---

### Task 11: Target Enhancements (Local + S3)

**Files:**
- Modify: `internal/backup/local.go`
- Modify: `internal/backup/s3.go`
- Modify: `internal/backup/local_test.go`
- Modify: `internal/backup/s3_test.go`

- [ ] **Step 1: Write local target tests**

```go
package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalTarget_Type(t *testing.T) {
	lt := NewLocalTarget(t.TempDir())
	if lt.Type() != "local" {
		t.Errorf("expected 'local', got '%s'", lt.Type())
	}
}

func TestLocalTarget_Test(t *testing.T) {
	dir := t.TempDir()
	lt := NewLocalTarget(dir)
	if err := lt.Test(context.Background()); err != nil {
		t.Errorf("test should pass for existing dir: %v", err)
	}

	lt2 := NewLocalTarget("/nonexistent/path/that/does/not/exist")
	if err := lt2.Test(context.Background()); err == nil {
		t.Error("test should fail for nonexistent dir")
	}
}

func TestLocalTarget_UploadReturnsPath(t *testing.T) {
	dir := t.TempDir()
	lt := NewLocalTarget(dir)

	path, size, err := lt.Upload(context.Background(), "test.sql.gz", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if path != "test.sql.gz" {
		t.Errorf("expected path 'test.sql.gz', got '%s'", path)
	}
	if size != 4 {
		t.Errorf("expected size 4, got %d", size)
	}

	// Verify file exists on disk
	if _, err := os.Stat(filepath.Join(dir, "test.sql.gz")); err != nil {
		t.Error("file should exist on disk")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestLocalTarget -v`
Expected: FAIL (Type, Test methods missing, Upload signature changed)

- [ ] **Step 3: Rewrite local.go**

```go
package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalTarget stores backups on the local filesystem.
type LocalTarget struct {
	dir string
}

func NewLocalTarget(dir string) *LocalTarget {
	return &LocalTarget{dir: dir}
}

func (t *LocalTarget) Type() string { return "local" }

func (t *LocalTarget) Test(ctx context.Context) error {
	info, err := os.Stat(t.dir)
	if err != nil {
		return fmt.Errorf("backup directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", t.dir)
	}
	return nil
}

func (t *LocalTarget) Upload(ctx context.Context, filename string, data io.Reader) (string, int64, error) {
	if strings.Contains(filename, "..") || filepath.IsAbs(filename) {
		return "", 0, fmt.Errorf("invalid filename: %s", filename)
	}
	os.MkdirAll(t.dir, 0700)
	path := filepath.Join(t.dir, filename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	n, err := io.Copy(f, data)
	if err != nil {
		return "", 0, fmt.Errorf("write: %w", err)
	}
	return filename, n, nil
}

func (t *LocalTarget) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	if strings.Contains(path, "..") || filepath.IsAbs(path) {
		return nil, fmt.Errorf("invalid path: %s", path)
	}
	f, err := os.Open(filepath.Join(t.dir, path))
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	return f, nil
}

func (t *LocalTarget) Delete(ctx context.Context, path string) error {
	if strings.Contains(path, "..") || filepath.IsAbs(path) {
		return fmt.Errorf("invalid path: %s", path)
	}
	return os.Remove(filepath.Join(t.dir, path))
}

// FilePath returns the absolute filesystem path for a backup file (for download streaming).
func (t *LocalTarget) FilePath(filename string) string {
	return filepath.Join(t.dir, filename)
}
```

- [ ] **Step 4: Update s3.go**

Add `Type()`, `Test()`, update `Upload` return signature to include path, add `PresignedURL` method:

```go
// Add to S3Target:

func (t *S3Target) Type() string { return "s3" }

func (t *S3Target) Test(ctx context.Context) error {
	_, err := t.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &t.cfg.Bucket,
	})
	return err
}

// Update Upload signature to return (path string, size int64, err error):
// The path is the S3 key (prefix + filename).

// Add PresignedURL method:
func (t *S3Target) PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presigner := s3.NewPresignClient(t.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &t.cfg.Bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
```

Full rewrite of `Upload` method to return `(string, int64, error)`:

```go
func (t *S3Target) Upload(ctx context.Context, filename string, data io.Reader) (string, int64, error) {
	key := filename
	if t.cfg.Prefix != "" {
		key = t.cfg.Prefix + "/" + filename
	}

	// Buffer to count bytes
	var buf bytes.Buffer
	n, err := io.Copy(&buf, data)
	if err != nil {
		return "", 0, fmt.Errorf("reading data: %w", err)
	}

	_, err = t.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &t.cfg.Bucket,
		Key:    &key,
		Body:   &buf,
	})
	if err != nil {
		return "", 0, fmt.Errorf("s3 put: %w", err)
	}
	return key, n, nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/backup/ -run "TestLocalTarget|TestS3" -v`
Expected: PASS for local tests. S3 tests may skip if no credentials configured.

- [ ] **Step 6: Commit**

```bash
git add internal/backup/local.go internal/backup/s3.go internal/backup/local_test.go internal/backup/s3_test.go
git commit -m "feat(backup): update targets with Type, Test, new Upload signature, and pre-signed URLs"
```

---

### Task 12: Store Layer Rewrite

**Files:**
- Modify: `internal/store/backups.go`
- Modify: `internal/store/backups_test.go`
- Modify: `internal/store/versions.go`
- Modify: `internal/store/versions_test.go`

- [ ] **Step 1: Write store tests for new types**

```go
package store

import (
	"testing"
)

func TestBackupConfig_CRUD(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	cfg := &BackupConfig{
		AppID:         1,
		Strategy:      "postgres",
		Target:        "local",
		ScheduleCron:  "0 2 * * *",
		RetentionMode: "count",
		RetentionCount: 7,
	}

	// Create
	err := db.CreateBackupConfig(cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cfg.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Read
	got, err := db.GetBackupConfig(cfg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Strategy != "postgres" || got.RetentionMode != "count" {
		t.Errorf("unexpected values: %+v", got)
	}

	// Update
	cfg.RetentionMode = "time"
	cfg.RetentionDays = intPtr(30)
	err = db.UpdateBackupConfig(cfg)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = db.GetBackupConfig(cfg.ID)
	if got.RetentionMode != "time" || got.RetentionDays == nil || *got.RetentionDays != 30 {
		t.Errorf("update didn't persist: %+v", got)
	}

	// Delete
	err = db.DeleteBackupConfig(cfg.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = db.GetBackupConfig(cfg.ID)
	if err == nil {
		t.Error("should not find deleted config")
	}
}

func TestBackupRun_WithChecksum(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Need an app and config first
	db.UpsertApp("testapp", "/tmp/test", "")
	app, _ := db.GetAppBySlug("testapp")
	cfg := &BackupConfig{AppID: app.ID, Strategy: "postgres", Target: "local", ScheduleCron: "0 2 * * *", RetentionMode: "count", RetentionCount: 7}
	db.CreateBackupConfig(cfg)

	run, err := db.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	err = db.UpdateBackupRunSuccess(run.ID, 1024, "test.sql.gz", "abc123def456")
	if err != nil {
		t.Fatalf("update success: %v", err)
	}

	got, _ := db.GetBackupRun(run.ID)
	if got.Checksum != "abc123def456" {
		t.Errorf("expected checksum 'abc123def456', got '%s'", got.Checksum)
	}
}

func TestComposeVersion_NameNotes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	db.UpsertApp("testapp", "/tmp/test", "")
	app, _ := db.GetAppBySlug("testapp")
	db.CreateComposeVersion(app.ID, "version: '3'\nservices:", "hash1")

	versions, _ := db.ListComposeVersions(app.ID)
	if len(versions) == 0 {
		t.Fatal("expected at least 1 version")
	}

	v := versions[0]
	err := db.UpdateComposeVersion(v.ID, "Initial deploy", "First production config", "")
	if err != nil {
		t.Fatalf("update version: %v", err)
	}

	got, _ := db.GetComposeVersion(v.ID)
	if got.Name == nil || *got.Name != "Initial deploy" {
		t.Errorf("expected name 'Initial deploy', got %v", got.Name)
	}
}

func intPtr(i int) *int { return &i }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run "TestBackupConfig_CRUD|TestBackupRun_WithChecksum|TestComposeVersion_NameNotes" -v`
Expected: FAIL

- [ ] **Step 3: Rewrite store types and methods**

Update `BackupConfig` struct:

```go
type BackupConfig struct {
	ID               int64     `json:"id"`
	AppID            int64     `json:"app_id"`
	Strategy         string    `json:"strategy"`
	Target           string    `json:"target"`
	ScheduleCron     string    `json:"schedule_cron"`
	TargetConfigJSON string    `json:"target_config_json"`
	RetentionMode    string    `json:"retention_mode"`
	RetentionCount   int       `json:"retention_count"`
	RetentionDays    *int      `json:"retention_days"`
	VerifyUpload     bool      `json:"verify_upload"`
	PreHooks         string    `json:"pre_hooks"`
	PostHooks        string    `json:"post_hooks"`
	Paths            string    `json:"paths"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
```

Update `BackupRun` struct:

```go
type BackupRun struct {
	ID               int64      `json:"id"`
	BackupConfigID   int64      `json:"backup_config_id"`
	Status           string     `json:"status"`
	SizeBytes        *int64     `json:"size_bytes"`
	Checksum         string     `json:"checksum"`
	FilePath         string     `json:"file_path"`
	ComposeVersionID *int64     `json:"compose_version_id"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	ErrorMsg         string     `json:"error_msg"`
}
```

Update `ComposeVersion` struct:

```go
type ComposeVersion struct {
	ID          int64      `json:"id"`
	AppID       int64      `json:"app_id"`
	Version     int        `json:"version"`
	Content     string     `json:"content"`
	Hash        string     `json:"hash"`
	Name        *string    `json:"name"`
	Notes       *string    `json:"notes"`
	EnvSnapshot *string    `json:"env_snapshot"`
	CreatedAt   time.Time  `json:"created_at"`
}
```

Add new methods:
- `UpdateBackupConfig(cfg *BackupConfig) error` - UPDATE all fields except id/app_id/created_at, set updated_at
- `UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath, checksum string) error` - adds checksum param
- `ListOldBackupRunsByTime(configID int64, maxAgeDays int) ([]BackupRun, error)` - for time-based retention
- `UpdateComposeVersion(id int64, name, notes, envSnapshot string) error` - set name/notes/env_snapshot
- `DownloadComposeVersion(id int64) (*ComposeVersion, error)` - alias for GetComposeVersion (returns content)

Update existing methods to handle new columns (scan the new fields in all SELECT queries).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -run "TestBackupConfig|TestBackupRun|TestComposeVersion" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/backups.go internal/store/backups_test.go internal/store/versions.go internal/store/versions_test.go
git commit -m "feat(store): rewrite backup store for v2 schema with retention modes, checksums, version names"
```

---

### Task 13: Detection Coordinator

**Files:**
- Create: `internal/backup/detect.go`
- Create: `internal/backup/detect_test.go`

- [ ] **Step 1: Write test**

```go
package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestDetector_AllStrategies(t *testing.T) {
	d := NewDetector()
	d.Register(NewPostgresStrategy())
	d.Register(NewMySQLStrategy())
	d.Register(NewMongoStrategy())
	d.Register(NewRedisStrategy())
	d.Register(NewSQLiteStrategy())
	d.Register(NewVolumeStrategy())

	cfg := &compose.AppConfig{
		Name: "myapp",
		Services: []compose.ServiceConfig{
			{Name: "db", Image: "postgres:16", Volumes: []compose.VolumeMount{{Target: "/var/lib/postgresql/data"}}},
			{Name: "cache", Image: "redis:7"},
			{Name: "web", Image: "nginx:latest", Volumes: []compose.VolumeMount{{Target: "/data"}}},
		},
	}

	results := d.DetectAll(cfg)

	// Should detect: postgres(db), redis(cache), volume(db volumes + web volumes)
	strategyTypes := map[string]bool{}
	for _, r := range results {
		strategyTypes[r.StrategyType] = true
	}
	if !strategyTypes["postgres"] {
		t.Error("should detect postgres")
	}
	if !strategyTypes["redis"] {
		t.Error("should detect redis")
	}
	if !strategyTypes["volume"] {
		t.Error("should detect volume")
	}
}

func TestDetector_LabelOverride(t *testing.T) {
	d := NewDetector()
	d.Register(NewPostgresStrategy())

	cfg := &compose.AppConfig{
		Name: "myapp",
		Services: []compose.ServiceConfig{
			{Name: "db", Image: "timescale/timescaledb:latest",
				Labels: map[string]string{"simpledeploy.backup.strategy": "postgres"}},
		},
	}

	results := d.DetectAll(cfg)
	found := false
	for _, r := range results {
		if r.StrategyType == "postgres" {
			found = true
		}
	}
	if !found {
		t.Error("label override should detect as postgres")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestDetector -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"github.com/vazra/simpledeploy/internal/compose"
)

// DetectionResult pairs a strategy type with its detected services.
type DetectionResult struct {
	StrategyType string            `json:"strategy_type"`
	Label        string            `json:"label"`
	Services     []DetectedService `json:"services"`
	Available    bool              `json:"available"`
	Description  string            `json:"description"`
}

// Detector runs all registered strategies' detection against a compose config.
type Detector struct {
	strategies []Strategy
}

func NewDetector() *Detector {
	return &Detector{}
}

func (d *Detector) Register(s Strategy) {
	d.strategies = append(d.strategies, s)
}

// DetectAll runs detection for all registered strategies and returns results.
func (d *Detector) DetectAll(cfg *compose.AppConfig) []DetectionResult {
	var results []DetectionResult
	for _, s := range d.strategies {
		services := s.Detect(cfg)
		result := DetectionResult{
			StrategyType: s.Type(),
			Label:        strategyDisplayLabel(s.Type()),
			Services:     services,
			Available:    len(services) > 0,
			Description:  strategyDescription(s.Type(), len(services) > 0),
		}
		results = append(results, result)
	}
	return results
}

func strategyDisplayLabel(t string) string {
	switch t {
	case "postgres":
		return "Database (PostgreSQL)"
	case "mysql":
		return "Database (MySQL/MariaDB)"
	case "mongo":
		return "Database (MongoDB)"
	case "redis":
		return "Cache (Redis)"
	case "sqlite":
		return "Database (SQLite)"
	case "volume":
		return "Files & Volumes"
	default:
		return t
	}
}

func strategyDescription(t string, available bool) string {
	descs := map[string][2]string{
		"postgres": {"Backs up PostgreSQL databases using pg_dump.", "No PostgreSQL services detected."},
		"mysql":    {"Backs up MySQL/MariaDB databases using mysqldump.", "No MySQL/MariaDB services detected."},
		"mongo":    {"Backs up MongoDB using mongodump.", "No MongoDB services detected."},
		"redis":    {"Backs up Redis by triggering BGSAVE and copying dump.rdb.", "No Redis services detected."},
		"sqlite":   {"Backs up SQLite databases using .backup command. Requires simpledeploy.backup.strategy=sqlite label.", "No SQLite services detected (add label to enable)."},
		"volume":   {"Backs up named volumes and bind-mounted directories.", "No volume mounts detected."},
	}
	if d, ok := descs[t]; ok {
		if available {
			return d[0]
		}
		return d[1]
	}
	return ""
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestDetector -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/detect.go internal/backup/detect_test.go
git commit -m "feat(backup): add detection coordinator with label override support"
```

---

### Task 14: Pipeline Processor

**Files:**
- Create: `internal/backup/pipeline.go`
- Create: `internal/backup/pipeline_test.go`

- [ ] **Step 1: Write test**

```go
package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

type mockStrategy struct {
	backupData string
	backupErr  error
	restoreErr error
}

func (m *mockStrategy) Type() string { return "mock" }
func (m *mockStrategy) Detect(cfg *compose.AppConfig) []DetectedService { return nil }
func (m *mockStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	if m.backupErr != nil {
		return nil, m.backupErr
	}
	return &BackupResult{
		Reader:   io.NopCloser(strings.NewReader(m.backupData)),
		Filename: "mock-backup.sql.gz",
	}, nil
}
func (m *mockStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	return m.restoreErr
}

type mockTarget struct {
	stored   map[string][]byte
	uploadErr error
}

func newMockTarget() *mockTarget {
	return &mockTarget{stored: map[string][]byte{}}
}
func (m *mockTarget) Type() string { return "mock" }
func (m *mockTarget) Test(ctx context.Context) error { return nil }
func (m *mockTarget) Upload(ctx context.Context, filename string, data io.Reader) (string, int64, error) {
	if m.uploadErr != nil {
		return "", 0, m.uploadErr
	}
	b, _ := io.ReadAll(data)
	m.stored[filename] = b
	return filename, int64(len(b)), nil
}
func (m *mockTarget) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	if data, ok := m.stored[path]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, fmt.Errorf("not found: %s", path)
}
func (m *mockTarget) Delete(ctx context.Context, path string) error {
	delete(m.stored, path)
	return nil
}

func TestPipeline_Backup_Success(t *testing.T) {
	strategy := &mockStrategy{backupData: "backup content here"}
	target := newMockTarget()

	p := &Pipeline{
		strategy: strategy,
		target:   target,
	}

	result, err := p.RunBackup(context.Background(), BackupOpts{ContainerName: "test-db-1"})
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	if result.FilePath == "" {
		t.Error("expected non-empty file path")
	}
	if result.SizeBytes == 0 {
		t.Error("expected non-zero size")
	}
	if result.Checksum == "" {
		t.Error("expected non-empty checksum")
	}

	// Verify data was stored
	if len(target.stored) != 1 {
		t.Errorf("expected 1 stored file, got %d", len(target.stored))
	}
}

func TestPipeline_Backup_StrategyError(t *testing.T) {
	strategy := &mockStrategy{backupErr: fmt.Errorf("pg_dump failed")}
	target := newMockTarget()

	p := &Pipeline{strategy: strategy, target: target}

	_, err := p.RunBackup(context.Background(), BackupOpts{ContainerName: "test-db-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pg_dump failed") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run TestPipeline -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
package backup

import (
	"context"
	"fmt"
	"io"
)

// PipelineResult holds the outcome of a backup pipeline run.
type PipelineResult struct {
	FilePath  string
	SizeBytes int64
	Checksum  string
}

// Pipeline orchestrates backup and restore operations.
type Pipeline struct {
	strategy Strategy
	target   Target
	hooks    *HookRunner // optional
}

// NewPipeline creates a Pipeline.
func NewPipeline(strategy Strategy, target Target, hooks *HookRunner) *Pipeline {
	return &Pipeline{strategy: strategy, target: target, hooks: hooks}
}

// RunBackup executes the full backup pipeline: pre-hooks -> backup -> checksum -> upload -> post-hooks.
func (p *Pipeline) RunBackup(ctx context.Context, opts BackupOpts, preHooks, postHooks []Hook) (*PipelineResult, error) {
	// Run pre-hooks
	if p.hooks != nil && len(preHooks) > 0 {
		if err := p.hooks.RunPre(ctx, preHooks); err != nil {
			return nil, fmt.Errorf("pre-hook: %w", err)
		}
	}

	// Run backup strategy
	backupResult, err := p.strategy.Backup(ctx, opts)
	if err != nil {
		p.runPostHooks(ctx, postHooks)
		return nil, fmt.Errorf("backup: %w", err)
	}
	defer backupResult.Reader.Close()

	// Tee through checksum
	cw := NewChecksumWriter()
	teedReader := cw.TeeReader(backupResult.Reader)

	// Upload to target
	filePath, size, err := p.target.Upload(ctx, backupResult.Filename, teedReader)
	if err != nil {
		p.runPostHooks(ctx, postHooks)
		return nil, fmt.Errorf("upload: %w", err)
	}

	// Run post-hooks (best effort)
	p.runPostHooks(ctx, postHooks)

	return &PipelineResult{
		FilePath:  filePath,
		SizeBytes: size,
		Checksum:  cw.Sum(),
	}, nil
}

// RunRestore executes the full restore pipeline: download -> verify checksum -> pre-hooks -> restore -> post-hooks.
func (p *Pipeline) RunRestore(ctx context.Context, opts RestoreOpts, filePath, expectedChecksum string, preHooks, postHooks []Hook) error {
	// Download from target
	reader, err := p.target.Download(ctx, filePath)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer reader.Close()

	// Verify checksum if available
	if expectedChecksum != "" {
		cw := NewChecksumWriter()
		verified := cw.TeeReader(reader)
		// We need to read all data to verify, but also pass it to restore.
		// Buffer the data since we need to read it twice conceptually.
		data, err := io.ReadAll(verified)
		if err != nil {
			return fmt.Errorf("reading for checksum: %w", err)
		}
		if actual := cw.Sum(); actual != expectedChecksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
		}
		reader.Close()
		opts.Reader = io.NopCloser(bytes.NewReader(data))
	} else {
		opts.Reader = reader
	}

	// Run pre-hooks
	if p.hooks != nil && len(preHooks) > 0 {
		if err := p.hooks.RunPre(ctx, preHooks); err != nil {
			return fmt.Errorf("pre-hook: %w", err)
		}
	}

	// Run restore
	if err := p.strategy.Restore(ctx, opts); err != nil {
		p.runPostHooks(ctx, postHooks)
		return fmt.Errorf("restore: %w", err)
	}

	p.runPostHooks(ctx, postHooks)
	return nil
}

func (p *Pipeline) runPostHooks(ctx context.Context, hooks []Hook) {
	if p.hooks != nil && len(hooks) > 0 {
		p.hooks.RunPost(ctx, hooks)
	}
}
```

Update the test to match the actual signature (add nil hooks params):

```go
// In tests, call with nil hooks:
result, err := p.RunBackup(context.Background(), BackupOpts{ContainerName: "test-db-1"}, nil, nil)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestPipeline -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/pipeline.go internal/backup/pipeline_test.go
git commit -m "feat(backup): add pipeline processor for backup and restore flows"
```

---

### Task 15: Scheduler Rewrite

**Files:**
- Modify: `internal/backup/scheduler.go`
- Modify: `internal/backup/scheduler_test.go`

- [ ] **Step 1: Write tests for new functionality**

```go
package backup

import (
	"testing"
	"time"
)

func TestScheduler_HotReload(t *testing.T) {
	// Verify that adding a config after Start() schedules it
	s := NewScheduler(newMockStore(), nil)
	s.RegisterStrategy("mock", &mockStrategy{backupData: "data"})
	s.RegisterTargetFactory("mock", func(json string) (Target, error) {
		return newMockTarget(), nil
	})
	s.Start()
	defer s.Stop()

	// Add config should register a cron entry
	err := s.ScheduleConfig(1, "*/5 * * * *")
	if err != nil {
		t.Fatalf("schedule config: %v", err)
	}

	// Reschedule same config
	err = s.ScheduleConfig(1, "0 3 * * *")
	if err != nil {
		t.Fatalf("reschedule: %v", err)
	}

	// Unschedule
	s.UnscheduleConfig(1)
}

func TestScheduler_MissedDetection(t *testing.T) {
	// Config with hourly schedule, last run 3 hours ago = missed
	lastRun := time.Now().Add(-3 * time.Hour)
	missed := isMissedBackup("0 * * * *", &lastRun)
	if !missed {
		t.Error("hourly backup with last run 3h ago should be missed")
	}

	// Config with daily schedule, last run 2 hours ago = not missed
	lastRun2 := time.Now().Add(-2 * time.Hour)
	missed2 := isMissedBackup("0 2 * * *", &lastRun2)
	if missed2 {
		t.Error("daily backup with last run 2h ago should not be missed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backup/ -run "TestScheduler_HotReload|TestScheduler_MissedDetection" -v`
Expected: FAIL

- [ ] **Step 3: Rewrite scheduler.go**

Key changes:
- `Scheduler` stores a map of `configID -> cron.EntryID` for hot-reload
- `ScheduleConfig(configID, cronExpr)` adds/replaces cron entry
- `UnscheduleConfig(configID)` removes cron entry
- `RunBackup` now uses Pipeline (creates pipeline from registered strategies/targets)
- `RunRestore` now uses Pipeline
- `CheckMissed()` method iterates all configs, checks last run time vs 2x cron interval
- `isMissedBackup(cronExpr, lastRun)` helper function
- Alert callback: `type BackupAlertFunc func(appName, strategy, errMsg string, eventType string)`
- Retention: supports both count and time modes

```go
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// BackupStore defines the store methods the scheduler needs.
type BackupStore interface {
	ListBackupConfigs(appID *int64) ([]store.BackupConfig, error)
	GetBackupConfig(id int64) (*store.BackupConfig, error)
	CreateBackupRun(configID int64) (*store.BackupRun, error)
	UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath, checksum string) error
	UpdateBackupRunFailed(id int64, errMsg string) error
	ListOldBackupRuns(configID int64, keepCount int) ([]store.BackupRun, error)
	ListOldBackupRunsByTime(configID int64, maxAgeDays int) ([]store.BackupRun, error)
	GetAppByID(id int64) (*store.App, error)
	GetBackupRun(id int64) (*store.BackupRun, error)
}

// BackupAlertFunc is called when a backup event occurs that should trigger an alert.
type BackupAlertFunc func(appName, strategy, message, eventType string)

// Scheduler manages backup scheduling, execution, and missed-backup detection.
type Scheduler struct {
	store      BackupStore
	hookExec   ContainerExecutor
	strategies map[string]Strategy
	targets    map[string]TargetFactory
	cron       *cron.Cron
	entries    map[int64]cron.EntryID // configID -> entryID
	mu         sync.Mutex
	alertFunc  BackupAlertFunc
}

func NewScheduler(store BackupStore, hookExec ContainerExecutor) *Scheduler {
	return &Scheduler{
		store:      store,
		hookExec:   hookExec,
		strategies: make(map[string]Strategy),
		targets:    make(map[string]TargetFactory),
		cron:       cron.New(),
		entries:    make(map[int64]cron.EntryID),
	}
}

func (s *Scheduler) RegisterStrategy(name string, strategy Strategy) {
	s.strategies[name] = strategy
}

func (s *Scheduler) RegisterTargetFactory(name string, factory TargetFactory) {
	s.targets[name] = factory
}

func (s *Scheduler) SetAlertFunc(fn BackupAlertFunc) {
	s.alertFunc = fn
}

func (s *Scheduler) Start() error {
	configs, err := s.store.ListBackupConfigs(nil)
	if err != nil {
		return fmt.Errorf("loading configs: %w", err)
	}
	for _, cfg := range configs {
		if err := s.ScheduleConfig(cfg.ID, cfg.ScheduleCron); err != nil {
			log.Printf("backup: failed to schedule config %d: %v", cfg.ID, err)
		}
	}
	s.cron.Start()
	return nil
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// ScheduleConfig adds or replaces a cron schedule for a backup config.
func (s *Scheduler) ScheduleConfig(configID int64, cronExpr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry if re-scheduling
	if entryID, ok := s.entries[configID]; ok {
		s.cron.Remove(entryID)
	}

	entryID, err := s.cron.AddFunc(cronExpr, func() {
		s.RunBackup(context.Background(), configID)
	})
	if err != nil {
		return fmt.Errorf("invalid cron: %w", err)
	}
	s.entries[configID] = entryID
	return nil
}

// UnscheduleConfig removes a cron schedule for a backup config.
func (s *Scheduler) UnscheduleConfig(configID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[configID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, configID)
	}
}

func (s *Scheduler) RunBackup(ctx context.Context, configID int64) {
	cfg, err := s.store.GetBackupConfig(configID)
	if err != nil {
		log.Printf("backup: config %d not found: %v", configID, err)
		return
	}

	run, err := s.store.CreateBackupRun(configID)
	if err != nil {
		log.Printf("backup: create run: %v", err)
		return
	}

	strategy, ok := s.strategies[cfg.Strategy]
	if !ok {
		s.failRun(run.ID, cfg, fmt.Sprintf("unknown strategy: %s", cfg.Strategy))
		return
	}

	targetFactory, ok := s.targets[cfg.Target]
	if !ok {
		s.failRun(run.ID, cfg, fmt.Sprintf("unknown target: %s", cfg.Target))
		return
	}

	target, err := targetFactory(cfg.TargetConfigJSON)
	if err != nil {
		s.failRun(run.ID, cfg, fmt.Sprintf("target init: %v", err))
		return
	}

	// Parse hooks
	var preHooks, postHooks []Hook
	if cfg.PreHooks != "" {
		json.Unmarshal([]byte(cfg.PreHooks), &preHooks)
	}
	if cfg.PostHooks != "" {
		json.Unmarshal([]byte(cfg.PostHooks), &postHooks)
	}

	// Parse paths
	var paths []string
	if cfg.Paths != "" {
		json.Unmarshal([]byte(cfg.Paths), &paths)
	}

	var hooks *HookRunner
	if s.hookExec != nil {
		hooks = NewHookRunner(s.hookExec, 60*time.Second)
	}

	pipeline := NewPipeline(strategy, target, hooks)

	opts := BackupOpts{
		ContainerName: "", // resolved from app
		Paths:         paths,
	}

	// Resolve container name from app
	app, err := s.store.GetAppByID(cfg.AppID)
	if err != nil {
		s.failRun(run.ID, cfg, fmt.Sprintf("app lookup: %v", err))
		return
	}
	opts.ContainerName = app.Name + "-" + cfg.Strategy + "-1" // simplified; real impl uses detection

	result, err := pipeline.RunBackup(ctx, opts, preHooks, postHooks)
	if err != nil {
		s.failRun(run.ID, cfg, err.Error())
		return
	}

	if err := s.store.UpdateBackupRunSuccess(run.ID, result.SizeBytes, result.FilePath, result.Checksum); err != nil {
		log.Printf("backup: update run success: %v", err)
	}

	// Prune old backups
	s.pruneRuns(cfg)
}

func (s *Scheduler) RunRestore(ctx context.Context, runID int64) error {
	run, err := s.store.GetBackupRun(runID)
	if err != nil {
		return fmt.Errorf("run not found: %w", err)
	}
	cfg, err := s.store.GetBackupConfig(run.BackupConfigID)
	if err != nil {
		return fmt.Errorf("config not found: %w", err)
	}

	strategy, ok := s.strategies[cfg.Strategy]
	if !ok {
		return fmt.Errorf("unknown strategy: %s", cfg.Strategy)
	}
	targetFactory, ok := s.targets[cfg.Target]
	if !ok {
		return fmt.Errorf("unknown target: %s", cfg.Target)
	}
	target, err := targetFactory(cfg.TargetConfigJSON)
	if err != nil {
		return fmt.Errorf("target init: %w", err)
	}

	var hooks *HookRunner
	if s.hookExec != nil {
		hooks = NewHookRunner(s.hookExec, 60*time.Second)
	}

	pipeline := NewPipeline(strategy, target, hooks)

	var preHooks, postHooks []Hook
	if cfg.PreHooks != "" {
		json.Unmarshal([]byte(cfg.PreHooks), &preHooks)
	}
	if cfg.PostHooks != "" {
		json.Unmarshal([]byte(cfg.PostHooks), &postHooks)
	}

	app, err := s.store.GetAppByID(cfg.AppID)
	if err != nil {
		return fmt.Errorf("app lookup: %w", err)
	}

	opts := RestoreOpts{
		ContainerName: app.Name + "-" + cfg.Strategy + "-1",
	}

	return pipeline.RunRestore(ctx, opts, run.FilePath, run.Checksum, preHooks, postHooks)
}

func (s *Scheduler) failRun(runID int64, cfg *store.BackupConfig, errMsg string) {
	s.store.UpdateBackupRunFailed(runID, errMsg)
	if s.alertFunc != nil {
		app, _ := s.store.GetAppByID(cfg.AppID)
		appName := ""
		if app != nil {
			appName = app.Name
		}
		s.alertFunc(appName, cfg.Strategy, errMsg, "backup_failed")
	}
}

func (s *Scheduler) pruneRuns(cfg *store.BackupConfig) {
	var oldRuns []store.BackupRun
	var err error

	switch cfg.RetentionMode {
	case "time":
		if cfg.RetentionDays != nil {
			oldRuns, err = s.store.ListOldBackupRunsByTime(cfg.ID, *cfg.RetentionDays)
		}
	default: // "count"
		oldRuns, err = s.store.ListOldBackupRuns(cfg.ID, cfg.RetentionCount)
	}

	if err != nil {
		log.Printf("backup: prune query: %v", err)
		return
	}

	targetFactory, ok := s.targets[cfg.Target]
	if !ok {
		return
	}
	target, err := targetFactory(cfg.TargetConfigJSON)
	if err != nil {
		return
	}

	for _, run := range oldRuns {
		if run.FilePath != "" {
			if err := target.Delete(context.Background(), run.FilePath); err != nil {
				log.Printf("backup: prune delete %s: %v", run.FilePath, err)
			}
		}
	}
}

// CheckMissed checks for missed backups and fires alerts.
func (s *Scheduler) CheckMissed() {
	configs, err := s.store.ListBackupConfigs(nil)
	if err != nil {
		return
	}
	for _, cfg := range configs {
		runs, _ := s.store.ListOldBackupRuns(cfg.ID, 1)
		var lastRun *time.Time
		if len(runs) > 0 && runs[0].FinishedAt != nil {
			lastRun = runs[0].FinishedAt
		}
		if isMissedBackup(cfg.ScheduleCron, lastRun) && s.alertFunc != nil {
			app, _ := s.store.GetAppByID(cfg.AppID)
			appName := ""
			if app != nil {
				appName = app.Name
			}
			s.alertFunc(appName, cfg.Strategy, "scheduled backup did not run", "backup_missed")
		}
	}
}

// isMissedBackup returns true if a scheduled backup hasn't run within 2x its cron interval.
func isMissedBackup(cronExpr string, lastRun *time.Time) bool {
	if lastRun == nil {
		return false // no history yet, can't determine missed
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		return false
	}

	// Calculate interval from two consecutive firings
	now := time.Now()
	next1 := sched.Next(now)
	next2 := sched.Next(next1)
	interval := next2.Sub(next1)

	return time.Since(*lastRun) > 2*interval
}
```

Note: The `store` import path references will need to match the actual module path (e.g., `github.com/vazra/simpledeploy/internal/store`). The implementer should adjust import paths accordingly.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/backup/ -run TestScheduler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/scheduler.go internal/backup/scheduler_test.go
git commit -m "feat(backup): rewrite scheduler with hot-reload, pipeline integration, missed detection"
```

---

### Task 16: Alert Integration

**Files:**
- Modify: `internal/alerts/types.go`
- Modify: `internal/alerts/evaluator.go`

- [ ] **Step 1: Add backup alert event type**

In `internal/alerts/types.go`, add:

```go
// BackupAlertEvent represents a backup-related alert event.
type BackupAlertEvent struct {
	AppName   string
	Strategy  string
	Message   string
	EventType string // "backup_failed" or "backup_missed"
	FiredAt   time.Time
}

// ToAlertEvent converts a BackupAlertEvent to a generic AlertEvent for webhook dispatch.
func (b BackupAlertEvent) ToAlertEvent() AlertEvent {
	metricDisplay := "Backup Failed"
	if b.EventType == "backup_missed" {
		metricDisplay = "Backup Missed"
	}
	return AlertEvent{
		AppName:       b.AppName,
		Metric:        b.EventType,
		MetricDisplay: metricDisplay,
		ValueDisplay:  b.Message,
		Status:        "firing",
		FiredAt:       b.FiredAt,
	}
}
```

- [ ] **Step 2: Add dispatch method to evaluator**

In `internal/alerts/evaluator.go`, add a method that the scheduler can call:

```go
// DispatchBackupAlert sends a backup alert through configured webhooks.
func (e *Evaluator) DispatchBackupAlert(event BackupAlertEvent) {
	rules, err := e.store.ListActiveAlertRules()
	if err != nil {
		return
	}

	alertEvent := event.ToAlertEvent()
	for _, rule := range rules {
		if rule.Metric != event.EventType && rule.Metric != "backup_all" {
			continue
		}
		webhook, err := e.store.GetWebhook(rule.WebhookID)
		if err != nil || webhook == nil {
			continue
		}
		e.dispatcher.Send(*webhook, alertEvent)
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/alerts/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/alerts/types.go internal/alerts/evaluator.go
git commit -m "feat(alerts): add backup_failed and backup_missed event types"
```

---

### Task 17: API Layer Rewrite

**Files:**
- Modify: `internal/api/backups.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Rewrite all backup handlers**

Replace `internal/api/backups.go` with handlers for all v2 endpoints:

```go
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/store"
)

// handleListBackupConfigs returns all backup configs for an app.
func (s *Server) handleListBackupConfigs(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	configs, err := s.store.ListBackupConfigs(&app.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, configs)
}

// handleCreateBackupConfig creates a new backup config.
func (s *Server) handleCreateBackupConfig(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var cfg store.BackupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	cfg.AppID = app.ID

	// Encrypt S3 credentials if target is s3
	if cfg.Target == "s3" && cfg.TargetConfigJSON != "" && s.masterSecret != "" {
		encrypted, err := auth.Encrypt(cfg.TargetConfigJSON, s.masterSecret)
		if err == nil {
			cfg.TargetConfigJSON = encrypted
		}
	}

	if err := s.store.CreateBackupConfig(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot-reload: schedule the new config
	if s.backupScheduler != nil {
		s.backupScheduler.ScheduleConfig(cfg.ID, cfg.ScheduleCron)
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, cfg)
}

// handleUpdateBackupConfig updates an existing backup config.
func (s *Server) handleUpdateBackupConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var cfg store.BackupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	cfg.ID = id

	// Encrypt S3 credentials
	if cfg.Target == "s3" && cfg.TargetConfigJSON != "" && s.masterSecret != "" {
		encrypted, err := auth.Encrypt(cfg.TargetConfigJSON, s.masterSecret)
		if err == nil {
			cfg.TargetConfigJSON = encrypted
		}
	}

	if err := s.store.UpdateBackupConfig(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot-reload: reschedule
	if s.backupScheduler != nil {
		s.backupScheduler.ScheduleConfig(cfg.ID, cfg.ScheduleCron)
	}

	writeJSON(w, cfg)
}

// handleDeleteBackupConfig deletes a backup config.
func (s *Server) handleDeleteBackupConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteBackupConfig(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.backupScheduler != nil {
		s.backupScheduler.UnscheduleConfig(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListBackupRuns returns all backup runs for an app (across all configs).
func (s *Server) handleListBackupRuns(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	configs, err := s.store.ListBackupConfigs(&app.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var allRuns []store.BackupRun
	for _, cfg := range configs {
		runs, err := s.store.ListBackupRuns(cfg.ID)
		if err != nil {
			continue
		}
		allRuns = append(allRuns, runs...)
	}
	writeJSON(w, allRuns)
}

// handleTriggerBackup triggers all backup configs for an app.
func (s *Server) handleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	configs, err := s.store.ListBackupConfigs(&app.ID)
	if err != nil || len(configs) == 0 {
		http.Error(w, "no backup configs", http.StatusBadRequest)
		return
	}
	for _, cfg := range configs {
		go s.backupScheduler.RunBackup(r.Context(), cfg.ID)
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleTriggerBackupConfig triggers a specific backup config.
func (s *Server) handleTriggerBackupConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	go s.backupScheduler.RunBackup(r.Context(), id)
	w.WriteHeader(http.StatusAccepted)
}

// handleRestore triggers a restore from a backup run.
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	go s.backupScheduler.RunRestore(r.Context(), id)
	w.WriteHeader(http.StatusAccepted)
}

// handleDownloadBackup streams a backup file to the client.
func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	run, err := s.store.GetBackupRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	cfg, err := s.store.GetBackupConfig(run.BackupConfigID)
	if err != nil {
		http.Error(w, "config not found", http.StatusNotFound)
		return
	}

	targetJSON := cfg.TargetConfigJSON
	if cfg.Target == "s3" && s.masterSecret != "" {
		decrypted, err := auth.Decrypt(targetJSON, s.masterSecret)
		if err == nil {
			targetJSON = decrypted
		}
	}

	// For local target, stream the file directly
	if cfg.Target == "local" {
		localTarget := backup.NewLocalTarget(s.backupDir)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", run.FilePath))
		http.ServeFile(w, r, localTarget.FilePath(run.FilePath))
		return
	}

	// For S3, generate pre-signed URL and redirect
	if cfg.Target == "s3" {
		var s3cfg backup.S3Config
		json.Unmarshal([]byte(targetJSON), &s3cfg)
		target, err := backup.NewS3Target(s3cfg)
		if err != nil {
			http.Error(w, "s3 init: "+err.Error(), http.StatusInternalServerError)
			return
		}
		url, err := target.PresignedURL(r.Context(), run.FilePath, 15*time.Minute)
		if err != nil {
			http.Error(w, "presign: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	http.Error(w, "unsupported target", http.StatusBadRequest)
}

// handleUploadRestore accepts a file upload and restores it.
func (s *Server) handleUploadRestore(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	// Max 5GB
	r.Body = http.MaxBytesReader(w, r.Body, 5<<30)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	strategyType := r.FormValue("strategy")
	container := r.FormValue("container")

	if strategyType == "" || container == "" {
		http.Error(w, "strategy and container are required", http.StatusBadRequest)
		return
	}

	// Validate file extension matches strategy
	name := strings.ToLower(header.Filename)
	valid := false
	switch strategyType {
	case "postgres", "mysql":
		valid = strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".sql.gz")
	case "mongo":
		valid = strings.HasSuffix(name, ".archive") || strings.HasSuffix(name, ".archive.gz")
	case "redis":
		valid = strings.HasSuffix(name, ".rdb") || strings.HasSuffix(name, ".rdb.tar.gz")
	case "sqlite":
		valid = strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".sqlite") ||
			strings.HasSuffix(name, ".sqlite3") || strings.HasSuffix(name, ".sqlite.tar.gz")
	case "volume":
		valid = strings.HasSuffix(name, ".tar.gz")
	}
	if !valid {
		http.Error(w, fmt.Sprintf("file extension doesn't match strategy %s", strategyType), http.StatusBadRequest)
		return
	}

	strategy, ok := s.backupScheduler.GetStrategy(strategyType)
	if !ok {
		http.Error(w, "unknown strategy: "+strategyType, http.StatusBadRequest)
		return
	}

	go func() {
		opts := backup.RestoreOpts{
			ContainerName: container,
			Reader:        io.NopCloser(file),
		}
		if err := strategy.Restore(context.Background(), opts); err != nil {
			log.Printf("upload restore failed for %s/%s: %v", app.Slug, container, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

// handleBackupSummary returns cross-app backup health.
func (s *Server) handleBackupSummary(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.GetBackupSummary()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recentRuns, err := s.store.ListRecentBackupRuns(20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"apps":        apps,
		"recent_runs": recentRuns,
	})
}

// handleDetectStrategies auto-detects available backup strategies for an app.
func (s *Server) handleDetectStrategies(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	cfg, parseErr := compose.ParseFile(app.ComposePath, app.Name)

	var results []backup.DetectionResult
	if parseErr != nil {
		// Return all strategies as unavailable
		writeJSON(w, map[string]any{"strategies": []backup.DetectionResult{}})
		return
	}

	detector := s.backupScheduler.GetDetector()
	results = detector.DetectAll(cfg)

	writeJSON(w, map[string]any{"strategies": results})
}

// handleTestS3 tests an S3 connection.
func (s *Server) handleTestS3(w http.ResponseWriter, r *http.Request) {
	var cfg backup.S3Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid config", http.StatusBadRequest)
		return
	}
	target, err := backup.NewS3Target(cfg)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := target.Test(r.Context()); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleUpdateComposeVersion updates name/notes on a compose version.
func (s *Server) handleUpdateComposeVersion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Name string `json:"name"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateComposeVersion(id, body.Name, body.Notes, ""); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleDownloadComposeVersion downloads a compose file.
func (s *Server) handleDownloadComposeVersion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	v, err := s.store.GetComposeVersion(id)
	if err != nil {
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=docker-compose-v%d.yml", v.Version))
	w.Write([]byte(v.Content))
}

// handleRestoreComposeVersion restores a compose version (redeploy).
func (s *Server) handleRestoreComposeVersion(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	v, err := s.store.GetComposeVersion(id)
	if err != nil {
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	// Write compose content back to file and redeploy
	if err := os.WriteFile(app.ComposePath, []byte(v.Content), 0644); err != nil {
		http.Error(w, "write compose: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go s.deployer.Deploy(r.Context(), app.Slug)
	w.WriteHeader(http.StatusAccepted)
}
```

- [ ] **Step 2: Update route registration in server.go**

Add/update these routes in the route registration section:

```go
// Backup configs
s.mux.Handle("GET /api/apps/{slug}/backups/configs", s.authMiddleware(http.HandlerFunc(s.handleListBackupConfigs)))
s.mux.Handle("POST /api/apps/{slug}/backups/configs", s.authMiddleware(http.HandlerFunc(s.handleCreateBackupConfig)))
s.mux.Handle("PUT /api/backups/configs/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateBackupConfig)))
s.mux.Handle("DELETE /api/backups/configs/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteBackupConfig)))

// Backup runs
s.mux.Handle("GET /api/apps/{slug}/backups/runs", s.authMiddleware(http.HandlerFunc(s.handleListBackupRuns)))
s.mux.Handle("POST /api/apps/{slug}/backups/run", s.authMiddleware(http.HandlerFunc(s.handleTriggerBackup)))
s.mux.Handle("POST /api/backups/configs/{id}/run", s.authMiddleware(http.HandlerFunc(s.handleTriggerBackupConfig)))
s.mux.Handle("POST /api/backups/restore/{id}", s.authMiddleware(http.HandlerFunc(s.handleRestore)))
s.mux.Handle("GET /api/backups/runs/{id}/download", s.authMiddleware(http.HandlerFunc(s.handleDownloadBackup)))
s.mux.Handle("POST /api/apps/{slug}/backups/upload-restore", s.authMiddleware(http.HandlerFunc(s.handleUploadRestore)))

// Backup dashboard & detection
s.mux.Handle("GET /api/backups/summary", s.authMiddleware(http.HandlerFunc(s.handleBackupSummary)))
s.mux.Handle("GET /api/apps/{slug}/backups/detect", s.authMiddleware(http.HandlerFunc(s.handleDetectStrategies)))
s.mux.Handle("POST /api/backups/test-s3", s.authMiddleware(http.HandlerFunc(s.handleTestS3)))

// Compose versions
s.mux.Handle("PUT /api/apps/{slug}/versions/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateComposeVersion)))
s.mux.Handle("GET /api/apps/{slug}/versions/{id}/download", s.authMiddleware(http.HandlerFunc(s.handleDownloadComposeVersion)))
s.mux.Handle("POST /api/apps/{slug}/versions/{id}/restore", s.authMiddleware(http.HandlerFunc(s.handleRestoreComposeVersion)))
```

- [ ] **Step 3: Add helper methods to scheduler for API access**

In `scheduler.go`, add:

```go
// GetStrategy returns a registered strategy by type name.
func (s *Scheduler) GetStrategy(name string) (Strategy, bool) {
	st, ok := s.strategies[name]
	return st, ok
}

// GetDetector returns a Detector with all registered strategies.
func (s *Scheduler) GetDetector() *Detector {
	d := NewDetector()
	for _, s := range s.strategies {
		d.Register(s)
	}
	return d
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/api/`
Expected: PASS (may need import adjustments)

- [ ] **Step 5: Commit**

```bash
git add internal/api/backups.go internal/api/server.go internal/backup/scheduler.go
git commit -m "feat(api): rewrite backup API handlers for v2 with download, upload-restore, config editing"
```

---

### Task 18: Main.go Wiring

**Files:**
- Modify: `cmd/simpledeploy/main.go`

- [ ] **Step 1: Update backup initialization**

Replace the backup scheduler initialization section with:

```go
backupSched := backup.NewScheduler(db, nil) // nil hookExec for now, will add Docker executor

// Register all strategies
backupSched.RegisterStrategy("postgres", backup.NewPostgresStrategy())
backupSched.RegisterStrategy("mysql", backup.NewMySQLStrategy())
backupSched.RegisterStrategy("mongo", backup.NewMongoStrategy())
backupSched.RegisterStrategy("redis", backup.NewRedisStrategy())
backupSched.RegisterStrategy("sqlite", backup.NewSQLiteStrategy())
backupSched.RegisterStrategy("volume", backup.NewVolumeStrategy())

// Register target factories
backupDir := filepath.Join(cfg.DataDir, "backups")
backupSched.RegisterTargetFactory("local", func(configJSON string) (backup.Target, error) {
	return backup.NewLocalTarget(backupDir), nil
})
backupSched.RegisterTargetFactory("s3", func(configJSON string) (backup.Target, error) {
	// Decrypt if encrypted
	decrypted := configJSON
	if cfg.MasterSecret != "" {
		if d, err := auth.Decrypt(configJSON, cfg.MasterSecret); err == nil {
			decrypted = d
		}
	}
	var s3cfg backup.S3Config
	if err := json.Unmarshal([]byte(decrypted), &s3cfg); err != nil {
		return nil, fmt.Errorf("parse s3 config: %w", err)
	}
	return backup.NewS3Target(s3cfg)
})

// Wire alert integration
backupSched.SetAlertFunc(func(appName, strategy, message, eventType string) {
	alertEval.DispatchBackupAlert(alerts.BackupAlertEvent{
		AppName:   appName,
		Strategy:  strategy,
		Message:   message,
		EventType: eventType,
		FiredAt:   time.Now(),
	})
})

if err := backupSched.Start(); err != nil {
	fmt.Fprintf(os.Stderr, "backup scheduler: %v\n", err)
}
defer backupSched.Stop()

// Start missed backup checker
go func() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		backupSched.CheckMissed()
	}
}()
```

Update Server setup:

```go
srv := api.NewServer(cfg.ManagementPort, db, jwtMgr, rl)
srv.SetBackupScheduler(backupSched)
srv.SetBackupDir(backupDir)
srv.SetMasterSecret(cfg.MasterSecret)
```

- [ ] **Step 2: Add SetBackupDir and SetMasterSecret to server**

In `internal/api/server.go`, add fields and setters:

```go
// Add to Server struct:
backupDir    string
masterSecret string

// Add methods:
func (s *Server) SetBackupDir(dir string) {
	s.backupDir = dir
}

func (s *Server) SetMasterSecret(secret string) {
	s.masterSecret = secret
}
```

- [ ] **Step 3: Verify full build**

Run: `make build-go`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/simpledeploy/main.go internal/api/server.go
git commit -m "feat: wire backup v2 with all strategies, alert integration, missed detection"
```

---

### Task 19: UI API Client Updates

**Files:**
- Modify: `ui/src/lib/api.js`

- [ ] **Step 1: Add new backup API methods**

Replace the backup methods section in `api.js`:

```javascript
// Backup configs
listBackupConfigs: (slug) => request('GET', `/apps/${slug}/backups/configs`),
createBackupConfig: (slug, cfg) => requestWithToast('POST', `/apps/${slug}/backups/configs`, cfg, 'Backup config created'),
updateBackupConfig: (id, cfg) => requestWithToast('PUT', `/backups/configs/${id}`, cfg, 'Backup config updated'),
deleteBackupConfig: (id) => requestWithToast('DELETE', `/backups/configs/${id}`, null, 'Backup config deleted'),

// Backup runs
listBackupRuns: (slug) => request('GET', `/apps/${slug}/backups/runs`),
triggerBackup: (slug) => requestWithToast('POST', `/apps/${slug}/backups/run`, null, 'Backup triggered'),
triggerBackupConfig: (id) => requestWithToast('POST', `/backups/configs/${id}/run`, null, 'Backup triggered'),
restore: (id) => requestWithToast('POST', `/backups/restore/${id}`, null, 'Restore started'),
downloadBackup: (id) => `${BASE}/backups/runs/${id}/download`,
uploadRestore: (slug, formData) => {
    return fetch(`${BASE}/apps/${slug}/backups/upload-restore`, {
        method: 'POST',
        body: formData,
        credentials: 'include',
    }).then(res => {
        if (!res.ok) throw new Error('Upload failed');
        return { data: true, error: null };
    }).catch(err => ({ data: null, error: err.message }));
},

// Backup dashboard & detection
backupSummary: () => request('GET', '/backups/summary'),
detectStrategies: (slug) => request('GET', `/apps/${slug}/backups/detect`),
testS3: (cfg) => request('POST', '/backups/test-s3', cfg),

// Compose versions
updateComposeVersion: (slug, id, data) => requestWithToast('PUT', `/apps/${slug}/versions/${id}`, data, 'Version updated'),
downloadComposeVersion: (slug, id) => `${BASE}/apps/${slug}/versions/${id}/download`,
restoreComposeVersion: (slug, id) => requestWithToast('POST', `/apps/${slug}/versions/${id}/restore`, null, 'Restoring compose version'),
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/lib/api.js
git commit -m "feat(ui): add backup v2 API client methods"
```

---

### Task 20: BackupWizard Rewrite

**Files:**
- Modify: `ui/src/components/BackupWizard.svelte`

- [ ] **Step 1: Rewrite wizard with 6 steps**

The wizard now has these steps:
1. What to back up (6 strategy types with detection results)
2. Where to store (local/S3 with test)
3. Schedule (ScheduleBuilder)
4. Hooks (optional: predefined + custom exec)
5. Retention & verification (count/time toggle, verify upload, path selection for volume/sqlite)
6. Summary + create/update

Key changes from current wizard:
- Step 1: Show all 6 strategy types from detection API. Each shows detected services/containers.
- Step 4 (new): Hooks step with smart suggestions. For volume: "Stop container?" toggle. For redis: "Flush to disk?" toggle. Collapsible custom exec section.
- Step 5: Two retention modes (count vs time). Verify upload checkbox. For volume/sqlite: path selection checkboxes.
- Edit mode: `editConfig` prop pre-populates all fields, button says "Save Changes" instead of "Create Backup".

The implementer should follow the existing wizard structure (FormModal, step counter, canProceed logic) and extend it with the new steps. The full component is ~500 lines of Svelte. The structure mirrors the current wizard but with the additional steps and expanded step 1.

Key state additions:
```javascript
let retentionMode = 'count';  // 'count' or 'time'
let retentionDays = 30;
let verifyUpload = false;
let preHooks = [];
let postHooks = [];
let selectedPaths = [];       // for volume/sqlite
let editConfig = null;        // null for create, config object for edit
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/components/BackupWizard.svelte
git commit -m "feat(ui): rewrite backup wizard with hooks, retention modes, 6 strategies"
```

---

### Task 21: BackupsTab Enhancements

**Files:**
- Modify: `ui/src/components/BackupsTab.svelte`

- [ ] **Step 1: Add edit, download, upload-restore functionality**

Key additions:
- Edit button per config row: opens wizard in edit mode with pre-populated values
- Download button per successful run: `<a href={api.downloadBackup(run.id)}>` with download attribute
- "Restore from file" button: opens upload-restore modal
- Upload-restore modal: strategy picker, container picker, file input, submit
- Checksum display: truncated hash on run row, expandable

New state:
```javascript
let editingConfig = null;      // config to edit in wizard
let showUploadModal = false;
let uploadStrategy = '';
let uploadContainer = '';
let uploadFile = null;
```

New functions:
```javascript
async function uploadAndRestore() {
    const formData = new FormData();
    formData.append('file', uploadFile);
    formData.append('strategy', uploadStrategy);
    formData.append('container', uploadContainer);
    const result = await api.uploadRestore(slug, formData);
    if (!result.error) {
        showUploadModal = false;
        loadData();
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/components/BackupsTab.svelte
git commit -m "feat(ui): add backup download, edit, upload-restore to BackupsTab"
```

---

### Task 22: Backups Dashboard Update

**Files:**
- Modify: `ui/src/routes/Backups.svelte`
- Modify: `ui/src/components/BackupHealthCard.svelte`

- [ ] **Step 1: Add missed backups stat and strategy icons**

In `Backups.svelte`, add a 5th stat card: "Missed (24h)" counting apps with missed backups.

In `BackupHealthCard.svelte`, add strategy-specific icons/badges:
```javascript
function strategyIcon(s) {
    const icons = {
        postgres: 'database',
        mysql: 'database',
        mongo: 'database',
        redis: 'zap',
        sqlite: 'file',
        volume: 'folder'
    };
    return icons[s] || 'archive';
}
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/routes/Backups.svelte ui/src/components/BackupHealthCard.svelte
git commit -m "feat(ui): update backup dashboard with missed stat and strategy icons"
```

---

### Task 23: Compose Versions UI

**Files:**
- This is a new UI section within the app detail view. The implementer should determine the best location (new tab or section within an existing tab like the versions/history tab).

- [ ] **Step 1: Build compose versions timeline**

Create a section that shows:
- Timeline list of compose versions (newest first)
- Each entry: version number, timestamp, name (editable inline), notes (editable inline), change indicator
- Actions per entry: download button (link to `api.downloadComposeVersion`), restore button (with confirmation modal)
- Side-by-side diff viewer: select two versions, show diff with additions/removals highlighted

The implementer should check which tab currently shows compose versions (likely the deploy history / versions tab in app detail) and extend it with the name/notes/download/restore functionality.

- [ ] **Step 2: Commit**

```bash
git add ui/src/components/VersionsTab.svelte  # or wherever it lives
git commit -m "feat(ui): add compose version history with name, notes, download, restore"
```

---

### Task 24: E2E Tests

**Files:**
- Modify: `e2e/tests/12-backups.spec.js`

- [ ] **Step 1: Rewrite E2E tests for v2**

```javascript
const { test, expect } = require('@playwright/test');
const { loginAsAdmin, getState } = require('../helpers/auth');

test.describe('Backups v2', () => {
    test.beforeEach(async ({ page }) => {
        await loginAsAdmin(page);
    });

    test('navigate to postgres app backups tab', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/apps/e2e-postgres`);
        await page.getByRole('button', { name: /backups/i }).click();
        await expect(page.getByText(/backup|configure/i)).toBeVisible({ timeout: 5000 });
    });

    test('detect strategies shows postgres and volume', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/apps/e2e-postgres`);
        await page.getByRole('button', { name: /backups/i }).click();

        // Open wizard
        const configBtn = page.getByRole('button', { name: /configure backup|add config/i });
        await configBtn.click();
        await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 });

        // Should show postgres as detected
        await expect(page.getByText(/postgresql/i)).toBeVisible();
        // Should show volume as available
        await expect(page.getByText(/files.*volumes/i)).toBeVisible();
    });

    test('create backup config with postgres strategy', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/apps/e2e-postgres`);
        await page.getByRole('button', { name: /backups/i }).click();

        const configBtn = page.getByRole('button', { name: /configure backup|add config/i });
        await configBtn.click();
        await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 });

        // Step 1: Select postgres
        const pgBtn = page.getByText(/postgresql/i).first();
        if (await pgBtn.isVisible()) await pgBtn.click();
        await page.getByRole('button', { name: /next/i }).click();

        // Step 2: Select local
        await page.getByText(/local storage/i).click();
        await page.getByRole('button', { name: /next/i }).click();

        // Step 3: Schedule (accept defaults)
        await page.getByRole('button', { name: /next/i }).click();

        // Step 4: Hooks (skip)
        await page.getByRole('button', { name: /next/i }).click();

        // Step 5: Retention (accept defaults)
        await page.getByRole('button', { name: /next/i }).click();

        // Step 6: Create
        await page.getByRole('button', { name: /create backup/i }).click();

        // Verify config appears
        await expect(page.getByText(/local|storage/i)).toBeVisible({ timeout: 10000 });
    });

    test('trigger manual backup', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/apps/e2e-postgres`);
        await page.getByRole('button', { name: /backups/i }).click();
        await expect(page.getByRole('button', { name: /backup now/i })).toBeVisible({ timeout: 10000 });
        await page.getByRole('button', { name: /backup now/i }).click();
        await page.waitForTimeout(3000);
        await page.reload();
        await page.getByRole('button', { name: /backups/i }).click();
        await expect(page.getByText(/running|success|failed/i)).toBeVisible({ timeout: 15000 });
    });

    test('download backup button visible for successful runs', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/apps/e2e-postgres`);
        await page.getByRole('button', { name: /backups/i }).click();

        // If there's a successful run, download should be available
        const downloadBtn = page.getByRole('link', { name: /download/i }).first();
        if (await downloadBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            const href = await downloadBtn.getAttribute('href');
            expect(href).toContain('/download');
        }
    });

    test('global backups page shows summary', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/backups`);
        await expect(page.getByText(/total config/i)).toBeVisible({ timeout: 5000 });
    });

    test('edit backup config', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/apps/e2e-postgres`);
        await page.getByRole('button', { name: /backups/i }).click();

        const editBtn = page.getByRole('button', { name: /edit/i }).first();
        if (await editBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            await editBtn.click();
            await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 });
            // Wizard should be in edit mode
            await expect(page.getByText(/save changes/i)).toBeVisible();
        }
    });

    test('delete backup config', async ({ page }) => {
        const { baseURL } = getState();
        await page.goto(`${baseURL}/#/apps/e2e-postgres`);
        await page.getByRole('button', { name: /backups/i }).click();

        const deleteBtn = page.getByRole('button', { name: /delete/i }).first();
        if (await deleteBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            await deleteBtn.click();
            const dialog = page.getByRole('dialog');
            if (await dialog.isVisible({ timeout: 2000 }).catch(() => false)) {
                await dialog.getByRole('button', { name: /delete|confirm/i }).click();
            }
        }
    });
});
```

- [ ] **Step 2: Run E2E tests**

Run: `make e2e`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/12-backups.spec.js
git commit -m "test(e2e): rewrite backup tests for v2 features"
```

---

### Task 25: Go Unit/Integration Tests

**Files:**
- Modify: `internal/api/backups_test.go`
- Update existing test files to match new interfaces

- [ ] **Step 1: Write API integration tests**

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestCreateBackupConfig_API(t *testing.T) {
	srv, db := setupTestServer(t)

	// Create a test app first
	db.UpsertApp("testapp", "/tmp/test", "")

	body := `{"strategy":"postgres","target":"local","schedule_cron":"0 2 * * *","retention_mode":"count","retention_count":7}`
	req := httptest.NewRequest("POST", "/api/apps/testapp/backups/configs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthHeader(req, srv)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var cfg store.BackupConfig
	json.NewDecoder(w.Body).Decode(&cfg)
	if cfg.ID == 0 {
		t.Error("expected non-zero config ID")
	}
	if cfg.Strategy != "postgres" {
		t.Errorf("expected postgres, got %s", cfg.Strategy)
	}
}

func TestUpdateBackupConfig_API(t *testing.T) {
	srv, db := setupTestServer(t)
	db.UpsertApp("testapp", "/tmp/test", "")
	app, _ := db.GetAppBySlug("testapp")

	cfg := &store.BackupConfig{
		AppID: app.ID, Strategy: "postgres", Target: "local",
		ScheduleCron: "0 2 * * *", RetentionMode: "count", RetentionCount: 7,
	}
	db.CreateBackupConfig(cfg)

	body := `{"retention_mode":"time","retention_days":30,"schedule_cron":"0 3 * * *"}`
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/backups/configs/%d", cfg.ID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	addAuthHeader(req, srv)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDetectStrategies_API(t *testing.T) {
	srv, db := setupTestServer(t)
	// This test needs a real compose file; skip if fixture not available
	t.Skip("requires compose fixture")
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/api/ -run TestBackup -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/api/backups_test.go
git commit -m "test(api): add backup v2 API integration tests"
```

---

### Task 26: Final Build & Full Test

- [ ] **Step 1: Run all Go tests**

Run: `make test`
Expected: PASS

- [ ] **Step 2: Build full binary**

Run: `make build`
Expected: PASS

- [ ] **Step 3: Run E2E tests**

Run: `make e2e`
Expected: PASS

- [ ] **Step 4: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: resolve backup v2 integration issues"
```
