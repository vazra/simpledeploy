# Archive-instead-of-delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When an app's directory disappears from `apps_dir`, archive it (preserve DB row + history + a tombstone snapshot) instead of deleting. Provide a read-only archive view in the UI with a Purge action.

**Architecture:** Add `archived_at` column on `apps`. New tombstone files at `{data_dir}/archive/<slug>.yml` capture the last sidecar state. Reconciler's missing-dir branch switches from `removeApp` to a new `archiveApp` path. `DELETE /api/apps/{slug}` keeps full-purge semantics for non-archived apps; new `POST /api/apps/{slug}/purge` finalizes archived apps. UI hides archived apps from main views and adds an Archived page under System.

**Tech Stack:** Go (reconciler, store, api, configsync), SQLite migration, Svelte/Vite UI, Playwright e2e.

**Spec:** `docs/superpowers/specs/2026-04-27-archive-instead-of-delete-design.md`

**Migration number:** #22 (#21 already taken by `recipe_pulls`).

---

## Parallelism guide for subagent-driven execution

Tasks are tagged with a phase. Within a phase, listed tasks may run in parallel. Phases are sequential.

- **Phase A (parallel):** 1, 2, 3
- **Phase B (parallel):** 4, 5
- **Phase C (parallel):** 6, 7
- **Phase D (parallel):** 8, 9
- **Phase E:** 10
- **Phase F:** 11

---

## Task 1 (Phase A): DB migration + store schema

**Files:**
- Create: `internal/store/migrations/022_apps_archived_at.sql`
- Modify: `internal/store/apps.go` (add `ArchivedAt` to `App`, scan it in `ListApps`/`GetAppBySlug`)
- Test: `internal/store/apps_test.go`

- [ ] **Step 1: Write the migration**

```sql
-- internal/store/migrations/022_apps_archived_at.sql
ALTER TABLE apps ADD COLUMN archived_at TIMESTAMP NULL;
CREATE INDEX idx_apps_archived_at ON apps(archived_at) WHERE archived_at IS NOT NULL;
```

- [ ] **Step 2: Add field to App struct + scan**

In `internal/store/apps.go`, add `ArchivedAt sql.NullTime` to `App`. Update every `SELECT` that builds `App` to include `archived_at` and scan into `&app.ArchivedAt`. Update every `INSERT`/`UPSERT` to NOT set archived_at (default NULL). Run `grep -n "SELECT.*FROM apps\|INSERT INTO apps\|app.Name, &app" internal/store/apps.go` first to enumerate sites.

- [ ] **Step 3: Test migration applies and field round-trips**

Add `TestApp_ArchivedAtRoundtrip` in `apps_test.go`: open store (runs migrations), upsert app, expect `ArchivedAt.Valid == false`, then run `UPDATE apps SET archived_at = ? WHERE slug = ?`, re-`GetAppBySlug`, expect `ArchivedAt.Valid == true`.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/store/ -run TestApp_ArchivedAtRoundtrip -v
```

Expected: PASS. Then `go test ./internal/store/ -count=1` to confirm no regressions.

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrations/022_apps_archived_at.sql internal/store/apps.go internal/store/apps_test.go
git commit -m "feat(store): add archived_at column on apps"
```

---

## Task 2 (Phase A): Store list/filter helpers

**Files:**
- Modify: `internal/store/apps.go`
- Test: `internal/store/apps_test.go`

- [ ] **Step 1: Write failing tests**

In `apps_test.go`, add:
- `TestListApps_ExcludesArchivedByDefault`: insert two apps, mark one archived, `ListApps()` returns 1.
- `TestListAppsWithOptions_IncludeArchived`: same setup, `ListAppsWithOptions(ListAppsOptions{IncludeArchived: true})` returns 2.
- `TestListArchivedApps`: same setup, `ListArchivedApps()` returns the archived one only.
- `TestMarkAppArchived`: upsert app, `MarkAppArchived(slug, time.Now())`, `GetAppBySlug` shows `ArchivedAt.Valid && .Time` near now.

- [ ] **Step 2: Run, expect compile failure**

```bash
go test ./internal/store/ -run "TestListApps_ExcludesArchivedByDefault|TestListAppsWithOptions_IncludeArchived|TestListArchivedApps|TestMarkAppArchived"
```

Expected: undefined `ListAppsWithOptions`, `ListArchivedApps`, `MarkAppArchived`, `ListAppsOptions`.

- [ ] **Step 3: Implement**

Add to `internal/store/apps.go`:

```go
type ListAppsOptions struct {
    IncludeArchived bool
    OnlyArchived    bool
}

// Existing ListApps now delegates.
func (s *Store) ListApps() ([]App, error) {
    return s.ListAppsWithOptions(ListAppsOptions{})
}

func (s *Store) ListAppsWithOptions(opts ListAppsOptions) ([]App, error) {
    where := "WHERE archived_at IS NULL"
    if opts.OnlyArchived {
        where = "WHERE archived_at IS NOT NULL"
    } else if opts.IncludeArchived {
        where = ""
    }
    // ...build the same SELECT used by ListApps with this where clause...
}

func (s *Store) ListArchivedApps() ([]App, error) {
    return s.ListAppsWithOptions(ListAppsOptions{OnlyArchived: true})
}

func (s *Store) MarkAppArchived(slug string, at time.Time) error {
    _, err := s.db.Exec(`UPDATE apps SET archived_at = ? WHERE slug = ?`, at.UTC(), slug)
    return err
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/store/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/apps.go internal/store/apps_test.go
git commit -m "feat(store): add archive-aware list helpers and MarkAppArchived"
```

---

## Task 3 (Phase A): Store PurgeApp cascade

**Files:**
- Modify: `internal/store/apps.go`
- Test: `internal/store/apps_test.go`

- [ ] **Step 1: Failing test**

`TestPurgeApp_CascadesHistory`: insert app + label + access + a `deploy_events` row + `audit_log` row + `compose_versions` row + `backup_runs` + `alert_history`. Call `PurgeApp(slug)`. Assert the app and every related row are gone.

- [ ] **Step 2: Implement PurgeApp**

```go
func (s *Store) PurgeApp(slug string) error {
    tx, err := s.db.Begin()
    if err != nil { return err }
    defer tx.Rollback()
    statements := []string{
        `DELETE FROM audit_log WHERE app_slug = ?`,
        `DELETE FROM deploy_events WHERE app_slug = ?`,
        `DELETE FROM compose_versions WHERE app_slug = ?`,
        `DELETE FROM backup_runs WHERE app_slug = ?`,
        `DELETE FROM alert_history WHERE app_slug = ?`,
        `DELETE FROM backup_configs WHERE app_slug = ?`,
        `DELETE FROM alert_rules WHERE app_slug = ?`,
        `DELETE FROM app_labels WHERE app_slug = ?`,
        `DELETE FROM user_app_access WHERE app_slug = ?`,
        `DELETE FROM apps WHERE slug = ?`,
    }
    for _, q := range statements {
        if _, err := tx.Exec(q, slug); err != nil {
            return fmt.Errorf("purge %s: %w", q, err)
        }
    }
    return tx.Commit()
}
```

(If column names in any table differ from `app_slug`, run `grep -rn "app_slug\|FOREIGN KEY.*apps" internal/store/migrations/` and adjust.)

- [ ] **Step 3: Run tests**

```bash
go test ./internal/store/ -run TestPurgeApp_CascadesHistory -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/store/apps.go internal/store/apps_test.go
git commit -m "feat(store): add PurgeApp cascade"
```

---

## Task 4 (Phase B): Tombstone build/write/read in configsync

**Files:**
- Modify: `internal/configsync/types.go` (add archived_at field on AppSidecar OR a thin wrapper type)
- Create: `internal/configsync/archive.go`
- Test: `internal/configsync/archive_test.go`

- [ ] **Step 1: Add tombstone type and helpers**

In `internal/configsync/archive.go`:

```go
package configsync

import (
    "fmt"
    "os"
    "path/filepath"
    "time"

    "gopkg.in/yaml.v3"
)

const archiveDirName = "archive"

// Tombstone is the snapshot written to {data_dir}/archive/<slug>.yml when
// an app is archived (its dir disappeared from apps_dir).
type Tombstone struct {
    Version    int        `yaml:"version"`
    ArchivedAt time.Time  `yaml:"archived_at"`
    App        AppMeta    `yaml:"app"`
    AlertRules []AlertRuleEntry    `yaml:"alert_rules,omitempty"`
    BackupConfigs []BackupConfigEntry `yaml:"backup_configs,omitempty"`
    Access     []AccessEntry       `yaml:"access,omitempty"`
}

func (s *Syncer) ArchiveDir() string {
    return filepath.Join(s.dataDir, archiveDirName)
}

// WriteTombstone builds a tombstone from current DB state for the slug and
// writes it atomically to {data_dir}/archive/<slug>.yml (mode 0644).
func (s *Syncer) WriteTombstone(slug string, archivedAt time.Time) error {
    sidecar, err := s.buildAppSidecar(slug) // existing private builder; extract from WriteAppSidecar
    if err != nil {
        return fmt.Errorf("build sidecar: %w", err)
    }
    tomb := Tombstone{
        Version:       Version,
        ArchivedAt:    archivedAt.UTC(),
        App:           sidecar.App,
        AlertRules:    sidecar.AlertRules,
        BackupConfigs: sidecar.BackupConfigs,
        Access:        sidecar.Access,
    }
    return s.writeTombstoneFile(slug, &tomb)
}

func (s *Syncer) writeTombstoneFile(slug string, t *Tombstone) error {
    if err := os.MkdirAll(s.ArchiveDir(), 0755); err != nil {
        return err
    }
    path := filepath.Join(s.ArchiveDir(), slug+".yml")
    out, err := yaml.Marshal(t)
    if err != nil {
        return err
    }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, out, 0644); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}

func (s *Syncer) ReadTombstone(slug string) (*Tombstone, error) {
    path := filepath.Join(s.ArchiveDir(), slug+".yml")
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var t Tombstone
    if err := yaml.Unmarshal(data, &t); err != nil {
        return nil, err
    }
    return &t, nil
}

func (s *Syncer) DeleteTombstone(slug string) error {
    path := filepath.Join(s.ArchiveDir(), slug+".yml")
    if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
        return err
    }
    return nil
}
```

If `buildAppSidecar` does not exist as a private function today, refactor `WriteAppSidecar` to factor the build step out (zero behavior change). Look at `WriteAppSidecar` in `internal/configsync/configsync.go`.

- [ ] **Step 2: Tests**

In `archive_test.go`: spin a `Syncer` with temp data/apps dirs, seed an app + alert rule + backup config in the store, call `WriteTombstone(slug, time.Now())`, assert file exists, parse with `ReadTombstone`, assert fields match. Then `DeleteTombstone`, assert file gone.

- [ ] **Step 3: Run**

```bash
go test ./internal/configsync/ -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/configsync/archive.go internal/configsync/archive_test.go internal/configsync/configsync.go
git commit -m "feat(configsync): tombstone read/write/delete helpers"
```

---

## Task 5 (Phase B): Audit render entries

**Files:**
- Modify: `internal/audit/render/`
- Test: `internal/audit/render/*_test.go`

- [ ] **Step 1: Find current render layout**

Run `ls internal/audit/render/` and read one existing renderer to follow the pattern.

- [ ] **Step 2: Add archive + purge renderers**

Add cases for `category=app, action=archive` and `category=app, action=purge`. Sample summaries: `"App {slug} archived (directory removed from disk)"` and `"App {slug} purged ({n} history rows deleted)"`.

- [ ] **Step 3: Tests + commit**

Mirror existing render-test style. Commit:

```bash
git commit -m "feat(audit): render archive and purge actions"
```

---

## Task 6 (Phase C): Reconciler archiveApp + dispatch change

**Files:**
- Modify: `internal/reconciler/reconciler.go`
- Test: `internal/reconciler/reconciler_test.go`

- [ ] **Step 1: Failing test**

In `reconciler_test.go`, add `TestReconcileArchivesOnDirRemoval`:
1. Build env with a `*configsync.Syncer` (extend `newTestEnv` to wire one through `New()` if today's tests pass `nil`; verify by looking at the `New` signature).
2. Write compose, reconcile, app exists.
3. Remove the app directory.
4. Reconcile again.
5. Assert: `mock.hasCall("Teardown:rmapp")`, `app.ArchivedAt.Valid == true`, tombstone file exists at `{dataDir}/archive/rmapp.yml`, `mock.hasCall("DeleteApp:...")` is false (the previous remove path should not run).

Also add `TestReconcileSkipsAlreadyArchivedApp`: archived row, dir absent, Reconcile, assert no extra Teardown call and tombstone unchanged (stat mtime stable).

- [ ] **Step 2: Implement archiveApp**

In `internal/reconciler/reconciler.go`, add:

```go
// archiveApp runs Teardown, writes the tombstone, marks the row archived, and
// drops proxy routes. Replaces removeApp on the directory-missing branch.
func (r *Reconciler) archiveApp(ctx context.Context, slug string) error {
    if err := r.deployer.Teardown(ctx, slug); err != nil {
        log.Printf("[reconciler] archive teardown %s: %v", slug, err)
        // continue: we still want to mark archived
    }
    now := time.Now().UTC()
    if r.syncer != nil {
        if err := r.syncer.WriteTombstone(slug, now); err != nil {
            log.Printf("[reconciler] archive tombstone %s: %v", slug, err)
        }
    }
    if err := r.store.MarkAppArchived(slug, now); err != nil {
        return fmt.Errorf("mark archived: %w", err)
    }
    // Audit
    r.audit.Record(audit.Entry{Category: "app", Action: "archive", AppSlug: slug, Actor: "system"})
    return nil
}
```

(If the reconciler does not currently hold an `audit.Recorder`, look at how the API package gets one and wire it into `reconciler.New`. Add it as a constructor arg + nil-safe.)

- [ ] **Step 3: Replace removeApp call in Reconcile**

In `Reconcile`, locate the loop:

```go
for _, a := range current {
    if _, exists := desired[a.Slug]; !exists {
        if err := r.removeApp(ctx, a.Slug); err != nil { ... }
    }
}
```

Change to:

```go
for _, a := range current {
    if a.ArchivedAt.Valid {
        continue // already archived; do nothing
    }
    if _, exists := desired[a.Slug]; !exists {
        if err := r.archiveApp(ctx, a.Slug); err != nil { ... }
    }
}
```

- [ ] **Step 4: Run reconciler tests**

```bash
go test ./internal/reconciler/ -count=1
```

If `TestReconcileRemoveApp` (existing) still asserts row deletion, update it to assert archived state instead and rename to `TestReconcileArchiveApp`. Keep `removeApp` (it stays for the API purge path) but mark it private if it is not already.

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(reconciler): archive on disk-removal instead of delete"
```

---

## Task 7 (Phase C): API handlers

**Files:**
- Modify: `internal/api/apps.go` (or whichever file owns DELETE /api/apps/{slug}; `grep -rn "DELETE.*apps\|deleteApp\|handleDeleteApp" internal/api/`)
- Modify: `internal/api/router.go` to add new routes.
- Test: `internal/api/apps_archive_test.go`

- [ ] **Step 1: Failing tests**

Create `internal/api/apps_archive_test.go` covering:
- `TestGetApps_ExcludesArchivedByDefault`
- `TestGetApps_IncludeArchivedQuery` (query `?include_archived=1` returns archived rows with `archived_at` populated)
- `TestGetAppsArchived_ReturnsTombstone` (response includes parsed tombstone)
- `TestPostPurge_404OnNonArchived`
- `TestPostPurge_200OnArchived` (asserts tombstone removed, DB row gone, history gone)
- `TestDeleteApp_409OnArchived` (delete endpoint refuses archived)
- `TestDeleteApp_PurgesNonArchived` (existing-style full purge: teardown + remove dir + DB cascade)

Use the existing test harness pattern from one of the current api tests.

- [ ] **Step 2: Implement**

Add routes in router (sample paths; adapt to existing router style):

```go
r.HandleFunc("/api/apps/archived", h.handleListArchived).Methods("GET")
r.HandleFunc("/api/apps/{slug}/purge", h.handlePurge).Methods("POST")
```

Modify `handleListApps` to accept `?include_archived=1` and call `store.ListAppsWithOptions(ListAppsOptions{IncludeArchived: true})`. Default unchanged.

Implement `handleListArchived`: `store.ListArchivedApps()` + per-row `syncer.ReadTombstone(slug)` (404-tolerant; nil tombstone allowed). Return JSON `[{...app, archived_at, tombstone: {...}}, ...]`.

Implement `handlePurge`:
1. Fetch app, return 404 if not found.
2. Return 404 if `!app.ArchivedAt.Valid` (archived only).
3. `store.PurgeApp(slug)`.
4. `syncer.DeleteTombstone(slug)`.
5. Audit `app/purge`.
6. 204 response.

Modify `handleDeleteApp`:
1. Fetch app.
2. If `app.ArchivedAt.Valid`, return 409 with `{"error":"app is archived; use POST /api/apps/{slug}/purge"}`.
3. Else, current behavior: deployer Teardown, remove `<apps_dir>/<slug>`, `store.PurgeApp(slug)` (replaces today's `DeleteApp` to ensure history is cleaned).
4. Audit `app/purge`, actor=user.

- [ ] **Step 3: Run tests + commit**

```bash
go test ./internal/api/ -count=1
git commit -m "feat(api): archive view, purge endpoint, delete refuses archived"
```

---

## Task 8 (Phase D): UI archive view

**Files:**
- Create: `ui/src/routes/Archive.svelte`
- Create: `ui/src/routes/__tests__/Archive.test.js`
- Modify: `ui/src/lib/api.js` (or equivalent client) to add `listArchived()` and `purgeApp(slug)`.
- Modify: sidebar component (find via `grep -rn "System\|sidebar" ui/src/components/`) to add "Archived apps" link.
- Modify: dashboard list filter (already excluded by API default; verify no client-side filter needed).
- Modify: router config to register `/archive` (likely `ui/src/App.svelte` or `ui/src/router.js`).

- [ ] **Step 1: API client methods**

Add to client:

```js
export async function listArchived() {
  const r = await fetch('/api/apps/archived', { credentials: 'include' })
  if (!r.ok) throw new Error('fetch archived')
  return r.json()
}
export async function purgeApp(slug) {
  const r = await fetch(`/api/apps/${slug}/purge`, { method: 'POST', credentials: 'include' })
  if (!r.ok) throw new Error('purge ' + slug)
}
```

- [ ] **Step 2: Archive.svelte**

Page layout:
- Title: "Archived apps".
- Empty state if list empty: "No archived apps."
- For each row: slug, archived timestamp (relative + absolute tooltip), last domain, expand toggle showing tombstone fields (alert rules, backup configs, access list).
- Per-row "Clean up" button → confirm modal → `purgeApp(slug)` → refresh list. Warn explicitly: "This permanently deletes the app row and all history (audit, deploys, backups, alerts)."

Match design language of existing pages (System, Users) for consistency.

- [ ] **Step 3: Sidebar entry**

Under existing System group, add "Archived apps" link visible only if `archivedCount > 0` (fetch count via `listArchived().length` lazily, or wire into dashboard load). Acceptable initial impl: always show, with a count badge.

- [ ] **Step 4: Vitests**

Mock the api client. Render component with: empty list → empty state shown. With one item → row + expand reveals tombstone fields. Click Clean up → confirm modal → confirm → api called.

- [ ] **Step 5: Build + run**

```bash
cd ui && npm test -- --run
```

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(ui): archived apps view with purge action"
```

---

## Task 9 (Phase D): Documentation

**Files:**
- Create: `docs/operations/archived-apps.md`
- Modify: `docs-site/astro.config.mjs` sidebar to include the new page.

- [ ] **Step 1: Write the page**

Sections:
1. What "archived" means (dir removed from disk, DB row + history retained).
2. How to archive (delete the app's dir, e.g. `rm -rf <apps_dir>/<slug>` or via gitsync).
3. Where archived apps appear (System → Archived apps).
4. What's preserved (full audit/deploy/backup/alert history).
5. How to purge (UI Clean up button or `POST /api/apps/{slug}/purge`).
6. Difference from UI Delete (Delete = full purge, no archive step).
7. Tombstone files (`{data_dir}/archive/<slug>.yml`, format).

- [ ] **Step 2: Commit**

```bash
git commit -m "docs(operations): document archived apps lifecycle"
```

---

## Task 10 (Phase E): E2E spec

**Files:**
- Create: `e2e/tests/20-archive.spec.js` (numbered after 19-cleanup so it runs late but cleanup still runs after to teardown).
  - Actually: 19-cleanup must remain last. Add as `18b-archive.spec.js` between 18 and 19, or rename order. Confirm with `ls e2e/tests/` first; place numerically before cleanup.

- [ ] **Step 1: Spec layout**

```js
import { test, expect } from '@playwright/test'
import { loginAsAdmin } from '../helpers/auth.js'
import { getState } from '../helpers/server.js'
import fs from 'fs'
import path from 'path'

test.describe('archive flow', () => {
  test('removing app dir archives it; archive view lists; purge cleans up', async ({ page }) => {
    await loginAsAdmin(page)
    // Pick an app from the deployed-app fixtures (e.g. e2e-nginx).
    const slug = 'e2e-nginx'
    const { appsDir } = getState()
    fs.rmSync(path.join(appsDir, slug), { recursive: true, force: true })
    // Wait for reconciler watcher (debounce ~1s) plus teardown.
    await page.waitForTimeout(5000)
    // Dashboard should no longer list it.
    await page.goto(`${getState().baseURL}/#/`)
    await expect(page.getByText(slug)).toHaveCount(0)
    // Archive view shows it.
    await page.goto(`${getState().baseURL}/#/archive`)
    await expect(page.locator('main').getByText(slug)).toBeVisible()
    // Clean up.
    await page.getByRole('button', { name: /clean up/i }).first().click()
    await page.getByRole('button', { name: /confirm/i }).click()
    await expect(page.locator('main').getByText(slug)).toHaveCount(0)
  })
})
```

Confirm `getState()` exposes `appsDir`. If not, extend `e2e/helpers/server.js` to expose it.

- [ ] **Step 2: Run locally with the minimal chain**

```bash
cd e2e
npx playwright test 01-setup.spec.js 03-deploy.spec.js 18b-archive.spec.js --reporter=list
```

- [ ] **Step 3: Commit**

```bash
git commit -m "test(e2e): cover archive lifecycle end to end"
```

---

## Task 11 (Phase F): Final integration check

- [ ] **Step 1: Full short test suite**

```bash
go test ./... -short
cd ui && npm test -- --run
```

- [ ] **Step 2: Build**

```bash
make build-go
```

- [ ] **Step 3: Run E2E lite (sanity)**

```bash
make e2e-lite
```

- [ ] **Step 4: Push**

```bash
git push
```

---

## Self-review notes

- Spec coverage: migration ✓ (T1), archived_at ✓ (T1), tombstone ✓ (T4), reconciler archive path ✓ (T6), API list-archived/purge/delete-409 ✓ (T7), UI archive view ✓ (T8), audit ✓ (T5/T6/T7), e2e ✓ (T10), docs ✓ (T9).
- No TBD/TODO placeholders.
- `archiveApp` writes tombstone before marking archived so a partial failure leaves a tombstone (harmless) without an archived flag (next reconcile retries). Conversely, mark-archived after tombstone-write means double-archive races overwrite the same tombstone path atomically (no corruption).
- `removeApp` is retained for the explicit Delete-non-archived API path. No dead code.
