# System Page Docker-Mode Awareness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect whether SimpleDeploy is running as native binary, Docker, Docker Desktop, or a contributor dev container, expose this on `/api/system/info`, and adapt the System page + footer StatusBar to show mode-appropriate guidance.

**Architecture:** New `internal/deployment` package probes `/.dockerenv` + two env vars once at server start, result cached on `Server` and returned in `simpledeploy` block of system-info response. UI adds a mode badge in the existing `StatusBar`, a new "Deployment" card at the top of System → Overview, a resource-caveat footnote, and three inline helper strings in Maintenance/Logs/Overview tabs. All UI changes are purely conditional on `deployment_mode` — no structural rewrites.

**Tech Stack:** Go 1.x, Svelte 5 (runes), Tailwind, Playwright.

**Spec:** `docs/superpowers/specs/2026-04-18-system-page-docker-mode-design.md`

---

## File Structure

### Go (new)
- `internal/deployment/detect.go` — `Mode` type, constants, `Detect()`, internal `detectConfig`
- `internal/deployment/detect_test.go` — table-driven tests

### Go (modify)
- `internal/api/server.go:37-63` — add `deploymentMode deployment.Mode` field on `Server`; initialize in `NewServer`
- `internal/api/system.go:31-39` — add `DeploymentMode`, `DeploymentLabel` JSON fields on `simpleDeployInfo`
- `internal/api/system.go:108-123` — populate them in `handleSystemInfo`
- `internal/api/system_test.go` — new test (or extend existing) asserting the fields render

### Compose (modify)
- `deploy/docker-compose.dev.yml:25-27` — add `SIMPLEDEPLOY_DEV_MODE: "1"` to env

### UI (modify)
- `ui/src/components/StatusBar.svelte` — insert mode badge span
- `ui/src/routes/System.svelte` — new Deployment card; resource caveat; inline helpers in Maintenance + Logs + Database card

### E2E (modify)
- `e2e/tests/17-system.spec.js` — assert Native badge in StatusBar and Deployment card

---

## Task 1: Deployment detection package

**Files:**
- Create: `internal/deployment/detect.go`
- Create: `internal/deployment/detect_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/deployment/detect_test.go`:

```go
package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tmp := t.TempDir()
	dockerenv := filepath.Join(tmp, ".dockerenv")
	if err := os.WriteFile(dockerenv, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(tmp, "does-not-exist")

	tests := []struct {
		name      string
		path      string
		dev       string
		upstream  string
		want      Mode
	}{
		{"native_no_dockerenv", missing, "", "", ModeNative},
		{"docker_linux_host", dockerenv, "", "", ModeDocker},
		{"docker_desktop", dockerenv, "", "host.docker.internal", ModeDockerDesktop},
		{"docker_dev_wins_over_desktop", dockerenv, "1", "host.docker.internal", ModeDockerDev},
		{"docker_dev_alone", dockerenv, "1", "", ModeDockerDev},
		{"docker_desktop_empty_dev", dockerenv, "", "host.docker.internal", ModeDockerDesktop},
		{"dev_mode_zero_ignored", dockerenv, "0", "host.docker.internal", ModeDockerDesktop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := func(k string) string {
				switch k {
				case "SIMPLEDEPLOY_DEV_MODE":
					return tt.dev
				case "SIMPLEDEPLOY_UPSTREAM_HOST":
					return tt.upstream
				}
				return ""
			}
			got := detect(detectConfig{dockerenvPath: tt.path, env: env})
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLabel(t *testing.T) {
	cases := map[Mode]string{
		ModeNative:        "Native",
		ModeDocker:        "Docker",
		ModeDockerDesktop: "Desktop",
		ModeDockerDev:     "Dev",
		Mode("unknown"):   "",
	}
	for m, want := range cases {
		if got := m.Label(); got != want {
			t.Errorf("%s: got %q, want %q", m, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/deployment/ -v`
Expected: FAIL with "undefined: Mode" / "undefined: detect" etc.

- [ ] **Step 3: Write the implementation**

Create `internal/deployment/detect.go`:

```go
// Package deployment detects how the simpledeploy server binary was launched
// (native vs containerized) so the API can surface mode-appropriate guidance.
package deployment

import "os"

type Mode string

const (
	ModeNative        Mode = "native"
	ModeDocker        Mode = "docker"
	ModeDockerDesktop Mode = "docker-desktop"
	ModeDockerDev     Mode = "docker-dev"
)

// Label returns the short UI-facing string for a mode, or "" if unknown.
func (m Mode) Label() string {
	switch m {
	case ModeNative:
		return "Native"
	case ModeDocker:
		return "Docker"
	case ModeDockerDesktop:
		return "Desktop"
	case ModeDockerDev:
		return "Dev"
	}
	return ""
}

type detectConfig struct {
	dockerenvPath string
	env           func(string) string
}

func detect(c detectConfig) Mode {
	if _, err := os.Stat(c.dockerenvPath); err != nil {
		return ModeNative
	}
	if c.env("SIMPLEDEPLOY_DEV_MODE") == "1" {
		return ModeDockerDev
	}
	if c.env("SIMPLEDEPLOY_UPSTREAM_HOST") != "" {
		return ModeDockerDesktop
	}
	return ModeDocker
}

// Detect probes the runtime environment once and returns the mode.
func Detect() Mode {
	return detect(detectConfig{
		dockerenvPath: "/.dockerenv",
		env:           os.Getenv,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/deployment/ -v`
Expected: PASS all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/deployment/
git commit -m "feat(deployment): detect native/docker/desktop/dev modes"
```

---

## Task 2: Expose deployment mode via `/api/system/info`

**Files:**
- Modify: `internal/api/server.go` (Server struct + NewServer)
- Modify: `internal/api/system.go` (simpleDeployInfo + handleSystemInfo)
- Modify: `internal/api/system_test.go` (or create if it does not exist)

- [ ] **Step 1: Write/extend the failing test**

Check whether `internal/api/system_test.go` exists (`ls internal/api/system_test.go`). If it does not, create it with the minimum harness. If it does, add the subtest below.

Minimal new test file `internal/api/system_test.go` (only create if the file is absent):

```go
package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

func TestSystemInfoDeploymentFields(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	jwt := auth.NewJWTManager("secret-for-test-only-32-bytes!!", 0)
	rl := auth.NewRateLimiter(1000, 60)
	srv := NewServer(0, st, jwt, rl)

	req := httptest.NewRequest("GET", "/api/system/info", nil)
	rr := httptest.NewRecorder()
	srv.handleSystemInfo(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	var body struct {
		SimpleDeploy struct {
			DeploymentMode  string `json:"deployment_mode"`
			DeploymentLabel string `json:"deployment_label"`
		} `json:"simpledeploy"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SimpleDeploy.DeploymentMode == "" {
		t.Error("deployment_mode empty")
	}
	if body.SimpleDeploy.DeploymentLabel == "" {
		t.Error("deployment_label empty")
	}
	// CI environment: no /.dockerenv expected.
	if body.SimpleDeploy.DeploymentMode != "native" {
		t.Errorf("want native in CI, got %q", body.SimpleDeploy.DeploymentMode)
	}
}
```

If `internal/api/system_test.go` already exists, just append `TestSystemInfoDeploymentFields` (adjust imports as needed to avoid duplication). The constructor signature, store/auth helpers, and rate limiter setup here match `NewServer` at `internal/api/server.go:65`; if the signatures drift, align to whatever the current file uses (check other `*_test.go` files in `internal/api/` for the canonical setup).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestSystemInfoDeploymentFields -v`
Expected: FAIL (fields don't exist or are empty strings in JSON).

- [ ] **Step 3: Add field to Server struct and populate in NewServer**

Edit `internal/api/server.go`. Add import for the new package and a field.

At the top of the imports block, add:
```go
	"github.com/vazra/simpledeploy/internal/deployment"
```

In the `Server` struct (around line 37-63), add before the closing brace:
```go
	deploymentMode deployment.Mode
```

In `NewServer` (line 65-76), change the body to:
```go
func NewServer(port int, st *store.Store, jwtMgr *auth.JWTManager, rl *auth.RateLimiter) *Server {
	s := &Server{
		mux:            http.NewServeMux(),
		port:           port,
		store:          st,
		jwt:            jwtMgr,
		rateLimiter:    rl,
		startedAt:      time.Now(),
		deploymentMode: deployment.Detect(),
	}
	s.routes()
	return s
}
```

- [ ] **Step 4: Add JSON fields and populate them**

Edit `internal/api/system.go`.

In `simpleDeployInfo` struct (lines 31-39), add two fields after `GoVersion`:
```go
	GoVersion       string      `json:"go_version"`
	DeploymentMode  string      `json:"deployment_mode"`
	DeploymentLabel string      `json:"deployment_label"`
	Process         processInfo `json:"process"`
```

In `handleSystemInfo` (lines 108-123), populate the fields inside the `sd := simpleDeployInfo{...}` literal after `GoVersion`:
```go
		GoVersion:       runtime.Version(),
		DeploymentMode:  string(s.deploymentMode),
		DeploymentLabel: s.deploymentMode.Label(),
		Process: processInfo{
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/api/ -run TestSystemInfoDeploymentFields -v`
Expected: PASS.

- [ ] **Step 6: Run full Go tests to catch regressions**

Run: `go test ./...`
Expected: PASS (no regressions).

- [ ] **Step 7: Commit**

```bash
git add internal/api/server.go internal/api/system.go internal/api/system_test.go
git commit -m "feat(api): expose deployment_mode/deployment_label on /system/info"
```

---

## Task 3: Add dev-mode env marker to dev compose

**Files:**
- Modify: `deploy/docker-compose.dev.yml:25-27`

- [ ] **Step 1: Edit the environment block**

In `deploy/docker-compose.dev.yml`, find the existing `environment:` block:
```yaml
    environment:
      SIMPLEDEPLOY_UPSTREAM_HOST: host.docker.internal
      SIMPLEDEPLOY_HEALTH_PORT: "8500"
```

Add one line:
```yaml
    environment:
      SIMPLEDEPLOY_UPSTREAM_HOST: host.docker.internal
      SIMPLEDEPLOY_HEALTH_PORT: "8500"
      SIMPLEDEPLOY_DEV_MODE: "1"
```

- [ ] **Step 2: Commit**

```bash
git add deploy/docker-compose.dev.yml
git commit -m "chore(dev-docker): set SIMPLEDEPLOY_DEV_MODE=1 for mode detection"
```

---

## Task 4: StatusBar mode badge

**Files:**
- Modify: `ui/src/components/StatusBar.svelte`

- [ ] **Step 1: Edit StatusBar to insert the badge span**

Replace the full contents of `ui/src/components/StatusBar.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import { statusBar } from '../lib/stores/statusbar.svelte.js'
  import { connection } from '../lib/stores/connection.svelte.js'
  import { formatBytes } from '../lib/format.js'

  const unsubReconnect = connection.onReconnect(() => statusBar.load())
  onMount(() => {
    if (!statusBar.loaded) statusBar.load()
    return unsubReconnect
  })

  const deploymentTitles = {
    native: 'Running as a native binary',
    docker: 'Running inside a Docker container (host networking)',
    'docker-desktop': 'Running inside a Docker Desktop container',
    'docker-dev': 'Running inside a contributor dev container',
  }
</script>

{#if statusBar.loaded}
<a href="#/docker" class="shrink-0 flex items-center gap-x-5 px-4 py-1.5 bg-surface-1 border-t border-border/30 text-[11px] text-text-muted hover:text-text-secondary transition-colors cursor-pointer">
  {#if statusBar.sysInfo?.simpledeploy}
    <span class="flex items-center gap-1.5">
      <svg class="w-3 h-3 text-accent" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>
      SimpleDeploy {statusBar.sysInfo.simpledeploy.version || 'dev'}
    </span>
    {#if statusBar.sysInfo.simpledeploy.deployment_label}
      <span
        class="flex items-center gap-1.5"
        title={deploymentTitles[statusBar.sysInfo.simpledeploy.deployment_mode] || ''}
      >
        {#if statusBar.sysInfo.simpledeploy.deployment_mode === 'native'}
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></svg>
        {:else}
          <svg class="w-3.5 h-3" viewBox="0 0 24 14" fill="currentColor"><path d="M23.1 6.3c-.06-.04-.6-.43-1.74-.43-.3 0-.62.03-.93.08-.23-1.6-1.55-2.38-1.6-2.42l-.33-.19-.21.31c-.27.42-.47.89-.58 1.37-.22.95-.08 1.84.39 2.6-.57.32-1.48.4-1.67.4H.63a.63.63 0 0 0-.63.62 9.46 9.46 0 0 0 .58 3.4c.46 1.2 1.14 2.08 2.03 2.62C3.5 14.7 5.1 15 6.88 15c.8 0 1.6-.07 2.39-.22 1.1-.2 2.16-.58 3.14-1.12a8.64 8.64 0 0 0 2.14-1.74c1.05-1.18 1.68-2.5 2.14-3.66h.2c1.22 0 1.97-.49 2.39-.9.28-.27.5-.58.65-.95l.08-.24-.22-.12zM2.1 7.32h2.07c.1 0 .18-.08.18-.18V5.28a.18.18 0 0 0-.18-.18H2.1a.18.18 0 0 0-.18.18v1.86c0 .1.08.18.18.18zm2.84 0h2.07c.1 0 .18-.08.18-.18V5.28a.18.18 0 0 0-.18-.18H4.94a.18.18 0 0 0-.18.18v1.86c0 .1.08.18.18.18zm2.89 0H9.9c.1 0 .18-.08.18-.18V5.28a.18.18 0 0 0-.18-.18H7.83a.18.18 0 0 0-.18.18v1.86c0 .1.08.18.18.18zm2.84 0h2.07c.1 0 .18-.08.18-.18V5.28a.18.18 0 0 0-.18-.18h-2.07a.18.18 0 0 0-.18.18v1.86c0 .1.08.18.18.18zM7.83 4.68H9.9c.1 0 .18-.08.18-.18V2.64a.18.18 0 0 0-.18-.18H7.83a.18.18 0 0 0-.18.18V4.5c0 .1.08.18.18.18zm2.84 0h2.07c.1 0 .18-.08.18-.18V2.64a.18.18 0 0 0-.18-.18h-2.07a.18.18 0 0 0-.18.18V4.5c0 .1.08.18.18.18zm0-2.64h2.07c.1 0 .18-.08.18-.18V0a.18.18 0 0 0-.18-.18h-2.07a.18.18 0 0 0-.18.18v1.86c0 .1.08.18.18.18zm2.89 2.64h2.07c.1 0 .18-.08.18-.18V2.64a.18.18 0 0 0-.18-.18h-2.07a.18.18 0 0 0-.18.18V4.5c0 .1.08.18.18.18z"/></svg>
        {/if}
        {statusBar.sysInfo.simpledeploy.deployment_label}
      </span>
    {/if}
    <span title="Process memory usage">Mem: {formatBytes(statusBar.sysInfo.simpledeploy.process?.mem_alloc || 0)}</span>
  {/if}
  {#if statusBar.sysInfo?.database}
    <span title="Database size on disk">DB: {formatBytes(statusBar.sysInfo.database.size_bytes || 0)}</span>
  {/if}
  {#if statusBar.dockerInfo}
    <span class="flex items-center gap-1">
      <span class="w-1.5 h-1.5 rounded-full bg-success"></span>
      Docker Engine {statusBar.dockerInfo.server_version}
    </span>
    {#if statusBar.dockerInfo.compose_version}
      <span class="flex items-center gap-1">
        <span class="w-1.5 h-1.5 rounded-full bg-success"></span>
        Compose {statusBar.dockerInfo.compose_version}
      </span>
    {/if}
  {:else}
    <span class="flex items-center gap-1">
      <span class="w-1.5 h-1.5 rounded-full bg-danger"></span>
      Docker unavailable
    </span>
  {/if}
</a>
{/if}
```

- [ ] **Step 2: Build UI and run dev server locally to eyeball**

Run: `make build`
Expected: Completes without errors. (Skip the manual eyeball check if running in CI — Task 8 covers it via E2E.)

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/StatusBar.svelte
git commit -m "feat(ui): show deployment mode badge in StatusBar"
```

---

## Task 5: System → Overview Deployment card + resource caveat

**Files:**
- Modify: `ui/src/routes/System.svelte`

- [ ] **Step 1: Add deployment helpers to the script block**

At the bottom of the `<script>` block in `ui/src/routes/System.svelte` (after the existing `tierBarWidth` function, before the closing `</script>` at line 277), add:

```javascript
  async function copyCmd(cmd) {
    try {
      await navigator.clipboard.writeText(cmd)
      toasts.success('Copied')
    } catch {
      toasts.error('Copy failed')
    }
  }

  const deploymentHeadings = {
    native: { icon: 'monitor', title: 'Native Binary', blurb: 'Running as a native binary on the host OS.' },
    docker: { icon: 'docker', title: 'Docker', blurb: 'Running inside a Docker container with host networking.' },
    'docker-desktop': { icon: 'docker', title: 'Docker Desktop', blurb: 'Running inside Docker Desktop. Resource metrics below reflect the Docker VM, not your host machine.' },
    'docker-dev': { icon: 'docker', title: 'Contributor Dev Container', blurb: 'Running inside the dev container started by `make dev-docker`. Resource metrics reflect the Docker VM.' },
  }

  function deploymentUpgrade(mode) {
    switch (mode) {
      case 'docker':
      case 'docker-desktop':
        return ['cd /etc/simpledeploy', 'docker compose pull && docker compose up -d']
      case 'docker-dev':
        return ['make dev-docker']
      case 'native':
      default:
        return ['# apt install:', 'sudo apt update && sudo apt upgrade simpledeploy', '', '# manual binary swap:', 'sudo systemctl stop simpledeploy', 'sudo mv ~/simpledeploy-new /usr/local/bin/simpledeploy', 'sudo systemctl start simpledeploy']
    }
  }

  function deploymentLogHint(mode) {
    if (mode === 'native') return 'journalctl -u simpledeploy -f'
    return 'docker compose logs -f simpledeploy'
  }

  function resourceCaveat(mode) {
    if (mode === 'native') return ''
    if (mode === 'docker-desktop' || mode === 'docker-dev') return 'Values reflect the Docker VM, not your host.'
    return 'Values reflect container view, not host.'
  }
```

- [ ] **Step 2: Insert the Deployment card at the top of the Overview tab**

Find the line that opens the Overview block (around line 297-298):
```svelte
  {:else if activeTab === 'overview'}
    {#if info}
      <!-- SimpleDeploy -->
      <h2 class="text-base font-medium text-text-primary mb-4">SimpleDeploy</h2>
```

Replace that with:
```svelte
  {:else if activeTab === 'overview'}
    {#if info}
      {#if info.simpledeploy?.deployment_mode && deploymentHeadings[info.simpledeploy.deployment_mode]}
        {@const dh = deploymentHeadings[info.simpledeploy.deployment_mode]}
        <h2 class="text-base font-medium text-text-primary mb-4">Deployment</h2>
        <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-8">
          <div class="flex items-center gap-2 mb-3">
            {#if dh.icon === 'monitor'}
              <svg class="w-5 h-5 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></svg>
            {:else}
              <svg class="w-6 h-4 text-accent" viewBox="0 0 24 14" fill="currentColor"><path d="M23.1 6.3c-.06-.04-.6-.43-1.74-.43-.3 0-.62.03-.93.08-.23-1.6-1.55-2.38-1.6-2.42l-.33-.19-.21.31c-.27.42-.47.89-.58 1.37-.22.95-.08 1.84.39 2.6-.57.32-1.48.4-1.67.4H.63a.63.63 0 0 0-.63.62 9.46 9.46 0 0 0 .58 3.4c.46 1.2 1.14 2.08 2.03 2.62C3.5 14.7 5.1 15 6.88 15c.8 0 1.6-.07 2.39-.22 1.1-.2 2.16-.58 3.14-1.12a8.64 8.64 0 0 0 2.14-1.74c1.05-1.18 1.68-2.5 2.14-3.66h.2c1.22 0 1.97-.49 2.39-.9.28-.27.5-.58.65-.95l.08-.24-.22-.12z"/></svg>
            {/if}
            <span class="text-sm font-semibold text-text-primary">{dh.title}</span>
          </div>
          <p class="text-xs text-text-secondary mb-4">{dh.blurb}</p>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <div class="text-xs font-medium text-text-secondary mb-1">Upgrade</div>
              <div class="space-y-1">
                {#each deploymentUpgrade(info.simpledeploy.deployment_mode) as line}
                  {#if line === ''}
                    <div class="h-1"></div>
                  {:else if line.startsWith('#')}
                    <div class="text-xs text-text-muted font-mono">{line}</div>
                  {:else}
                    <button
                      type="button"
                      onclick={() => copyCmd(line)}
                      class="block w-full text-left text-xs font-mono bg-surface-1 border border-border/30 rounded px-2 py-1 text-text-primary hover:border-accent transition-colors"
                      title="Click to copy"
                    >{line}</button>
                  {/if}
                {/each}
              </div>
            </div>
            <div>
              <div class="text-xs font-medium text-text-secondary mb-1">Process logs</div>
              <button
                type="button"
                onclick={() => copyCmd(deploymentLogHint(info.simpledeploy.deployment_mode))}
                class="block w-full text-left text-xs font-mono bg-surface-1 border border-border/30 rounded px-2 py-1 text-text-primary hover:border-accent transition-colors"
                title="Click to copy"
              >{deploymentLogHint(info.simpledeploy.deployment_mode)}</button>
              {#if info.simpledeploy.deployment_mode !== 'native'}
                <div class="text-xs font-medium text-text-secondary mt-3 mb-1">Backups</div>
                <p class="text-xs text-text-secondary">Backup destination must be a path inside a mounted volume. Default <span class="font-mono text-text-primary">/var/lib/simpledeploy</span> is already mounted in the standard docker-compose.</p>
              {/if}
            </div>
          </div>
        </div>
      {/if}

      <!-- SimpleDeploy -->
      <h2 class="text-base font-medium text-text-primary mb-4">SimpleDeploy</h2>
```

- [ ] **Step 3: Add the resource caveat footnote**

Find the "System Resources" card. The closing `</div>` of the grid appears on roughly line 380 (just before `<!-- Database -->`). Replace the end of that card:
```svelte
          </div>
        </div>
      </div>

      <!-- Database -->
```

with:
```svelte
          </div>
        </div>
        {#if resourceCaveat(info.simpledeploy?.deployment_mode)}
          <p class="text-xs text-text-muted mt-4">{resourceCaveat(info.simpledeploy?.deployment_mode)}</p>
        {/if}
      </div>

      <!-- Database -->
```

- [ ] **Step 4: Add the DB path helper**

Find the Database card's Path field (around line 387-390):
```svelte
          <div>
            <div class="text-xs font-medium text-text-secondary">Path</div>
            <div class="text-sm font-semibold text-text-primary font-mono truncate">{info.database?.path || '-'}</div>
          </div>
```

Replace with:
```svelte
          <div>
            <div class="text-xs font-medium text-text-secondary">Path</div>
            <div class="text-sm font-semibold text-text-primary font-mono truncate">{info.database?.path || '-'}</div>
            {#if info.simpledeploy?.deployment_mode && info.simpledeploy.deployment_mode !== 'native'}
              <div class="text-xs text-text-muted mt-1">Path is inside the container; same-path bind mount ensures it matches on the host.</div>
            {/if}
          </div>
```

- [ ] **Step 5: Run vitest (if any UI unit tests exist) and build**

Run: `cd ui && npm run build && cd ..`
Expected: Completes without errors.

- [ ] **Step 6: Commit**

```bash
git add ui/src/routes/System.svelte
git commit -m "feat(ui): add Deployment card + mode-aware hints to System overview"
```

---

## Task 6: Inline helpers in Maintenance and Logs tabs

**Files:**
- Modify: `ui/src/routes/System.svelte`

- [ ] **Step 1: Add helper text under the Destination Path input**

Find the Destination Path block in the Maintenance tab (around line 631-637):
```svelte
              <div>
                <label class="block text-xs font-medium text-text-secondary mb-1">Destination Path</label>
                <input
                  type="text"
                  bind:value={backupConfig.destination}
                  placeholder="/var/backups/simpledeploy"
                  class="w-full px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent font-mono"
                />
              </div>
```

Replace with:
```svelte
              <div>
                <label class="block text-xs font-medium text-text-secondary mb-1">Destination Path</label>
                <input
                  type="text"
                  bind:value={backupConfig.destination}
                  placeholder="/var/backups/simpledeploy"
                  class="w-full px-3 py-1.5 text-sm bg-surface-1 border border-border/50 rounded-lg text-text-primary focus:outline-none focus:border-accent font-mono"
                />
                {#if info?.simpledeploy?.deployment_mode && info.simpledeploy.deployment_mode !== 'native'}
                  <span class="text-xs text-text-muted mt-1 block">Must be a path inside a mounted volume. <span class="font-mono">/var/lib/simpledeploy</span> is already mounted in the standard docker-compose.</span>
                {/if}
              </div>
```

- [ ] **Step 2: Add log hint under the Process Logs subtitle**

Find the Logs tab header block (around line 768-773):
```svelte
  {:else if activeTab === 'logs'}
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-base font-medium text-text-primary">Process Logs</h2>
          <p class="text-xs text-text-secondary mt-1">SimpleDeploy application logs from the current session.</p>
        </div>
```

Replace with:
```svelte
  {:else if activeTab === 'logs'}
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-base font-medium text-text-primary">Process Logs</h2>
          <p class="text-xs text-text-secondary mt-1">SimpleDeploy application logs from the current session.</p>
          {#if info?.simpledeploy?.deployment_mode && info.simpledeploy.deployment_mode !== 'native'}
            <p class="text-xs text-text-muted mt-1">
              Also available on the host:
              <button type="button" onclick={() => copyCmd('docker compose logs -f simpledeploy')} class="font-mono underline decoration-dotted hover:text-text-primary" title="Click to copy">docker compose logs -f simpledeploy</button>
            </p>
          {/if}
        </div>
```

- [ ] **Step 3: Build**

Run: `cd ui && npm run build && cd ..`
Expected: Completes without errors.

- [ ] **Step 4: Commit**

```bash
git add ui/src/routes/System.svelte
git commit -m "feat(ui): mode-aware hints in backup destination and logs tab"
```

---

## Task 7: E2E test — assert Native badge and Deployment card render

**Files:**
- Modify: `e2e/tests/17-system.spec.js`

- [ ] **Step 1: Read the current file to find the right insertion point**

Run: `cat e2e/tests/17-system.spec.js | head -80` to see existing structure. The spec already logs in and navigates to `/#/system`. Add assertions in the existing Overview test block (look for a test that asserts the "SimpleDeploy" heading or similar on Overview).

- [ ] **Step 2: Add a new test block**

Add this test inside the existing `test.describe` block (pick a location after login setup, alongside the existing overview tests). If unsure where, add at the end of the file's describe body:

```javascript
test('shows Native deployment badge in StatusBar and Deployment card on overview', async ({ page }) => {
  const { baseURL } = getState()
  await page.goto(`${baseURL}/#/system`)
  await page.waitForSelector('aside')

  // StatusBar shows the Native badge (footer).
  await expect(page.locator('a[href="#/docker"]').getByText('Native', { exact: true })).toBeVisible()

  // Overview tab renders the Deployment card.
  const main = page.locator('main')
  await expect(main.getByRole('heading', { name: 'Deployment', exact: true })).toBeVisible()
  await expect(main.getByText('Native Binary', { exact: true })).toBeVisible()

  // The resource caveat is absent in native mode.
  await expect(main.getByText(/Values reflect/)).toHaveCount(0)
})
```

Import helpers if not already imported at the top of the file. A minimal import set other specs use:
```javascript
import { test, expect } from '@playwright/test'
import { loginAsAdmin } from '../helpers/auth.js'
import { getState } from '../helpers/server.js'
```

(Use only what is missing; do not duplicate existing imports.)

Ensure a `test.beforeEach(async ({ page }) => { await loginAsAdmin(page) })` exists in the file; if not, add one at the top of the describe block.

- [ ] **Step 3: Run the single new test**

Run: `cd e2e && npx playwright test tests/17-system.spec.js -g "Native deployment badge" --reporter=list`
Expected: PASS (after prerequisite specs 01-16 have run — E2E tests build state; use `make e2e-lite` for a full shorter run).

If the test fails because prior-state specs haven't run, instead run: `cd .. && make e2e-lite`
Expected: PASS whole lite suite.

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/17-system.spec.js
git commit -m "test(e2e): assert Native deployment badge and Deployment card render"
```

---

## Task 8: Final verification

- [ ] **Step 1: Full Go test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 2: UI build**

Run: `make build`
Expected: PASS.

- [ ] **Step 3: Lite E2E**

Run: `make e2e-lite`
Expected: PASS (all 60-odd specs).

- [ ] **Step 4: Manual smoke of docker modes (one-time)**

Run: `make dev-docker`
Then open the UI, log in, visit `/#/system`. Expect:
- StatusBar shows `🐳 Dev` badge.
- Overview shows a Deployment card titled "Contributor Dev Container".
- Resources card shows "Values reflect the Docker VM, not your host."
- Maintenance → Database Backup shows the mounted-volume helper under Destination Path.
- Logs tab shows the `docker compose logs -f simpledeploy` hint.

Stop with: `make dev-docker-down`.

Record the result in the PR description.

---

## Self-review notes

- **Spec coverage:** Every section of the spec maps to at least one task (detection → T1, API → T2, dev env → T3, StatusBar → T4, Deployment card + resource caveat + DB helper → T5, Maintenance/Logs helpers → T6, E2E → T7, verify → T8). Native upgrade hint shows both apt + binary swap (per resolved open question). Copy-to-clipboard uses inline `navigator.clipboard.writeText` + toast (per resolved open question). StatusBar click target unchanged (per resolved open question).
- **Types:** `Mode` and `Mode.Label()` defined in T1, used in T2. JSON field names `deployment_mode` + `deployment_label` consistent across Go (T2), Svelte access (T4, T5, T6), and E2E (T7).
- **Placeholder scan:** No TBDs. All code blocks present. Step 1 of T2 notes two branches (file exists vs not) with explicit handling. Step 1 of T7 asks the engineer to read the file first, which is a concrete read command, not a placeholder.
