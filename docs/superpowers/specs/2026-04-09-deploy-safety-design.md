# Deploy Safety: Compose Versioning, Rollback, Audit Trail

## Context

Compose files are overwritten on deploy with no way to recover the previous config. No record of who deployed what and when. Users need rollback capability and deploy visibility.

## Design

### 1. Compose Version History

**Migration** `009_deploy_safety.sql`:

```sql
CREATE TABLE compose_versions (
    id INTEGER PRIMARY KEY,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    hash TEXT NOT NULL,
    created_at DATETIME DEFAULT (datetime('now')),
    UNIQUE(app_id, version)
);

CREATE TABLE deploy_events (
    id INTEGER PRIMARY KEY,
    app_slug TEXT NOT NULL,
    action TEXT NOT NULL,
    user_id INTEGER,
    detail TEXT,
    created_at DATETIME DEFAULT (datetime('now'))
);
```

**Store methods** in new file `internal/store/versions.go`:

```go
type ComposeVersion struct {
    ID        int64
    AppID     int64
    Version   int
    Content   string
    Hash      string
    CreatedAt time.Time
}

type DeployEvent struct {
    ID        int64
    AppSlug   string
    Action    string
    UserID    *int64
    Detail    string
    CreatedAt time.Time
}
```

- `CreateComposeVersion(appID int64, content, hash string) error` - inserts with auto-incrementing version per app, prunes versions beyond 10
- `ListComposeVersions(appID int64) ([]ComposeVersion, error)` - ordered by version DESC
- `GetComposeVersion(id int64) (*ComposeVersion, error)`
- `CreateDeployEvent(appSlug, action string, userID *int64, detail string) error`
- `ListDeployEvents(appSlug string) ([]DeployEvent, error)` - ordered by created_at DESC, limit 50

**Version numbering**: `SELECT COALESCE(MAX(version), 0) + 1 FROM compose_versions WHERE app_id = ?`

**Pruning**: after insert, `DELETE FROM compose_versions WHERE app_id = ? AND id NOT IN (SELECT id FROM compose_versions WHERE app_id = ? ORDER BY version DESC LIMIT 10)`

### 2. Version Creation Flow

In `reconciler.deployApp()`, before writing the compose file to disk:
1. Read current compose file content (if exists)
2. Call `store.CreateComposeVersion(appID, content, hash)`

The reconciler already has access to the store. The app ID is known after `UpsertApp`.

Actually, simpler: create the version AFTER UpsertApp (so we have the app ID), using the NEW content being deployed (not the old). This stores each deployed version.

Flow in `deployApp`:
1. Deploy via docker compose
2. UpsertApp (get app ID)
3. Read compose file content
4. CreateComposeVersion with content + hash

### 3. Rollback

**Reconciler method**: `RollbackOne(ctx, slug string, versionID int64) error`
1. Get compose version by ID from store
2. Write content to `{appsDir}/{slug}/docker-compose.yml`
3. Call `deployer.Deploy()` with the restored config
4. UpsertApp with new hash
5. Log deploy event: "rollback to version N"

**API endpoint**: `POST /api/apps/{slug}/rollback` with body `{"version_id": 5}`
- Protected with auth + app access middleware
- Calls `reconciler.RollbackOne()`

### 4. Deploy Audit Trail

**Logging points**:
- `reconciler.deployApp()` - logs "deploy" event after successful deploy
- `reconciler.RollbackOne()` - logs "rollback" event
- User ID passed through context when available (API requests have JWT)

**API endpoint**: `GET /api/apps/{slug}/events` returns `[]DeployEvent`

### 5. Frontend

**Config tab additions**:
- "Deploy History" section below the editor
- Table showing: version number, hash (truncated), timestamp, rollback button
- Rollback button opens confirmation modal, on confirm calls rollback API
- "Deploy Events" section showing action, detail, timestamp

**API functions**:
- `getComposeVersions(slug)` - GET `/apps/{slug}/versions`
- `rollbackApp(slug, versionID)` - POST `/apps/{slug}/rollback`
- `getDeployEvents(slug)` - GET `/apps/{slug}/events`

## Files Changed

| File | Change |
|------|--------|
| `internal/store/migrations/009_deploy_safety.sql` | Create tables |
| `internal/store/versions.go` | New: ComposeVersion/DeployEvent types + CRUD |
| `internal/store/versions_test.go` | New: tests |
| `internal/reconciler/reconciler.go` | Version creation in deployApp, RollbackOne method |
| `internal/reconciler/reconciler_test.go` | Rollback test |
| `internal/api/deploy.go` | Extend reconciler interface |
| `internal/api/actions.go` | Rollback + versions + events handlers |
| `internal/api/actions_test.go` | Tests |
| `internal/api/server.go` | Register routes |
| `ui/src/lib/api.js` | Add API functions |
| `ui/src/routes/AppDetail.svelte` or `ui/src/components/ConfigTab.svelte` | Deploy history + events UI |

## Verification

1. Deploy an app, verify version 1 created in compose_versions
2. Edit and redeploy, verify version 2 created
3. Rollback to version 1, verify compose file restored and app redeployed
4. Deploy 12 times, verify only last 10 versions retained
5. Check deploy_events shows deploy/rollback entries with timestamps
