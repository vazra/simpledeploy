# Deployer Refactor: Shell Out to Docker Compose CLI

## Context

The current deployer reimplements Docker container lifecycle via the Docker API (NetworkCreate, ImagePull, ContainerCreate, etc.). This causes issues on redeploy (network already exists), lacks compose features (depends_on ordering, health checks, build, orphan cleanup), and will always lag behind what `docker compose` provides natively.

Replace the deployer internals with `docker compose` CLI calls. The deployer interface stays the same so the reconciler and CLI commands need no changes.

## Design

### Deployer Constructor

```go
type Deployer struct {
    dockerBin string // path to docker binary
}

func New() (*Deployer, error) {
    // Find docker binary in PATH
    bin, err := exec.LookPath("docker")
    // Verify: docker compose version
    // Return error if not available
}
```

No longer takes a `docker.Client`. The Docker client is still used by metrics/logs/health check but not by the deployer.

### Deploy

```bash
docker compose -f <composePath> -p simpledeploy-<appName> up -d --force-recreate --remove-orphans
```

- `-p simpledeploy-<appName>`: project name prefix for isolation
- `--force-recreate`: ensures containers are rebuilt on redeploy
- `--remove-orphans`: cleans up services removed from the compose file
- `-d`: detached mode

### Teardown

```bash
docker compose -p simpledeploy-<appName> down --remove-orphans --volumes=false
```

Does not remove volumes (user data). Only stops containers and removes the network.

### Command Execution

A `CommandRunner` interface for testability:

```go
type CommandRunner interface {
    Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

type ExecRunner struct{} // real implementation using exec.CommandContext

type MockRunner struct { // test mock
    Calls []RunCall
    Err   error
}
```

The deployer takes a `CommandRunner` instead of calling `exec.Command` directly.

### Label Migration

Docker compose automatically sets `com.docker.compose.project` and `com.docker.compose.service` on containers. With project name `simpledeploy-<app>`:

- `com.docker.compose.project` = `simpledeploy-<app>`
- `com.docker.compose.service` = `<service>`

**metrics/collector.go** changes:
- Filter: `label=com.docker.compose.project` with prefix `simpledeploy-`
- Extract slug: strip `simpledeploy-` prefix from `com.docker.compose.project`

**api/logs.go** changes:
- Find container via `ContainerList` with filters:
  - `label=com.docker.compose.project=simpledeploy-<slug>`
  - `label=com.docker.compose.service=<service>`
- Use first matching container's ID for logs
- Removes hardcoded container name assumption

**Existing deployments**: containers deployed before this refactor will lose metrics/logs until redeployed. Accepted trade-off for clean break.

### docker.Client Interface Cleanup

8 methods become unused by the deployer: `NetworkCreate`, `NetworkRemove`, `ImagePull`, `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerRemove`.

`ContainerList` is still needed by metrics and the new logs lookup. Keep it in the interface.

Remove unused methods from `docker.Client` interface and `DockerClient` implementation. Keep: `Ping`, `Close`, `ContainerList`, `ContainerStats`, `ContainerLogs`.

### main.go Wiring

```go
// Before:
dc, _ := docker.NewClient()
dep := deployer.New(dc)
rec := reconciler.New(db, dep, proxy, appsDir)

// After:
dc, _ := docker.NewClient()
dep, _ := deployer.New()  // no docker client
rec := reconciler.New(db, dep, proxy, appsDir)
// dc still used for metrics collector, logs handler, health check
```

### Testing

**Unit tests (mock runner)**:
- Verify Deploy() calls `docker compose ... up` with correct args
- Verify Teardown() calls `docker compose ... down` with correct args
- Verify error propagation from non-zero exit codes
- Verify constructor fails when docker compose is unavailable

**Integration tests (real Docker, skipped when unavailable)**:
- Deploy a simple compose file, verify containers running
- Redeploy with changed config, verify update applied
- Teardown, verify containers removed
- Gated with `if testing.Short() { t.Skip() }` or Docker availability check

### Error Handling

- Non-zero exit: wrap stderr in error message
- Binary not found: fail at constructor time with clear message
- Timeout: use context deadline from caller

## Files Changed

| File | Change |
|------|--------|
| `internal/deployer/deployer.go` | Rewrite: exec-based Deploy/Teardown |
| `internal/deployer/runner.go` | New: CommandRunner interface + ExecRunner + MockRunner |
| `internal/deployer/deployer_test.go` | Rewrite: mock runner tests |
| `internal/deployer/integration_test.go` | New: real Docker integration tests |
| `internal/docker/client.go` | Remove unused methods from interface |
| `internal/docker/docker.go` | Remove unused method implementations |
| `internal/docker/mock.go` | Remove unused mock methods |
| `internal/metrics/collector.go` | Switch to compose labels |
| `internal/api/logs.go` | Find container by compose labels |
| `cmd/simpledeploy/main.go` | Update deployer construction (no docker.Client) |

## Verification

1. `go test ./internal/deployer/` - unit tests with mock runner
2. `go test ./internal/deployer/ -run Integration` - integration tests (needs Docker)
3. `go test ./internal/metrics/` - metrics collector tests
4. `go test ./internal/api/ -run Logs` - logs handler tests
5. Manual: deploy an app via UI, verify metrics and logs work, redeploy via compose editor, verify no errors
