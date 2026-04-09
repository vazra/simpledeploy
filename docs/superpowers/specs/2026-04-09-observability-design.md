# Observability: Status Sync, Health Visibility, Reconciler Update Detection

## Context

App status in the DB can drift from actual Docker state (containers crash but DB says "running"). No visibility into container health check status. Reconciler ignores compose file changes for existing apps.

## Design

### 1. Status Sync

Piggyback on the metrics collector's `Run` loop. After collecting container metrics each tick, sync app statuses.

**New interface** in `internal/metrics/collector.go`:

```go
type StatusSyncer interface {
    ListApps() ([]App, error)        // returns all apps with slug + status
    UpdateAppStatus(slug, status string) error
}
```

The collector takes an optional `StatusSyncer` (can be nil). At the end of each tick:

1. Call `StatusSyncer.ListApps()` to get all apps
2. For each app, check if any container with `com.docker.compose.project=simpledeploy-{slug}` exists in the already-fetched container list
3. If containers found and running -> ensure status is "running"
4. If no containers found and status was "running" -> update to "error"
5. If status is "stopped" -> leave it alone (user intentionally stopped)

The container list is already fetched during `CollectContainers`. Reuse it instead of fetching again.

**Store interface**: The `StatusSyncer` is satisfied by `*store.Store` which already has `ListApps()` and `UpdateAppStatus()`. A thin wrapper adapts the return type.

**Wiring in main.go**: Pass the store to the collector as the syncer.

### 2. Container Health Visibility

**Deployer method**: `Status(ctx, projectName) ([]ServiceStatus, error)`

Runs: `docker compose -p simpledeploy-{app} ps --format json`

Returns parsed output:
```go
type ServiceStatus struct {
    Service string `json:"service"`
    State   string `json:"state"`    // running, exited, restarting
    Health  string `json:"health"`   // healthy, unhealthy, starting, ""
}
```

**Reconciler method**: `AppServices(ctx, slug) ([]ServiceStatus, error)` - delegates to deployer.

**API endpoint**: `GET /api/apps/{slug}/services` - returns `[]ServiceStatus`.

**Frontend**:
- Add `getAppServices(slug)` API function
- Show service cards in the Overview tab with name, state, health badge
- Load on Overview tab activation

### 3. Reconciler Update Detection

**New migration**: `008_compose_hash.sql`
```sql
ALTER TABLE apps ADD COLUMN compose_hash TEXT NOT NULL DEFAULT '';
```

**Hash computation**: SHA256 of compose file contents, stored as hex string.

**Deploy flow change**: `deployApp()` computes hash and stores it via `UpsertApp`.

**Reconcile change**: When an app already exists in the store, compare the hash of the current compose file with the stored hash. If different, redeploy.

**Store changes**: Add `ComposeHash` field to `App` struct. `UpsertApp` stores it.

## Files Changed

| File | Change |
|------|--------|
| `internal/metrics/collector.go` | Add StatusSyncer interface, sync in Run loop |
| `internal/deployer/deployer.go` | Add Status method |
| `internal/deployer/deployer_test.go` | Test for Status |
| `internal/reconciler/reconciler.go` | Add AppServices, update Reconcile for hash check, add loadAppConfig hash |
| `internal/reconciler/reconciler_test.go` | Test hash-based redeploy |
| `internal/store/migrations/008_compose_hash.sql` | Add compose_hash column |
| `internal/store/apps.go` | Add ComposeHash to App, update UpsertApp |
| `internal/api/actions.go` | Add handleGetServices |
| `internal/api/actions_test.go` | Test for services endpoint |
| `internal/api/deploy.go` | Extend reconciler interface with AppServices |
| `internal/api/server.go` | Register services route |
| `ui/src/lib/api.js` | Add getAppServices |
| `ui/src/routes/AppDetail.svelte` | Show service health in Overview |
| `cmd/simpledeploy/main.go` | Pass store as StatusSyncer to collector |

## Verification

1. Stop a container manually (`docker stop`), verify status syncs to "error" within one metrics tick
2. Start it back, verify syncs to "running"
3. App with health check: verify health status shows in UI (healthy/unhealthy/starting)
4. Edit compose file on disk, verify reconciler detects change and redeploys
5. No-change reconcile: verify app is NOT redeployed when hash matches
