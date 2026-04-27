# Archive-instead-of-delete for apps

Date: 2026-04-27
Status: approved (brainstorm complete, ready for plan)

## Problem

When an app's directory disappears from `apps_dir`, the reconciler tears down containers AND deletes the DB row. This orphans/cascade-loses history that lives DB-side: `audit_log`, `deploy_events`, `backup_runs`, `alert_history`, `compose_versions`. Users lose the historical record of an app they (or a teammate, or a script) removed.

Today's path: `internal/reconciler/reconciler.go` `removeApp()` calls `deployer.Teardown` then `store.DeleteApp`.

## Goals

- Filesystem-removal of an app archives it, preserving DB metadata + history.
- An explicit UI "Delete" action remains the only way to actually purge data.
- Archived apps are hidden from the main dashboard but reachable via a dedicated archive view.
- Archive view is read-only (no restore); user can Purge from there to fully clean up.
- Design is forward-compatible with the FS-authoritative state model (separate spec, same date).

## Non-goals

- Restore-from-archive UX. (User can re-create the app from scratch using `compose_versions` if they want the old config.)
- Time-based auto-purge / recycle-bin retention.
- Cross-app archive bulk operations (out of scope for v1).

## Design

### Triggers

| Event                                       | Result                                                     |
| ------------------------------------------- | ---------------------------------------------------------- |
| App directory removed from `apps_dir`       | Archive: `compose down`, mark `archived_at`, write tombstone |
| UI "Delete" button (or `DELETE /api/apps/{slug}`) | Purge: `compose down`, remove dir, delete DB row + cascade history, no archive step |
| UI "Clean up" on archived app (`POST /api/apps/{slug}/purge`) | Purge: delete DB row + cascade history + remove tombstone (dir already gone) |

### Container handling on archive

Run `docker compose down` for the project. This removes containers, networks, and the project itself, but **preserves named volumes by default**. Matches today's teardown semantics minus the DB delete.

### DB changes

Migration #21 (`021_apps_archived_at.sql`):

```sql
ALTER TABLE apps ADD COLUMN archived_at TIMESTAMP NULL;
CREATE INDEX idx_apps_archived_at ON apps(archived_at) WHERE archived_at IS NOT NULL;
```

`store.ListApps` continues to return all rows; new `store.ListApps(opts)` accepts `IncludeArchived bool` (default false) and an `OnlyArchived bool` for the archive view. Existing call sites updated.

`store.PurgeApp(slug)` performs the cascade: delete from `audit_log`, `deploy_events`, `backup_runs`, `alert_history`, `compose_versions`, `app_labels`, `user_app_access`, `apps`. Single tx. Tombstone file removed after commit.

### Tombstone file

Location: `{data_dir}/archive/<slug>.yml`. Format: a snapshot of the app's `configsync.AppSidecar` at the moment of archive (app meta, alert rules, backup configs, access list). Plus an `archived_at` timestamp at the top.

Purpose:
1. Drives the read-only "last known config" panel in the archive view without re-querying live tables.
2. Survives DB rebuilds from FS in the FS-authoritative model.
3. Removed on Purge.

Format:

```yaml
version: 1
archived_at: 2026-04-27T09:14:23Z
app:
  slug: datafi
  display_name: Datafi
alert_rules:
  - metric: cpu_percent
    operator: ">"
    threshold: 90
    duration_sec: 300
    webhook: ops-alerts
    enabled: true
backup_configs: []
access:
  - username: alice
```

Identical schema to `internal/configsync/types.go AppSidecar` plus the `archived_at` field.

### API

- `GET /api/apps` — unchanged default (excludes archived). Accepts `?include_archived=1` to include archived rows in the list, each marked with `archived_at` field in the response.
- `GET /api/apps/archived` — new, returns archived rows only. Each row includes a `tombstone` field with the parsed YAML snapshot.
- `POST /api/apps/{slug}/purge` — new, archived-only. Performs `store.PurgeApp` and removes tombstone. Returns 404 if app is not archived. Rate-limited / requires admin.
- `DELETE /api/apps/{slug}` — repurposed: purges a non-archived app (compose down + remove dir + DB cascade). For archived apps, returns 409 with hint to use `/purge`.

### UI

- Sidebar entry under "System": "Archived apps." Hidden if no archived apps exist.
- Page lists archived apps with: slug, archived timestamp, last domain, count of preserved history rows (audit/deploys/backups), expandable "Last config" panel (from tombstone).
- Single action per row: "Clean up" (purge confirmation modal warning that history is permanently deleted).
- Main dashboard, search, and metrics views exclude archived apps.

### Reconciler changes

`internal/reconciler/reconciler.go`:

- `Reconcile` loop: when `current` has a row and `desired` does not, instead of `removeApp` (which deletes), call new `archiveApp`.
- `archiveApp(slug)`:
  1. `deployer.Teardown(ctx, projectName)`.
  2. Build tombstone via `configsync.BuildAppSidecar(slug)` (extract from existing `WriteAppSidecar` logic), add `archived_at`, write atomically to `{data_dir}/archive/<slug>.yml` (mode 0644).
  3. `store.MarkAppArchived(slug, now)` — sets `archived_at`, leaves all other rows intact.
  4. Update proxy routes (existing flow) to drop the archived app's routes.
- `removeApp` retained but only invoked from `PurgeApp` API path (not from the reconcile-on-disk-missing path).
- Archived apps are skipped on subsequent reconciles (their dir not present in `desired`, but the DB row no longer triggers the missing-dir branch since `archived_at IS NOT NULL`).

### Store interface additions

```go
func (s *Store) MarkAppArchived(slug string, at time.Time) error
func (s *Store) PurgeApp(slug string) error
func (s *Store) ListArchivedApps() ([]App, error)
type ListAppsOptions struct { IncludeArchived bool; OnlyArchived bool }
func (s *Store) ListAppsWithOptions(opts ListAppsOptions) ([]App, error)
```

`App` struct gains `ArchivedAt sql.NullTime`.

### Audit

Archive and Purge both emit `audit_log` entries via `audit.Recorder`:
- `category=app, action=archive, actor=system|user, slug=<slug>` (system if reconciler-triggered, user if API-triggered indirectly).
- `category=app, action=purge, actor=user, slug=<slug>`.

Pre-rendered summaries added to `internal/audit/render`.

### Tests

- Reconciler: `TestReconcileArchivesOnDirRemoval` — write app, reconcile, remove dir, reconcile, assert: row exists with `archived_at` set, tombstone file present, `Teardown` called, no `DeleteApp` call.
- Reconciler: `TestReconcileSkipsArchivedApp` — archived app with no dir does not retrigger archive on subsequent reconciles.
- Store: archived filter, `PurgeApp` cascade.
- API: `GET /api/apps/archived`, `POST /api/apps/{slug}/purge` (404 on non-archived, 200 on archived, removes tombstone).
- API: `DELETE /api/apps/{slug}` returns 409 for archived apps.
- E2E: extend `19-cleanup.spec.js` (or new `20-archive.spec.js`) — deploy an app, remove its dir from disk via test helper, navigate to archive view, assert listed, click Clean up, assert removed.
- Vitest: archive view component (rendering tombstone, purge confirm modal).

### Forward-compat with FS-authoritative state (separate spec)

- Tombstone files live in `{data_dir}/archive/`, separate from per-app sidecars in `apps_dir`. They survive a DB rebuild because the reload path treats `archive/*.yml` as authoritative archived-app records.
- When the FS-authoritative model lands, startup reload will: scan `archive/`, ensure DB rows exist for each tombstone slug (status=archived), and ensure `archived_at` matches the tombstone. Purge removes both row and tombstone in one tx.

## Migration / rollout

- Single migration #21 adds the column and index.
- No backfill: existing apps remain non-archived. Behavior change starts at the reconciler level: from the upgrade onward, dir-removal archives instead of deletes. Apps deleted before the upgrade are gone.
- `removeApp` is no longer called from the reconcile loop. The function stays for the `DELETE /api/apps/{slug}` and `POST /purge` paths.

## Risks

- Users who relied on `rm -rf <app-dir>` to fully delete an app now find a row + tombstone left behind. Mitigated by clear archive view + Clean up button. Documented in upgrade notes.
- Tombstone disk usage. Bounded: one small YAML per archived app. Negligible compared to compose_versions blobs already kept.

## Open questions

None at finalize-time. All resolved during brainstorming.
