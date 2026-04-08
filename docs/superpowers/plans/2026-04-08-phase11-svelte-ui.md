# Phase 11: Svelte UI - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embedded Svelte SPA dashboard for managing apps, viewing metrics/logs, configuring backups/alerts, and managing users. Compiled and embedded into the Go binary via `go:embed`.

**Architecture:** Svelte 5 + Vite project in `ui/`. Builds to `ui/dist/`. Go embeds `ui/dist/` and serves it as static files. API client in JS wraps all REST endpoints. Client-side routing with hash-based router. Minimal dependencies, small bundle.

**Tech Stack:** Svelte 5, Vite, Chart.js (metrics), xterm.js (logs), svelte-spa-router

---

## File Structure

```
ui/
  package.json
  vite.config.js
  index.html
  src/
    main.js                    - app entry point
    App.svelte                 - root component + router
    lib/
      api.js                   - API client (fetch wrapper)
      auth.js                  - auth state, login/logout
      stores.js                - shared stores
    routes/
      Login.svelte             - login page
      Dashboard.svelte         - system overview + app list
      AppDetail.svelte         - app metrics, logs, config
      Backups.svelte           - backup config + runs
      Alerts.svelte            - alert rules, webhooks, history
      Users.svelte             - user + API key management
    components/
      Layout.svelte            - sidebar + header layout
      MetricsChart.svelte      - Chart.js time series
      LogViewer.svelte         - WebSocket log stream
      AppCard.svelte           - app summary card
      Modal.svelte             - confirmation modal

internal/api/server.go         - Add static file serving for embedded UI
Makefile                       - Add ui-build target
```

---

### Task 1: Svelte Project Setup + Go Embed + Auth

**Files:**
- Create: `ui/` project (package.json, vite.config.js, index.html, src/)
- Create: `ui/src/main.js`, `App.svelte`
- Create: `ui/src/lib/api.js`, `auth.js`
- Create: `ui/src/routes/Login.svelte`
- Create: `ui/src/components/Layout.svelte`
- Modify: `internal/api/server.go` (serve embedded static files)
- Modify: `Makefile` (add ui-build)

#### Svelte project init:

```bash
cd ui
npm create vite@latest . -- --template svelte
npm install
npm install svelte-spa-router
```

#### vite.config.js:
```js
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  }
})
```

#### src/lib/api.js:
```js
const BASE = '/api'

async function request(method, path, body = null) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
  }
  if (body) opts.body = JSON.stringify(body)
  const res = await fetch(BASE + path, opts)
  if (res.status === 401) {
    window.location.hash = '#/login'
    throw new Error('Unauthorized')
  }
  if (!res.ok) throw new Error(await res.text())
  if (res.status === 204) return null
  return res.json()
}

export const api = {
  // Auth
  login: (username, password) => request('POST', '/auth/login', { username, password }),
  logout: () => request('POST', '/auth/logout'),
  setup: (username, password) => request('POST', '/setup', { username, password }),

  // Apps
  listApps: () => request('GET', '/apps'),
  getApp: (slug) => request('GET', `/apps/${slug}`),
  removeApp: (slug) => request('DELETE', `/apps/${slug}`),

  // Metrics
  systemMetrics: (from, to) => request('GET', `/metrics/system?from=${from}&to=${to}`),
  appMetrics: (slug, from, to) => request('GET', `/apps/${slug}/metrics?from=${from}&to=${to}`),
  appRequests: (slug, from, to) => request('GET', `/apps/${slug}/requests?from=${from}&to=${to}`),

  // Backups
  listBackupConfigs: (slug) => request('GET', `/apps/${slug}/backups/configs`),
  createBackupConfig: (slug, cfg) => request('POST', `/apps/${slug}/backups/configs`, cfg),
  deleteBackupConfig: (id) => request('DELETE', `/backups/configs/${id}`),
  listBackupRuns: (slug) => request('GET', `/apps/${slug}/backups/runs`),
  triggerBackup: (slug) => request('POST', `/apps/${slug}/backups/run`),
  restore: (id) => request('POST', `/backups/restore/${id}`),

  // Alerts
  listWebhooks: () => request('GET', '/webhooks'),
  createWebhook: (w) => request('POST', '/webhooks', w),
  deleteWebhook: (id) => request('DELETE', `/webhooks/${id}`),
  listAlertRules: () => request('GET', '/alerts/rules'),
  createAlertRule: (r) => request('POST', '/alerts/rules', r),
  deleteAlertRule: (id) => request('DELETE', `/alerts/rules/${id}`),
  alertHistory: () => request('GET', '/alerts/history'),

  // Users
  listUsers: () => request('GET', '/users'),
  createUser: (u) => request('POST', '/users', u),
  deleteUser: (id) => request('DELETE', `/users/${id}`),
  listAPIKeys: () => request('GET', '/apikeys'),
  createAPIKey: (name) => request('POST', '/apikeys', { name }),
  deleteAPIKey: (id) => request('DELETE', `/apikeys/${id}`),

  // Health
  health: () => request('GET', '/health'),
}
```

#### src/lib/auth.js:
```js
import { writable } from 'svelte/store'

export const user = writable(null)
export const isLoggedIn = writable(false)
```

#### App.svelte (router):
```svelte
<script>
  import Router from 'svelte-spa-router'
  import Login from './routes/Login.svelte'
  import Dashboard from './routes/Dashboard.svelte'
  import AppDetail from './routes/AppDetail.svelte'
  import Backups from './routes/Backups.svelte'
  import Alerts from './routes/Alerts.svelte'
  import Users from './routes/Users.svelte'
  import Layout from './components/Layout.svelte'

  const routes = {
    '/login': Login,
    '/': Dashboard,
    '/apps/:slug': AppDetail,
    '/backups': Backups,
    '/alerts': Alerts,
    '/users': Users,
  }
</script>

<Router {routes} />
```

#### Login.svelte:
```svelte
<script>
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'

  let username = ''
  let password = ''
  let error = ''
  let setupMode = false

  async function handleLogin() {
    try {
      await api.login(username, password)
      push('/')
    } catch (e) {
      error = 'Invalid credentials'
    }
  }

  async function handleSetup() {
    try {
      await api.setup(username, password)
      await api.login(username, password)
      push('/')
    } catch (e) {
      error = e.message
    }
  }
</script>
```

#### Layout.svelte:
Simple sidebar with nav links (Dashboard, Backups, Alerts, Users). Header with logout button.

#### Go embed in server.go:

```go
import "embed"

//go:embed all:../../ui/dist
var uiFS embed.FS

// In routes():
// Serve SPA - fallback to index.html for client-side routing
distFS, _ := fs.Sub(uiFS, "ui/dist")
fileServer := http.FileServer(http.FS(distFS))
s.mux.Handle("/", fileServer)
```

Actually, the embed directive needs to be relative to the Go file. Since server.go is in `internal/api/`, the path would be `../../ui/dist`. But go:embed doesn't support `..`. So we need to embed from a file in the root or in `cmd/simpledeploy/`.

Better approach: embed in cmd/simpledeploy/main.go and pass to server:
```go
//go:embed all:../../ui/dist
var uiFS embed.FS
```

Wait, that won't work either. The embed must be relative to the package dir. Let's create a separate package:

Create `internal/ui/embed.go`:
```go
package ui

import "embed"

//go:embed all:dist
var FS embed.FS
```

But internal/ui/ doesn't contain dist/. The dist is in ui/dist. So we need the embed in the ui/ dir... but that's a JS project, not a Go package.

Simplest approach: put the embed in `cmd/simpledeploy/`:
```go
// In cmd/simpledeploy/main.go or a new file cmd/simpledeploy/ui.go:
//go:embed ui_dist
var uiDistFS embed.FS
```

And have the Makefile copy ui/dist to cmd/simpledeploy/ui_dist before building.

Or use a build script that copies. Let me use the Makefile approach:

```makefile
ui-build:
	cd ui && npm run build
	rm -rf cmd/simpledeploy/ui_dist
	cp -r ui/dist cmd/simpledeploy/ui_dist

build: ui-build
	go build -ldflags="-s -w" -o bin/simpledeploy ./cmd/simpledeploy
```

Then in `cmd/simpledeploy/ui.go`:
```go
package main

import "embed"

//go:embed all:ui_dist
var uiDistFS embed.FS
```

Pass to server, serve as static files with SPA fallback.

#### Makefile update:
```makefile
.PHONY: build test clean ui-build

ui-build:
	cd ui && npm install && npm run build
	rm -rf cmd/simpledeploy/ui_dist
	cp -r ui/dist cmd/simpledeploy/ui_dist

build: ui-build
	go build -ldflags="-s -w" -o bin/simpledeploy ./cmd/simpledeploy

build-go:
	go build -ldflags="-s -w" -o bin/simpledeploy ./cmd/simpledeploy

test:
	go test ./...

clean:
	rm -rf bin/ cmd/simpledeploy/ui_dist
```

- [ ] Steps: create Svelte project, implement api.js, Login, Layout, router, Go embed, Makefile
- [ ] Commit: `git commit -m "add Svelte project with auth, layout, and Go embed"`

---

### Task 2: Dashboard + App List

**Files:**
- Create: `ui/src/routes/Dashboard.svelte`
- Create: `ui/src/components/AppCard.svelte`
- Create: `ui/src/components/MetricsChart.svelte`

```bash
cd ui && npm install chart.js
```

#### Dashboard.svelte:
- System metrics overview (CPU, memory gauges or simple numbers)
- App list with status indicators
- Fetch on mount: api.listApps(), api.systemMetrics()

#### AppCard.svelte:
- App name, status badge (running/stopped/error), domain
- Click navigates to /apps/{slug}

#### MetricsChart.svelte:
- Chart.js line chart for time series
- Props: data points, label, color
- Auto-refresh every 10s

- [ ] Commit: `git commit -m "add dashboard with system metrics and app list"`

---

### Task 3: App Detail (metrics, logs, config)

**Files:**
- Create: `ui/src/routes/AppDetail.svelte`
- Create: `ui/src/components/LogViewer.svelte`

```bash
cd ui && npm install xterm @xterm/addon-fit
```

#### AppDetail.svelte:
- Tab-based layout: Metrics | Logs | Config | Requests
- Metrics tab: CPU/memory charts using MetricsChart
- Logs tab: LogViewer component
- Config tab: show compose labels, domain, status
- Requests tab: request rate, latency, status codes

#### LogViewer.svelte:
- Opens WebSocket to /api/apps/{slug}/logs?follow=true&tail=100
- Uses xterm.js for terminal-like display
- Or simpler: scrollable div with log lines
- Auto-scroll to bottom
- Stop/start follow button

For WebSocket, the URL needs the session cookie. fetch-based WebSocket inherits cookies.

```js
const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
const ws = new WebSocket(`${protocol}//${window.location.host}/api/apps/${slug}/logs?follow=true&tail=100`)

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data)
  addLogLine(msg)
}
```

- [ ] Commit: `git commit -m "add app detail page with metrics charts and log viewer"`

---

### Task 4: Management Pages (backups, alerts, users)

**Files:**
- Create: `ui/src/routes/Backups.svelte`
- Create: `ui/src/routes/Alerts.svelte`
- Create: `ui/src/routes/Users.svelte`
- Create: `ui/src/components/Modal.svelte`

#### Backups.svelte:
- Select app dropdown
- List backup configs for selected app
- Create backup config form
- List backup runs with status
- Trigger backup button
- Restore button (with confirm modal)

#### Alerts.svelte:
- Webhook management (list, create, delete)
- Alert rule management (list, create, delete)
- Alert history timeline

#### Users.svelte (super_admin only):
- User list with roles
- Create user form
- Delete user button
- API key management (create, list, revoke)

#### Modal.svelte:
- Simple confirmation modal: title, message, confirm/cancel buttons

- [ ] Commit: `git commit -m "add backup, alert, and user management pages"`

---

### Task 5: Build + Embed + Tidy

**Files:**
- Create: `cmd/simpledeploy/ui.go` (embed directive)
- Modify: `internal/api/server.go` (serve static files)
- Modify: `cmd/simpledeploy/main.go` (pass UI FS to server)

#### ui.go:
```go
package main

import "embed"

//go:embed all:ui_dist
var uiDistFS embed.FS
```

#### Server static file serving:

Add to Server:
```go
func (s *Server) SetUIFS(fsys fs.FS) {
    // Serve static files, fallback to index.html for SPA routing
    fileServer := http.FileServer(http.FS(fsys))
    s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Try to serve the file directly
        path := r.URL.Path
        if path == "/" { path = "/index.html" }
        f, err := fsys.Open(strings.TrimPrefix(path, "/"))
        if err != nil {
            // SPA fallback: serve index.html for unknown paths
            r.URL.Path = "/index.html"
            fileServer.ServeHTTP(w, r)
            return
        }
        f.Close()
        fileServer.ServeHTTP(w, r)
    }))
}
```

Wire in main.go:
```go
distFS, _ := fs.Sub(uiDistFS, "ui_dist")
srv.SetUIFS(distFS)
```

- [ ] Build: `make build` (runs npm build + go build)
- [ ] Verify: start server, open browser, see login page
- [ ] Run full test suite
- [ ] `go mod tidy`
- [ ] Add `ui/node_modules/`, `ui/dist/`, `cmd/simpledeploy/ui_dist/` to .gitignore
- [ ] Commit: `git commit -m "embed Svelte UI into Go binary"`

---

## Verification Checklist

- [ ] Svelte project builds with `npm run build`
- [ ] Go binary embeds UI static files
- [ ] Login page with setup mode (first-run)
- [ ] Dashboard shows system metrics + app list
- [ ] App detail page with metrics charts
- [ ] Log viewer with WebSocket streaming
- [ ] Backup management (config, trigger, restore)
- [ ] Alert management (webhooks, rules, history)
- [ ] User management (super_admin only)
- [ ] SPA routing works (hash-based)
- [ ] `make build` produces single binary with embedded UI
- [ ] All Go tests pass
