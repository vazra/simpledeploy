# System Page: Docker-Mode Awareness — Design

## Problem

The `#/system` page assumes SimpleDeploy runs as a native binary. When it runs inside a container, the Resources card (RAM/Disk/CPU) reflects the container cgroup view (misleading), the DB path is container-internal (confusing without volume context), and maintenance tips (upgrade command, process log access, backup destination) don't match reality.

## Goal

Detect the deployment mode at startup, expose it to the UI, and adapt the system page (plus a persistent footer badge) so users always see correct guidance for how they installed SimpleDeploy.

## Scope

Four deployment modes, detected once at server startup:

| Mode            | Detection                                                  |
| --------------- | ---------------------------------------------------------- |
| `native`        | `/.dockerenv` absent                                       |
| `docker-dev`    | `/.dockerenv` present AND `SIMPLEDEPLOY_DEV_MODE=1`        |
| `docker-desktop`| `/.dockerenv` present AND `SIMPLEDEPLOY_UPSTREAM_HOST` set |
| `docker`        | `/.dockerenv` present AND neither of the above             |

Non-goals: detecting Podman/containerd, auto-correcting resource metrics, changing the install/upgrade tooling itself.

## Architecture

### 1. Detection — `internal/deployment/`

New tiny package with one exported function:

```go
type Mode string
const (
    ModeNative        Mode = "native"
    ModeDocker        Mode = "docker"
    ModeDockerDesktop Mode = "docker-desktop"
    ModeDockerDev     Mode = "docker-dev"
)

func Detect() Mode
```

Internal `detectConfig{ dockerenvPath string; env func(string) string }` for injection; public `Detect()` wraps it with real filesystem and `os.Getenv`. Result is cached on `Server` as `s.deploymentMode` (computed once in `NewServer`).

### 2. API — `internal/api/system.go`

Extend `simpleDeployInfo`:

```go
DeploymentMode  string `json:"deployment_mode"`  // machine-readable
DeploymentLabel string `json:"deployment_label"` // short display string
```

`handleSystemInfo` reads from `s.deploymentMode`, maps to label:

| Mode             | Label     |
| ---------------- | --------- |
| `native`         | `Native`  |
| `docker`         | `Docker`  |
| `docker-desktop` | `Desktop` |
| `docker-dev`     | `Dev`     |

### 3. Dev compose marker — `deploy/docker-compose.dev.yml`

Add `SIMPLEDEPLOY_DEV_MODE: "1"` to the `environment` block so `docker-dev` is distinguishable from `docker-desktop` (both set `SIMPLEDEPLOY_UPSTREAM_HOST`).

### 4. UI — StatusBar (footer, all pages)

`ui/src/components/StatusBar.svelte`: insert one new span between the "SimpleDeploy vX" span and `Mem:`, showing inline SVG icon + short label:

- `🖥 Native`  (monitor SVG)
- `🐳 Docker`  (docker whale SVG)
- `🐳 Desktop` (docker whale SVG)
- `🐳 Dev`     (docker whale SVG)

Tooltip via `title=` with fuller detail (e.g. `"Running inside Docker Desktop container"`). Rendered only when `deployment_label` is present (backward-safe).

### 5. UI — System → Overview tab

**New "Deployment" card** inserted above the existing "SimpleDeploy" card. Content is mode-specific via `{#if mode === 'x'}` blocks:

- **Native:** icon + label, upgrade hint (`apt upgrade simpledeploy` OR binary swap reference), process log hint (`journalctl -u simpledeploy -f`). No resource caveat.
- **Docker (linux host):** icon + label, "Running in a Docker container with host networking." Resource caveat: "Values reflect container cgroup, not host." Upgrade: `cd /etc/simpledeploy && docker compose pull && docker compose up -d`. Log hint: `docker compose logs -f simpledeploy`. Backup destination note.
- **Docker Desktop:** same as Docker but caveat says "…Docker VM, not your Mac/Windows host." Otherwise identical content.
- **Docker Dev:** same as Desktop but labeled "Contributor dev container" and points upgrade to `make dev-docker`.

Code blocks are click-to-copy (matches existing endpoints-tab behavior).

**Resources card:** when `mode !== 'native'`, append a small footnote under the grid: `"Values reflect container view, not host."` (Desktop: `"…Docker VM, not host."`). No structural change.

### 6. UI — inline helpers in other tabs

**Maintenance → Database Backup:** under the Destination Path input, when `mode !== 'native'` show helper text: `"Must be a path inside a mounted volume. Default /var/lib/simpledeploy is already mounted in the standard docker-compose."`

**Logs tab:** under the "Process Logs" subtitle, when `mode !== 'native'` show one-liner with click-to-copy: `"Also available on the host: docker compose logs -f simpledeploy"`.

**Overview → Database card:** when `mode !== 'native'`, small helper under the Path field: `"Path is inside the container; same-path bind mount ensures it matches on host."`

No per-tab banners. All consolidated tips live in the Deployment card on Overview.

## Data flow

```
Server.NewServer()
  └─> deployment.Detect()  ──cached──> s.deploymentMode

/api/system/info
  └─> handleSystemInfo
      └─> simpledeploy.deployment_mode, deployment_label

UI statusBar.load()   ─┐
UI System.svelte       ├──> render conditionals against deployment_mode
UI nested tabs         ─┘
```

## Testing

### Unit — `internal/deployment/detect_test.go`

Table-driven. Fake `dockerenvPath` (use `t.TempDir()`) + injected `env` function. Cases:

| dockerenv | DEV_MODE  | UPSTREAM_HOST         | expected         |
| --------- | --------- | --------------------- | ---------------- |
| absent    | —         | —                     | `native`         |
| present   | unset     | unset                 | `docker`         |
| present   | `"1"`     | unset                 | `docker-dev`     |
| present   | `"1"`     | `host.docker.internal`| `docker-dev`     |
| present   | unset     | `host.docker.internal`| `docker-desktop` |
| present   | `""`      | `host.docker.internal`| `docker-desktop` |

Precedence: `docker-dev` wins over `docker-desktop` when both markers present.

### API — `internal/api/system_test.go`

Extend existing test (or add one) asserting the response JSON includes `simpledeploy.deployment_mode` and `simpledeploy.deployment_label` with the native values (test runs on CI without `/.dockerenv`).

### E2E — `e2e/tests/17-system.spec.js`

Extend. Assert:
- StatusBar shows the "Native" badge.
- System → Overview renders a "Deployment" card with "Native Binary" label.

No docker-mode E2E. Detection is unit-tested exhaustively; docker-mode rendering is a pure `{#if}` over a field — manual `make dev-docker` smoke check covers the UI variant.

## Rollout

Single PR. No migrations. Backward-safe on both axes:
- Older UI + newer server: ignores new fields (harmless).
- Newer UI + older server: conditionals hide when `deployment_label` is missing.

## Out of scope

- Auto-correcting resource metrics (cgroup v2 reads would be a separate effort).
- Detecting Podman, containerd, LXC, Kubernetes pods.
- Changing how `docker-compose.dev.yml` is referenced in `make dev-docker`.
- Surfacing container image tag, container ID, or hostname.
