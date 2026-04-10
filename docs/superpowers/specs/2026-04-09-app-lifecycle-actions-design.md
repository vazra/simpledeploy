# App Lifecycle Actions

## Context

Users can deploy and delete apps but have no way to restart, stop/start, pull new images, or scale services through the UI or API. These are basic operational actions needed for day-to-day app management.

## Design

### Deployer Methods

All new methods on `internal/deployer/deployer.go`. Each takes `ctx` and the project name (or AppConfig for methods needing the compose path).

**Restart** - force-recreate all containers:
```
docker compose -f {composePath} -p simpledeploy-{app} up -d --force-recreate --remove-orphans
```

**Stop** - stop containers without removing them:
```
docker compose -p simpledeploy-{app} stop
```

**Start** - start previously stopped containers:
```
docker compose -p simpledeploy-{app} start
```

**Pull** - pull latest images then redeploy:
```
docker compose -f {composePath} -p simpledeploy-{app} pull
docker compose -f {composePath} -p simpledeploy-{app} up -d --remove-orphans
```

**Scale** - set replica count per service:
```
docker compose -f {composePath} -p simpledeploy-{app} up -d --scale web=3 --scale worker=2 --no-recreate --remove-orphans
```
`--no-recreate` avoids restarting existing containers, only adds/removes replicas.

### Deployer Interface

Extend `AppDeployer` in `internal/reconciler/reconciler.go`:

```go
type AppDeployer interface {
    Deploy(ctx context.Context, app *compose.AppConfig) error
    Teardown(ctx context.Context, projectName string) error
    Restart(ctx context.Context, app *compose.AppConfig) error
    Stop(ctx context.Context, projectName string) error
    Start(ctx context.Context, projectName string) error
    Pull(ctx context.Context, app *compose.AppConfig) error
    Scale(ctx context.Context, app *compose.AppConfig, scales map[string]int) error
}
```

### API Reconciler Interface

The API server has its own `reconciler` interface in `internal/api/deploy.go`. Extend it to expose the new actions. The reconciler wraps each deployer call with store updates (status changes).

New reconciler methods:
- `RestartOne(ctx, slug)` - calls deployer.Restart, sets status "running"
- `StopOne(ctx, slug)` - calls deployer.Stop, sets status "stopped"
- `StartOne(ctx, slug)` - calls deployer.Start, sets status "running"
- `PullOne(ctx, slug)` - calls deployer.Pull, sets status "running"
- `ScaleOne(ctx, slug, scales)` - calls deployer.Scale, status stays "running"

All methods look up the app's compose path via the store and parse the compose file.

### API Endpoints

New handlers in `internal/api/actions.go`:

| Route | Method | Body | Handler |
|-------|--------|------|---------|
| `POST /api/apps/{slug}/restart` | POST | - | handleRestart |
| `POST /api/apps/{slug}/stop` | POST | - | handleStop |
| `POST /api/apps/{slug}/start` | POST | - | handleStart |
| `POST /api/apps/{slug}/pull` | POST | - | handlePull |
| `POST /api/apps/{slug}/scale` | POST | `{"scales":{"svc":n}}` | handleScale |

All protected with `authMiddleware` + `appAccessMiddleware`. Return 200 with `{"status": "ok"}` on success.

Register in `server.go` routes under the deploy/remove section.

### API Reconciler Interface Update

The `reconciler` interface in `internal/api/deploy.go` currently has:
```go
type reconciler interface {
    DeployOne(ctx, composePath, appName string) error
    RemoveOne(ctx, appName string) error
}
```

Extend to:
```go
type reconciler interface {
    DeployOne(ctx context.Context, composePath, appName string) error
    RemoveOne(ctx context.Context, appName string) error
    RestartOne(ctx context.Context, slug string) error
    StopOne(ctx context.Context, slug string) error
    StartOne(ctx context.Context, slug string) error
    PullOne(ctx context.Context, slug string) error
    ScaleOne(ctx context.Context, slug string, scales map[string]int) error
}
```

### Frontend

**api.js** - add functions:
```javascript
restartApp: (slug) => requestWithToast('POST', `/apps/${slug}/restart`, null, 'App restarted'),
stopApp: (slug) => requestWithToast('POST', `/apps/${slug}/stop`, null, 'App stopped'),
startApp: (slug) => requestWithToast('POST', `/apps/${slug}/start`, null, 'App started'),
pullApp: (slug) => requestWithToast('POST', `/apps/${slug}/pull`, null, 'Images pulled & redeployed'),
scaleApp: (slug, scales) => requestWithToast('POST', `/apps/${slug}/scale`, { scales }, 'App scaled'),
```

**AppDetail.svelte** header buttons:

Next to existing Delete button, add:
- **Restart** button (secondary variant) with confirmation modal
- **Stop** button (secondary) when status is "running"; **Start** button when "stopped"
- **Pull & Update** button (secondary)
- **Scale** button (secondary) - opens a modal with number inputs per service (service names from app data or compose parse)

After any action succeeds, reload app data to reflect new status.

**Scale Modal**: shows each service name with a number input (default 1). "Apply" button sends the scales object.

### Store Changes

No new tables needed. The existing `UpdateAppStatus(slug, status)` handles status changes. The actions just call this after the docker compose command succeeds.

### Files Changed

| File | Change |
|------|--------|
| `internal/deployer/deployer.go` | Add Restart, Stop, Start, Pull, Scale methods |
| `internal/deployer/deployer_test.go` | Tests for new methods |
| `internal/reconciler/reconciler.go` | Extend AppDeployer interface, add wrapper methods |
| `internal/reconciler/reconciler_test.go` | Tests for new wrapper methods |
| `internal/api/deploy.go` | Extend reconciler interface |
| `internal/api/actions.go` | New file: handlers for restart/stop/start/pull/scale |
| `internal/api/actions_test.go` | Tests for new handlers |
| `internal/api/server.go` | Register new routes |
| `ui/src/lib/api.js` | Add API functions |
| `ui/src/routes/AppDetail.svelte` | Action buttons + scale modal |

## Verification

1. Deploy an app, verify restart works (containers recreated)
2. Stop app, verify status changes to "stopped", containers stopped but not removed
3. Start stopped app, verify status back to "running"
4. Pull, verify images updated and containers redeployed
5. Scale web=3, verify 3 replicas running
6. All API endpoints return proper errors for missing apps
7. UI buttons reflect current status (stop/start toggle)
