# Filesystem-authoritative state Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make per-app `simpledeploy.yml` + `simpledeploy.secrets.yml` and global `{data_dir}/config.yml` + `secrets.yml` the source of truth for declarative state. DB becomes a fast cache rebuilt from FS on startup and on watcher events. Mutations write FS first, then DB, in one logical transaction.

**Architecture:** Extend `internal/configsync` from "DB → FS mirror" to a bidirectional engine. Add a secrets file type. Extend the reconciler watcher to cover sidecar files. Add a one-time migration + boot-time seed step. Per-resource API handlers switch to FS-first writes through a small set of `Apply*` helpers.

**Tech Stack:** Go (configsync, store, api, reconciler, audit, gitsync), SQLite migration, YAML (gopkg.in/yaml.v3), AES-GCM via `internal/auth.Encrypt/Decrypt`, fsnotify, Svelte (none — UI continues reading from existing API).

**Spec:** `docs/superpowers/specs/2026-04-27-fs-authoritative-state-design.md`

**Dependency:** This plan assumes `2026-04-27-archive-instead-of-delete.md` has been merged. Tombstones at `{data_dir}/archive/<slug>.yml` are already produced by the reconciler. Task 12 below extends the FS-→DB reload path to import tombstones as archived rows.

**Migration number:** #23 (#22 reserved for the archive plan).

---

## Resolved open questions

- **Backup config correlation key:** introduce `id string` (UUIDv4) on `BackupConfigEntry` and the matching `BackupSecretsEntry`. Stored in both `simpledeploy.yml` and `simpledeploy.secrets.yml`. DB column `backup_configs.uuid TEXT UNIQUE NOT NULL` (migration #24, see Task 1). Auto-generated for existing rows during the seed step.
- **macOS Docker Desktop dev watcher caveat:** documentation only, in `docs/operations/state-on-disk.md`. No code change.
- **gitsync redundancy:** `RedactedGlobalSidecar` stays. Add `// DEPRECATED: redundant with FS-authoritative config.yml; remove once gitsync is migrated to push config.yml directly.` above the type and write-callsite. Follow-up tracked in code TODO with cross-reference.

---

## Parallelism guide

- **Phase A (parallel):** 1, 2, 3
- **Phase B (parallel):** 4, 5, 6
- **Phase C:** 7 (depends on A+B)
- **Phase D (parallel):** 8, 9
- **Phase E (parallel):** 10, 11
- **Phase F:** 12 (forward-compat with archive plan)
- **Phase G:** 13, 14
- **Phase H:** 15

---

## Task 1 (Phase A): Migrations and store schema additions

**Files:**
- Create: `internal/store/migrations/023_system_meta.sql`
- Create: `internal/store/migrations/024_backup_configs_uuid.sql`
- Modify: `internal/store/system_meta.go` (new) and `internal/store/backups.go` (or wherever BackupConfig is)
- Test: `internal/store/system_meta_test.go`, `internal/store/backups_test.go`

- [ ] **Step 1: 023 system_meta migration**

```sql
-- 023_system_meta.sql
CREATE TABLE IF NOT EXISTS system_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

(Reuse if a similar table already exists. Verify with `grep -rn "system_meta\|settings_kv\|kv_store" internal/store/migrations/`.)

- [ ] **Step 2: 024 backup_configs uuid migration**

```sql
-- 024_backup_configs_uuid.sql
ALTER TABLE backup_configs ADD COLUMN uuid TEXT;
-- Backfill on first run via Go (SQLite doesn't have a uuid() function).
-- Uniqueness enforced after backfill via:
CREATE UNIQUE INDEX IF NOT EXISTS idx_backup_configs_uuid ON backup_configs(uuid) WHERE uuid IS NOT NULL;
```

- [ ] **Step 3: Store helpers**

Add `internal/store/system_meta.go`:

```go
package store

func (s *Store) GetMeta(key string) (string, bool, error) {
    var v string
    err := s.db.QueryRow(`SELECT value FROM system_meta WHERE key = ?`, key).Scan(&v)
    if err == sql.ErrNoRows {
        return "", false, nil
    }
    if err != nil { return "", false, err }
    return v, true, nil
}

func (s *Store) SetMeta(key, value string) error {
    _, err := s.db.Exec(`
        INSERT INTO system_meta(key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
    `, key, value)
    return err
}
```

Update `BackupConfig` struct to include `UUID string`. Update inserts/selects.

- [ ] **Step 4: UUID backfill helper**

In `internal/store/backups.go`:

```go
func (s *Store) BackfillBackupConfigUUIDs() (int, error) {
    rows, err := s.db.Query(`SELECT id FROM backup_configs WHERE uuid IS NULL OR uuid = ''`)
    if err != nil { return 0, err }
    defer rows.Close()
    var ids []int64
    for rows.Next() {
        var id int64
        if err := rows.Scan(&id); err != nil { return 0, err }
        ids = append(ids, id)
    }
    n := 0
    for _, id := range ids {
        u := uuid.NewString()
        if _, err := s.db.Exec(`UPDATE backup_configs SET uuid = ? WHERE id = ?`, u, id); err != nil {
            return n, err
        }
        n++
    }
    return n, nil
}
```

(Use `github.com/google/uuid` if already in go.mod; else add it.)

- [ ] **Step 5: Tests**

`TestSystemMeta_RoundTrip`, `TestBackfillBackupConfigUUIDs_AssignsForNullsOnly`. Standard temp-DB pattern.

- [ ] **Step 6: Commit**

```bash
go test ./internal/store/ -count=1
git commit -m "feat(store): system_meta table and backup_configs uuid backfill"
```

---

## Task 2 (Phase A): Secrets file types and YAML schema

**Files:**
- Modify: `internal/configsync/types.go` (add `id` on `BackupConfigEntry`)
- Create: `internal/configsync/secrets_types.go`

- [ ] **Step 1: Add UUID field**

In `types.go`, change `BackupConfigEntry`:

```go
type BackupConfigEntry struct {
    ID              string `yaml:"id"` // UUID, correlates with secrets file entry
    Strategy        string `yaml:"strategy"`
    Target          string `yaml:"target"`
    ScheduleCron    string `yaml:"schedule_cron"`
    RetentionMode   string `yaml:"retention_mode"`
    RetentionCount  int    `yaml:"retention_count"`
    RetentionDays   *int   `yaml:"retention_days"`
    VerifyUpload    bool   `yaml:"verify_upload"`
    PreHooks        string `yaml:"pre_hooks,omitempty"`
    PostHooks       string `yaml:"post_hooks,omitempty"`
    Paths           string `yaml:"paths,omitempty"`
    // target_config_enc moved to secrets file
}
```

- [ ] **Step 2: Create secrets_types.go**

```go
package configsync

import "time"

// AppSecrets is {apps_dir}/<slug>/simpledeploy.secrets.yml. Mode 0600.
type AppSecrets struct {
    Version       int                  `yaml:"version"`
    Slug          string               `yaml:"slug"`
    BackupConfigs []BackupSecretsEntry `yaml:"backup_configs,omitempty"`
}

type BackupSecretsEntry struct {
    ID              string `yaml:"id"` // matches BackupConfigEntry.ID
    TargetConfigEnc string `yaml:"target_config_enc"`
}

// GlobalSecrets is {data_dir}/secrets.yml. Mode 0600.
type GlobalSecrets struct {
    Version    int                       `yaml:"version"`
    Users      []UserSecretsEntry        `yaml:"users,omitempty"`
    APIKeys    []APIKeySecretsEntry      `yaml:"api_keys,omitempty"`
    Registries []RegistrySecretsEntry    `yaml:"registries,omitempty"`
    Webhooks   []WebhookSecretsEntry     `yaml:"webhooks,omitempty"`
    DBBackup   *DBBackupSecretsEntry     `yaml:"db_backup,omitempty"`
}

type UserSecretsEntry struct {
    Username     string `yaml:"username"`
    PasswordHash string `yaml:"password_hash"`
}

type APIKeySecretsEntry struct {
    KeyHash   string     `yaml:"key_hash"`
    Username  string     `yaml:"username"`
    Name      string     `yaml:"name"`
    ExpiresAt *time.Time `yaml:"expires_at,omitempty"`
}

type RegistrySecretsEntry struct {
    ID          string `yaml:"id"`
    UsernameEnc string `yaml:"username_enc"`
    PasswordEnc string `yaml:"password_enc"`
}

type WebhookSecretsEntry struct {
    Name             string `yaml:"name"`
    URL              string `yaml:"url"`
    HeadersJSON      string `yaml:"headers_json,omitempty"`
    TemplateOverride string `yaml:"template_override,omitempty"`
}

type DBBackupSecretsEntry struct {
    TargetConfigEnc string `yaml:"target_config_enc"`
}
```

Then strip secret fields from existing types in `types.go`:
- `UserEntry.PasswordHash` → remove.
- `APIKeyEntry.KeyHash` → remove.
- `RegistryEntry.UsernameEnc`, `.PasswordEnc` → remove.
- `WebhookEntry.URL`, `.HeadersJSON`, `.TemplateOverride` → remove.
- `BackupConfigEntry.TargetConfigEnc` → remove.

- [ ] **Step 3: Mark RedactedGlobalSidecar deprecated**

Above the `RedactedGlobalSidecar` type definition, add:

```go
// DEPRECATED: Redundant with FS-authoritative config.yml (which is already
// non-secret). Kept for gitsync-pushed snapshot compatibility. Remove once
// gitsync is migrated to push config.yml directly. See plan
// docs/superpowers/plans/2026-04-27-fs-authoritative-state.md.
```

- [ ] **Step 4: Compile only (no tests yet, all readers/writers update next tasks)**

```bash
go build ./internal/configsync/
```

This will likely fail with downstream errors. That's expected; subsequent tasks fix them. Commit anyway as a checkpoint:

```bash
git add internal/configsync/types.go internal/configsync/secrets_types.go
git commit -m "feat(configsync): split secret material into AppSecrets/GlobalSecrets types"
```

---

## Task 3 (Phase A): bootstrap order documentation in main.go

**Files:**
- Modify: `cmd/simpledeploy/main.go`

- [ ] **Step 1: Add stub functions for the new boot steps**

Find the `serve` cobra command's RunE, identify where store opens and where reconciler starts. Insert two no-op stubs in the right slots, to be filled in by later tasks:

```go
// after store.Open, before reconciler starts:
if err := configsync.RunFirstBootSeedIfNeeded(ctx, db, syncer, cfg); err != nil {
    return fmt.Errorf("fs-auth seed: %w", err)
}
if err := syncer.ReconcileDBFromFS(ctx); err != nil {
    return fmt.Errorf("fs-auth reload: %w", err)
}
```

For now, define both as stubs returning nil in `internal/configsync/`. Real implementation in Task 7.

- [ ] **Step 2: Compile + commit**

```bash
go build ./...
git commit -m "chore(bootstrap): wire FS-authoritative seed and reload entry points"
```

---

## Task 4 (Phase B): Configsync FS-write helpers (extend existing)

**Files:**
- Modify: `internal/configsync/configsync.go`
- Create: `internal/configsync/secrets_io.go`
- Test: `internal/configsync/secrets_io_test.go`

- [ ] **Step 1: Implement file IO helpers**

```go
// secrets_io.go
package configsync

import (
    "fmt"
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

const (
    appSecretsName    = "simpledeploy.secrets.yml"
    globalSecretsName = "secrets.yml"
)

func (s *Syncer) appSecretsPath(slug string) string {
    return filepath.Join(s.appsDir, slug, appSecretsName)
}
func (s *Syncer) globalSecretsPath() string {
    return filepath.Join(s.dataDir, globalSecretsName)
}

func writeYAMLAtomic(path string, mode os.FileMode, v any) error {
    out, err := yaml.Marshal(v)
    if err != nil { return err }
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { return err }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, out, mode); err != nil { return err }
    return os.Rename(tmp, path)
}

func readYAML(path string, into any) error {
    data, err := os.ReadFile(path)
    if err != nil { return err }
    return yaml.Unmarshal(data, into)
}

func (s *Syncer) WriteAppSecrets(slug string, secrets *AppSecrets) error {
    return writeYAMLAtomic(s.appSecretsPath(slug), 0600, secrets)
}
func (s *Syncer) ReadAppSecrets(slug string) (*AppSecrets, error) {
    var sec AppSecrets
    if err := readYAML(s.appSecretsPath(slug), &sec); err != nil { return nil, err }
    return &sec, nil
}
func (s *Syncer) WriteGlobalSecrets(g *GlobalSecrets) error {
    return writeYAMLAtomic(s.globalSecretsPath(), 0600, g)
}
func (s *Syncer) ReadGlobalSecrets() (*GlobalSecrets, error) {
    var g GlobalSecrets
    if err := readYAML(s.globalSecretsPath(), &g); err != nil { return nil, err }
    return &g, nil
}
```

- [ ] **Step 2: Update existing app/global writes**

In `configsync.go`, update `WriteAppSidecar` to:
1. Build the now-non-secret `AppSidecar` from the store.
2. Build a sibling `AppSecrets` from the store.
3. Atomically write `simpledeploy.secrets.yml` first (0600), then `simpledeploy.yml` (0644).

Same pattern for `WriteGlobalSidecar`: write `secrets.yml` (0600) first, then `config.yml` (0644). Continue writing `_global.yml` (redacted) for now (deprecated but kept).

- [ ] **Step 3: Tests**

`TestWriteAppSecrets_Mode0600`, `TestWriteGlobalSecrets_Mode0600` (assert `info.Mode()&0777 == 0600`). `TestWriteAppSidecar_SplitsSecretsAndDeclarative` (assert ciphertext appears only in `simpledeploy.secrets.yml`).

- [ ] **Step 4: Run + commit**

```bash
go test ./internal/configsync/ -count=1
git commit -m "feat(configsync): write secrets to mode-0600 sidecar files"
```

---

## Task 5 (Phase B): Configsync FS-read + parse helpers

**Files:**
- Create: `internal/configsync/load.go`
- Test: `internal/configsync/load_test.go`

- [ ] **Step 1: Loaders**

```go
// load.go
package configsync

import (
    "fmt"
    "os"
    "path/filepath"
)

type LoadedApp struct {
    Slug    string
    Sidecar *AppSidecar // may be nil if file missing
    Secrets *AppSecrets // may be nil if file missing
}

func (s *Syncer) LoadAppFromFS(slug string) (*LoadedApp, error) {
    out := &LoadedApp{Slug: slug}
    var sidecar AppSidecar
    sidecarPath := filepath.Join(s.appsDir, slug, appSidecarName)
    if err := readYAML(sidecarPath, &sidecar); err == nil {
        out.Sidecar = &sidecar
    } else if !os.IsNotExist(err) {
        return nil, fmt.Errorf("read sidecar %s: %w", sidecarPath, err)
    }
    if sec, err := s.ReadAppSecrets(slug); err == nil {
        out.Secrets = sec
    } else if !os.IsNotExist(err) {
        return nil, fmt.Errorf("read secrets %s: %w", slug, err)
    }
    return out, nil
}

type LoadedGlobal struct {
    Sidecar *GlobalSidecar
    Secrets *GlobalSecrets
}

func (s *Syncer) LoadGlobalFromFS() (*LoadedGlobal, error) {
    out := &LoadedGlobal{}
    var sidecar GlobalSidecar
    if err := readYAML(filepath.Join(s.dataDir, globalSidecar), &sidecar); err == nil {
        out.Sidecar = &sidecar
    } else if !os.IsNotExist(err) {
        return nil, err
    }
    if sec, err := s.ReadGlobalSecrets(); err == nil {
        out.Secrets = sec
    } else if !os.IsNotExist(err) {
        return nil, err
    }
    return out, nil
}
```

- [ ] **Step 2: Tests**

Round-trip test: write via `WriteAppSidecar`/`WriteGlobalSidecar`, then `LoadAppFromFS`/`LoadGlobalFromFS`, assert all fields match.

- [ ] **Step 3: Commit**

```bash
go test ./internal/configsync/ -count=1
git commit -m "feat(configsync): load app and global state from FS"
```

---

## Task 6 (Phase B): Apply helpers (FS → DB diff)

**Files:**
- Create: `internal/store/apply.go`
- Test: `internal/store/apply_test.go`

- [ ] **Step 1: Define interfaces**

`Apply*` functions take a parsed FS state and reconcile DB rows to match exactly within the FS-authoritative scope. Each runs in a single tx.

```go
// apply.go
package store

import (
    "database/sql"
    "fmt"
    "github.com/vazra/simpledeploy/internal/configsync"
)

func (s *Store) ApplyAppSidecar(slug string, loaded *configsync.LoadedApp) error {
    tx, err := s.db.Begin()
    if err != nil { return err }
    defer tx.Rollback()

    // 1. Update app meta (display_name).
    if loaded.Sidecar != nil {
        if _, err := tx.Exec(
            `UPDATE apps SET name = ? WHERE slug = ?`,
            firstNonEmpty(loaded.Sidecar.App.DisplayName, slug),
            slug,
        ); err != nil { return err }
    }

    // 2. Replace alert_rules for slug.
    if _, err := tx.Exec(`DELETE FROM alert_rules WHERE app_slug = ?`, slug); err != nil { return err }
    if loaded.Sidecar != nil {
        for _, ar := range loaded.Sidecar.AlertRules {
            // resolve webhook by name -> id
            var wid int64
            if err := tx.QueryRow(`SELECT id FROM webhooks WHERE name = ?`, ar.Webhook).Scan(&wid); err != nil {
                if err == sql.ErrNoRows {
                    continue // skip orphan; surface via reconcile log
                }
                return err
            }
            if _, err := tx.Exec(
                `INSERT INTO alert_rules (app_slug, metric, operator, threshold, duration_sec, webhook_id, enabled) VALUES (?,?,?,?,?,?,?)`,
                slug, ar.Metric, ar.Operator, ar.Threshold, ar.DurationSec, wid, ar.Enabled,
            ); err != nil { return err }
        }
    }

    // 3. Replace backup_configs for slug, joining secrets by UUID.
    if _, err := tx.Exec(`DELETE FROM backup_configs WHERE app_slug = ?`, slug); err != nil { return err }
    secretsByID := map[string]configsync.BackupSecretsEntry{}
    if loaded.Secrets != nil {
        for _, b := range loaded.Secrets.BackupConfigs {
            secretsByID[b.ID] = b
        }
    }
    if loaded.Sidecar != nil {
        for _, bc := range loaded.Sidecar.BackupConfigs {
            sec := secretsByID[bc.ID]
            if _, err := tx.Exec(
                `INSERT INTO backup_configs (uuid, app_slug, strategy, target, schedule_cron, target_config_enc, retention_mode, retention_count, retention_days, verify_upload, pre_hooks, post_hooks, paths) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
                bc.ID, slug, bc.Strategy, bc.Target, bc.ScheduleCron, sec.TargetConfigEnc,
                bc.RetentionMode, bc.RetentionCount, bc.RetentionDays, bc.VerifyUpload,
                bc.PreHooks, bc.PostHooks, bc.Paths,
            ); err != nil { return err }
        }
    }

    // 4. Replace user_app_access.
    if _, err := tx.Exec(`DELETE FROM user_app_access WHERE app_slug = ?`, slug); err != nil { return err }
    if loaded.Sidecar != nil {
        for _, a := range loaded.Sidecar.Access {
            var uid int64
            if err := tx.QueryRow(`SELECT id FROM users WHERE username = ?`, a.Username).Scan(&uid); err == nil {
                _, _ = tx.Exec(`INSERT INTO user_app_access (user_id, app_slug) VALUES (?, ?)`, uid, slug)
            }
        }
    }

    return tx.Commit()
}

func firstNonEmpty(a, b string) string { if a != "" { return a }; return b }
```

(Adapt column names to the actual schema; `grep -n "alert_rules\|backup_configs" internal/store/migrations/*.sql` to confirm.)

- [ ] **Step 2: ApplyGlobalSidecar**

Same shape, covering `users`, `api_keys`, `registries`, `webhooks`, `db_backup_config`. Each block: DELETE all rows, then INSERT from loaded sidecar+secrets joining on natural keys (username for users, name for webhooks, id for registries).

- [ ] **Step 3: Tests**

Per-resource: write FS state → ApplyAppSidecar → query DB → matches. Edge: rule referencing missing webhook is dropped with a log line. Backup config without matching secrets entry inserts NULL `target_config_enc`.

- [ ] **Step 4: Commit**

```bash
go test ./internal/store/ -count=1
git commit -m "feat(store): apply FS-loaded sidecars to DB transactionally"
```

---

## Task 7 (Phase C): First-boot seed + ReconcileDBFromFS

**Files:**
- Modify: `internal/configsync/configsync.go`
- Create: `internal/configsync/firstboot.go`
- Test: `internal/configsync/firstboot_test.go`

- [ ] **Step 1: First-boot seed**

```go
// firstboot.go
package configsync

import (
    "context"
    "log"
    "time"

    "github.com/vazra/simpledeploy/internal/config"
    "github.com/vazra/simpledeploy/internal/store"
)

const fsSeededKey = "fs_authoritative_seeded_at"

// RunFirstBootSeedIfNeeded checks system_meta for the seeded marker.
// If absent, writes per-app + global FS files from current DB state and
// records the marker. Idempotent.
func RunFirstBootSeedIfNeeded(ctx context.Context, db *store.Store, s *Syncer, cfg *config.Config) error {
    if _, ok, err := db.GetMeta(fsSeededKey); err != nil {
        return err
    } else if ok {
        return nil
    }

    // Backfill UUIDs first so writes carry stable IDs.
    n, err := db.BackfillBackupConfigUUIDs()
    if err != nil { return err }
    if n > 0 { log.Printf("[fs-auth] backfilled %d backup_config UUIDs", n) }

    apps, err := db.ListAppsWithOptions(store.ListAppsOptions{IncludeArchived: true})
    if err != nil { return err }
    for _, a := range apps {
        if err := s.WriteAppSidecar(a.Slug); err != nil {
            log.Printf("[fs-auth] write app sidecar %s: %v", a.Slug, err)
        }
    }
    if err := s.WriteGlobalSidecar(); err != nil {
        return err
    }
    if err := db.SetMeta(fsSeededKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
        return err
    }
    log.Printf("[fs-auth] first-boot seed complete; FS is now the source of truth")
    return nil
}
```

- [ ] **Step 2: ReconcileDBFromFS**

```go
// in configsync.go
func (s *Syncer) ReconcileDBFromFS(ctx context.Context) error {
    apps, err := os.ReadDir(s.appsDir)
    if err != nil { return err }
    for _, e := range apps {
        if !e.IsDir() || strings.HasPrefix(e.Name(), ".") { continue }
        loaded, err := s.LoadAppFromFS(e.Name())
        if err != nil {
            log.Printf("[fs-auth] load %s: %v", e.Name(), err)
            continue
        }
        if loaded.Sidecar == nil { continue } // not yet sidecared
        if err := s.store.ApplyAppSidecar(e.Name(), loaded); err != nil {
            log.Printf("[fs-auth] apply %s: %v", e.Name(), err)
        }
    }
    g, err := s.LoadGlobalFromFS()
    if err != nil { return err }
    if g.Sidecar != nil {
        if err := s.store.ApplyGlobalSidecar(g); err != nil {
            log.Printf("[fs-auth] apply global: %v", err)
        }
    }
    return nil
}
```

- [ ] **Step 3: Tests**

`TestFirstBootSeed_WritesFilesAndMarker` — empty FS, two apps + global state in DB, run seed, files exist, marker set. Re-run, no-op.

`TestReconcileDBFromFS_AppliesAlertRules` — write a sidecar by hand with an alert rule, call `ReconcileDBFromFS`, assert DB has the rule.

- [ ] **Step 4: Commit**

```bash
go test ./internal/configsync/ ./internal/store/ -count=1
git commit -m "feat(configsync): first-boot seed and FS->DB reload"
```

---

## Task 8 (Phase D): Watcher extension

**Files:**
- Modify: `internal/reconciler/watcher.go`
- Test: `internal/reconciler/watcher_test.go` (add or extend)

- [ ] **Step 1: Watch new paths**

Currently the watcher likely watches `apps_dir` recursively for compose changes. Extend so:

- Per-app: events for files matching `*/simpledeploy.yml` and `*/simpledeploy.secrets.yml` route to a debounced `syncer.LoadAppFromFS(slug)` + `store.ApplyAppSidecar(slug, loaded)`.
- Global: separately watch `{data_dir}/config.yml` and `{data_dir}/secrets.yml`. Debounce, then `syncer.LoadGlobalFromFS()` + `store.ApplyGlobalSidecar(...)`.

Follow the existing debounce pattern in `watcher.go`; reuse the same debounce duration.

- [ ] **Step 2: Tests**

`TestWatcher_AppliesSidecarEditOnChange`: start the reconciler with watcher, write a sidecar with a new alert rule, sleep past debounce, assert DB updated.

`TestWatcher_GlobalConfigEdit`: write `config.yml` adding a webhook (name+type), assert DB row appears.

These tests are slow-ish; gate with `if testing.Short() { t.Skip() }` per existing watcher-test convention.

- [ ] **Step 3: Commit**

```bash
go test ./internal/reconciler/ -count=1
git commit -m "feat(reconciler): watcher reloads FS-authoritative state on change"
```

---

## Task 9 (Phase D): API mutation handlers switch to FS-first

**Files:**
- Modify: per-resource handlers in `internal/api/`. Identify with `grep -rn "store.Insert\|store.Upsert\|store.Update\|store.Delete" internal/api/`.

For each mutating endpoint touching FS-authoritative state, wrap the mutation in the FS-first pattern.

- [ ] **Step 1: Define a small wrapper**

In `internal/api/fsauth.go`:

```go
package api

import (
    "github.com/vazra/simpledeploy/internal/configsync"
)

// withFSFirstApp re-exports app-scoped FS-authoritative writes by:
//   1. running the DB mutation closure inside a tx,
//   2. rebuilding sidecar+secrets from post-tx state,
//   3. writing files atomically.
// On FS write failure the tx is rolled back.
func (h *Handlers) withFSFirstApp(slug string, mutate func() error) error {
    if err := mutate(); err != nil { return err }
    return h.syncer.WriteAppSidecar(slug)
}

func (h *Handlers) withFSFirstGlobal(mutate func() error) error {
    if err := mutate(); err != nil { return err }
    return h.syncer.WriteGlobalSidecar()
}
```

(Note: this is "DB-first then FS" — simpler and matches the spec's "FS always wins on reload" property since the watcher will reapply if FS write happens out-of-order. True FS-first with rollback adds complexity; document this choice in the operations doc as "DB tx commits, then FS write, on FS failure the watcher will eventually reconcile.")

ALTERNATIVE (stricter, recommended): write FS first to a temp file, run DB tx, on success rename temp → final. If tx fails, delete temp.

Implement the stricter version:

```go
func (h *Handlers) withFSFirstApp(slug string, mutate func(tx *sql.Tx) error) error {
    tx, err := h.db.BeginTx()
    if err != nil { return err }
    defer tx.Rollback()
    if err := mutate(tx); err != nil { return err }
    // Build sidecar + secrets from in-tx state (helper that takes tx).
    sidecar, secrets, err := h.syncer.BuildAppSidecarTx(tx, slug)
    if err != nil { return err }
    if err := h.syncer.WriteAppSecrets(slug, secrets); err != nil { return err }
    sidecarPath := h.syncer.AppSidecarPath(slug)
    if err := writeYAMLAtomic(sidecarPath, 0644, sidecar); err != nil { return err }
    return tx.Commit()
}
```

(`BuildAppSidecarTx` is an addition: same as `buildAppSidecar` but accepts a `*sql.Tx`. Needed so the FS write reflects the in-tx state.)

- [ ] **Step 2: Wrap each mutating handler**

Endpoints to update (search and adapt one at a time):
- alert rules: create, update, delete → `withFSFirstApp(slug, ...)`
- backup configs: create, update, delete → `withFSFirstApp`. Generate a UUID on create.
- per-app access changes → `withFSFirstApp`
- users, api_keys, registries, webhooks, db_backup_config CRUD → `withFSFirstGlobal`

For each handler:
1. Update test to assert the FS file is written and contains the change.
2. Update implementation.
3. Run tests.
4. Commit per resource: `feat(api): FS-first writes for <resource>`.

- [ ] **Step 3: Final integration test**

Add `TestFSAuthoritative_AlertRuleCreateRoundTrip`:
1. POST a new alert rule via API.
2. Read `simpledeploy.yml` from disk.
3. Assert the new rule is present.
4. Hand-edit the file to add another rule.
5. Wait for watcher debounce.
6. GET alert rules via API.
7. Assert both rules visible.

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(api): integration test for FS-first round trip"
```

---

## Task 10 (Phase E): .gitignore template injection

**Files:**
- Modify: `internal/configsync/configsync.go` (or the bootstrap file from Task 3)

- [ ] **Step 1: Helper**

```go
// in firstboot.go
func ensureGitignore(dir string, lines []string) error {
    path := filepath.Join(dir, ".gitignore")
    var existing string
    if data, err := os.ReadFile(path); err == nil {
        existing = string(data)
    } else if !os.IsNotExist(err) {
        return err
    }
    var add []string
    for _, l := range lines {
        if !strings.Contains(existing, l) { add = append(add, l) }
    }
    if len(add) == 0 { return nil }
    block := "\n# simpledeploy: never commit secrets\n" + strings.Join(add, "\n") + "\n"
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil { return err }
    defer f.Close()
    _, err = f.WriteString(block)
    return err
}
```

Call from `RunFirstBootSeedIfNeeded`:

```go
_ = ensureGitignore(cfg.AppsDir, []string{"*.secrets.yml"})
_ = ensureGitignore(cfg.DataDir, []string{"secrets.yml"})
```

- [ ] **Step 2: Test + commit**

`TestEnsureGitignore_AppendsMissingOnly`. Commit:

```bash
git commit -m "feat(configsync): inject .gitignore entries for secrets files"
```

---

## Task 11 (Phase E): Documentation

**Files:**
- Create: `docs/operations/state-on-disk.md`
- Modify: `docs-site/astro.config.mjs` sidebar

- [ ] **Step 1: Write the page**

Sections (per spec):
1. File layout (per-app, global) with a tree diagram.
2. Authority table (resource × FS-or-DB).
3. Secrets boundary (what's encrypted, what's hashed, what's plaintext-with-perms; what `master_secret` protects; threat model).
4. Editing files by hand (live + watcher-debounce; perms; YAML correctness; recover from a bad edit by `git checkout`).
5. GitOps with gitsync (commit `*.yml`; never commit `*.secrets.yml`; pulling triggers DB reload).
6. Recovery from corrupted DB (`rm simpledeploy.db`, restart, server rebuilds from FS; runtime metrics/audit lost).
7. Troubleshooting (where logs surface FS/DB diffs, how to force a reload by `touch`-ing a sidecar).
8. Caveat: macOS Docker Desktop dev mode may miss bind-mount edits originating outside the container; use the in-container path or restart the dev container after large changes.

- [ ] **Step 2: Sidebar entry + commit**

```bash
git commit -m "docs(operations): file-as-truth state model + secret boundaries"
```

---

## Task 12 (Phase F): Forward-compat with archive plan (tombstone reload)

**Files:**
- Modify: `internal/configsync/configsync.go`'s `ReconcileDBFromFS` (Task 7)
- Test: extend Task 7's tests

- [ ] **Step 1: Extend reload**

Add to the end of `ReconcileDBFromFS`:

```go
archiveDir := filepath.Join(s.dataDir, archiveDirName)
entries, _ := os.ReadDir(archiveDir)
for _, e := range entries {
    if !strings.HasSuffix(e.Name(), ".yml") { continue }
    slug := strings.TrimSuffix(e.Name(), ".yml")
    tomb, err := s.ReadTombstone(slug)
    if err != nil { continue }
    if _, err := s.store.GetAppBySlug(slug); err != nil {
        // App row missing entirely: re-insert as archived shell.
        _ = s.store.UpsertApp(&store.App{
            Slug:        slug,
            Name:        firstNonEmpty(tomb.App.DisplayName, slug),
            Status:      "archived",
            ArchivedAt:  sql.NullTime{Time: tomb.ArchivedAt, Valid: true},
        }, nil)
    } else {
        // App row exists: ensure archived_at matches tombstone.
        _ = s.store.MarkAppArchived(slug, tomb.ArchivedAt)
    }
}
```

- [ ] **Step 2: Test**

`TestReconcileDBFromFS_RehydratesArchivedFromTombstone`: write a tombstone for a slug not in DB, run reload, assert the row exists with `archived_at` set.

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(configsync): rehydrate archived apps from tombstones on reload"
```

---

## Task 13 (Phase G): E2E

**Files:**
- Create: `e2e/tests/21-fs-authoritative.spec.js`

- [ ] **Step 1: Spec**

```js
import { test, expect } from '@playwright/test'
import { loginAsAdmin } from '../helpers/auth.js'
import { getState } from '../helpers/server.js'
import fs from 'fs'
import path from 'path'
import yaml from 'js-yaml'

test.describe('FS-authoritative', () => {
  test('hand-editing simpledeploy.yml propagates to UI', async ({ page }) => {
    await loginAsAdmin(page)
    const slug = 'e2e-nginx'
    const sidecarPath = path.join(getState().appsDir, slug, 'simpledeploy.yml')
    const content = yaml.load(fs.readFileSync(sidecarPath, 'utf8'))
    content.alert_rules = (content.alert_rules || []).concat([{
      metric: 'cpu_percent',
      operator: '>',
      threshold: 99,
      duration_sec: 60,
      webhook: 'fs-auth-test',
      enabled: true,
    }])
    fs.writeFileSync(sidecarPath, yaml.dump(content))
    await page.waitForTimeout(3000) // watcher debounce + apply
    await page.goto(`${getState().baseURL}/#/apps/${slug}/alerts`)
    await expect(page.locator('main').getByText('cpu_percent')).toBeVisible()
  })
})
```

(Adjust selectors to match the existing alerts UI; ensure a webhook named `fs-auth-test` exists or pre-create via API in this spec's setup.)

- [ ] **Step 2: Run + commit**

```bash
cd e2e && npx playwright test 01-setup.spec.js 03-deploy.spec.js 21-fs-authoritative.spec.js --reporter=list
git commit -m "test(e2e): hand edit sidecar propagates via watcher"
```

---

## Task 14 (Phase G): Migration/upgrade smoke test

**Files:**
- Create: `internal/configsync/upgrade_smoke_test.go`

- [ ] **Step 1: Test**

Simulate "existing install" by:
1. Open a fresh DB with migrations.
2. Manually insert: 1 user, 1 app, 1 alert rule, 1 backup config (with `target_config_enc`), 1 webhook.
3. Construct a `Syncer`. Confirm `system_meta` does not have `fs_authoritative_seeded_at`.
4. Run `RunFirstBootSeedIfNeeded`.
5. Assert: `simpledeploy.yml`, `simpledeploy.secrets.yml`, `config.yml`, `secrets.yml` all exist with correct perms. Marker set.
6. Re-run; no errors, no overwrites (mtime stable).
7. Hand-edit `simpledeploy.yml` to remove the alert rule.
8. Run `ReconcileDBFromFS`.
9. Assert DB no longer has the alert rule.

- [ ] **Step 2: Commit**

```bash
go test ./internal/configsync/ -run UpgradeSmoke -v
git commit -m "test(configsync): upgrade smoke covering seed + reload + edit"
```

---

## Task 15 (Phase H): Final integration

- [ ] **Step 1: Full short test suite + lint + build**

```bash
go vet ./... && go test ./... -short
make build-go
cd ui && npm test -- --run
```

- [ ] **Step 2: E2E lite**

```bash
make e2e-lite
```

- [ ] **Step 3: Push**

```bash
git push
```

---

## Self-review notes

- Spec coverage (per-resource):
  - app meta, alert_rules, backup_configs (split secret), user_app_access → ApplyAppSidecar (T6) + sidecar writes (T4) + watcher (T8) + handlers (T9). ✓
  - users, api_keys, registries, webhooks, db_backup_config → ApplyGlobalSidecar (T6) + writes (T4) + watcher (T8) + handlers (T9). ✓
  - first-boot seed + marker (T7) ✓
  - .gitignore (T10) ✓
  - docs (T11) ✓
  - tombstone forward-compat (T12) ✓
  - e2e (T13) + upgrade smoke (T14) ✓
- Resolved open questions: UUID on backup_configs (T1+T2), deprecation comment on RedactedGlobalSidecar (T2), macOS dev caveat in T11 docs.
- Method signature consistency: `WriteAppSidecar(slug)` is parameter-less today taking the slug; verify and align in T4 step 2. `BuildAppSidecarTx(tx, slug)` is introduced in T9 and consumed only in T9 helpers. `LoadAppFromFS(slug)` returns `*LoadedApp` consumed by `ApplyAppSidecar`. No drift.
- No "TBD"/"TODO"/"implement later" placeholders. Every step has either code, tests, or a commit command.
- The "DB-first then FS" alternative in T9 step 1 is documented but the stricter "FS-during-tx" version is the chosen path. The plan picks one; the alternative is left for context only.
