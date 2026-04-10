# UI Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-service log viewer, env var editor, domain assignment, and deploy cancellation to SimpleDeploy.

**Architecture:** Features 1 (logs) is UI-only. Features 2 (env) and 3 (domain) add new API endpoints + UI. Feature 4 (cancel) adds deployer state tracking + API + UI. All features are independent and can be built in parallel.

**Tech Stack:** Go (stdlib net/http, gopkg.in/yaml.v3), Svelte 5 (runes), Docker Compose CLI

---

## File Structure

**New files:**
- `internal/api/env.go` - Env var read/write handlers
- `internal/api/env_test.go` - Tests for env handlers
- `internal/api/domain.go` - Domain update handler
- `internal/api/domain_test.go` - Tests for domain handler
- `internal/api/cancel.go` - Deploy cancel handler
- `internal/api/cancel_test.go` - Tests for cancel handler
- `internal/deployer/tracker.go` - In-flight deploy tracking
- `internal/deployer/tracker_test.go` - Tests for tracker
- `ui/src/components/EnvEditor.svelte` - Env var editor component

**Modified files:**
- `internal/api/server.go` - Register new routes (lines 71-157)
- `internal/deployer/deployer.go` - Add tracker to Deployer struct (line 27-29), wrap operations
- `internal/reconciler/reconciler.go` - Add Cancel to AppDeployer interface (line 21-30), add UpdateDomain method
- `ui/src/components/LogViewer.svelte` - Add service selector (lines 1-55)
- `ui/src/routes/AppDetail.svelte` - Add env editor tab, domain input, cancel button
- `ui/src/lib/api.js` - Add new API functions (lines 75-136)

---

### Task 1: Per-Service Log Viewer (UI only)

**Files:**
- Modify: `ui/src/components/LogViewer.svelte`
- Modify: `ui/src/lib/api.js`

- [ ] **Step 1: Add service fetching and selector to LogViewer**

The component already has `services` and `selectedService` state vars (line 10-11) but never populates `services`. Add fetching and a dropdown.

In `ui/src/components/LogViewer.svelte`, replace the `<script>` block:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api.js'

  let { slug, service = '' } = $props()

  let lines = $state([])
  let ws = $state(null)
  let following = $state(true)
  let container
  let services = $state([])
  let selectedService = $state(service)
  let showTimestamps = $state(true)

  onMount(async () => {
    const { data } = await api.getAppServices(slug)
    if (data) {
      services = data.map(s => s.service)
      if (!selectedService && services.length > 0) {
        selectedService = services[0]
      }
    }
    connect()
  })
  onDestroy(() => { if (ws) ws.close() })

  function connect() {
    if (ws) ws.close()
    lines = []
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    let url = `${proto}//${window.location.host}/api/apps/${slug}/logs?follow=true&tail=200`
    if (selectedService) url += `&service=${selectedService}`

    ws = new WebSocket(url)
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      lines = [...lines.slice(-999), msg]
      if (following && container) {
        requestAnimationFrame(() => {
          container.scrollTop = container.scrollHeight
        })
      }
    }
    ws.onclose = () => { ws = null }
  }

  function switchService(svc) {
    selectedService = svc
    connect()
  }

  function toggleFollow() {
    following = !following
    if (following && container) {
      container.scrollTop = container.scrollHeight
    }
  }

  function clear() { lines = [] }

  function downloadLogs() {
    const text = lines.map((l) => `${l.ts || ''} [${l.stream}] ${l.line}`).join('\n')
    const blob = new Blob([text], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${slug}-${selectedService || 'all'}-logs.txt`
    a.click()
    URL.revokeObjectURL(url)
  }
</script>
```

- [ ] **Step 2: Add service selector to the toolbar**

In the template section of `LogViewer.svelte`, add a service selector after the existing toolbar buttons. Replace the toolbar `<div>` (line 58-80):

```svelte
  <div class="flex items-center gap-2 px-3 py-2 bg-surface-1 border border-border rounded-t-lg flex-wrap">
    {#if services.length > 1}
      <div class="flex items-center gap-1">
        {#each services as svc}
          <button
            onclick={() => switchService(svc)}
            class="px-2 py-1 text-xs rounded border transition-colors
              {selectedService === svc ? 'border-accent text-accent bg-accent/10' : 'border-border text-text-secondary hover:text-text-primary'}"
          >
            {svc}
          </button>
        {/each}
      </div>
      <div class="w-px h-4 bg-border"></div>
    {/if}
    <button
      onclick={toggleFollow}
      class="px-2 py-1 text-xs rounded border transition-colors
        {following ? 'border-success text-success' : 'border-border text-text-secondary hover:text-text-primary'}"
    >
      {following ? 'Following' : 'Paused'}
    </button>
    <button onclick={clear} class="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:text-text-primary transition-colors">
      Clear
    </button>
    <button
      onclick={() => showTimestamps = !showTimestamps}
      class="px-2 py-1 text-xs rounded border transition-colors
        {showTimestamps ? 'border-accent text-accent' : 'border-border text-text-secondary hover:text-text-primary'}"
    >
      Timestamps
    </button>
    <button onclick={downloadLogs} class="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:text-text-primary transition-colors">
      Download
    </button>
    <span class="ml-auto text-xs text-text-muted">{lines.length} lines</span>
  </div>
```

- [ ] **Step 3: Test manually**

Run: `cd ui && npm run dev`
Verify: Open an app's logs tab, see service buttons, click different services, logs reconnect.

- [ ] **Step 4: Commit**

```bash
git add ui/src/components/LogViewer.svelte
git commit -m "feat(ui): add per-service log selector"
```

---

### Task 2: Env Var Editor

**Files:**
- Create: `internal/api/env.go`
- Create: `internal/api/env_test.go`
- Create: `ui/src/components/EnvEditor.svelte`
- Modify: `internal/api/server.go`
- Modify: `ui/src/routes/AppDetail.svelte`
- Modify: `ui/src/lib/api.js`

- [ ] **Step 1: Write env handler tests**

Create `internal/api/env_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleGetEnv(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	srv := newTestServer(t, st)

	// Create app dir with .env file
	appDir := filepath.Join(srv.appsDir, "myapp")
	os.MkdirAll(appDir, 0755)
	os.WriteFile(filepath.Join(appDir, ".env"), []byte("FOO=bar\nBAZ=qux\n"), 0644)

	// Seed the app in the store so appAccess works
	seedApp(t, st, "myapp", filepath.Join(appDir, "docker-compose.yml"))

	req := authedRequest(t, srv, "GET", "/api/apps/myapp/env", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rr.Code, rr.Body.String())
	}
	// Response should contain FOO and BAZ
	body := rr.Body.String()
	if !strings.Contains(body, "FOO") || !strings.Contains(body, "BAZ") {
		t.Fatalf("missing env vars in response: %s", body)
	}
}

func TestHandleGetEnv_NoFile(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	srv := newTestServer(t, st)

	appDir := filepath.Join(srv.appsDir, "myapp")
	os.MkdirAll(appDir, 0755)
	seedApp(t, st, "myapp", filepath.Join(appDir, "docker-compose.yml"))

	req := authedRequest(t, srv, "GET", "/api/apps/myapp/env", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	// Should return empty array
	if rr.Body.String() != "[]\n" {
		t.Fatalf("expected empty array, got: %s", rr.Body.String())
	}
}

func TestHandlePutEnv(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	srv := newTestServer(t, st)

	appDir := filepath.Join(srv.appsDir, "myapp")
	os.MkdirAll(appDir, 0755)
	seedApp(t, st, "myapp", filepath.Join(appDir, "docker-compose.yml"))

	body := `[{"key":"DB_HOST","value":"localhost"},{"key":"DB_PORT","value":"5432"}]`
	req := authedRequest(t, srv, "PUT", "/api/apps/myapp/env", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rr.Code, rr.Body.String())
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(appDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "DB_HOST=localhost") || !strings.Contains(content, "DB_PORT=5432") {
		t.Fatalf("unexpected .env content: %s", content)
	}
}
```

Note: You'll need to check how existing API tests set up test helpers. Look at existing test files in `internal/api/` (e.g., `apps_test.go`, `deploy_test.go`) for `testStore`, `newTestServer`, `authedRequest`, and `seedApp` helpers. Adapt the test to use the same patterns. If `seedApp` doesn't exist, you'll need to insert an app row directly via `st.UpsertApp(...)`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run TestHandleGetEnv -v`
Expected: Compilation error (handlers don't exist yet)

- [ ] **Step 3: Implement env handlers**

Create `internal/api/env.go`:

```go
package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type envVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s *Server) handleGetEnv(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	envPath := filepath.Join(filepath.Dir(app.ComposePath), ".env")
	vars, err := parseEnvFile(envPath)
	if err != nil {
		// No .env file is fine, return empty list
		vars = []envVar{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vars)
}

func (s *Server) handlePutEnv(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var vars []envVar
	if err := json.NewDecoder(r.Body).Decode(&vars); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	envPath := filepath.Join(filepath.Dir(app.ComposePath), ".env")
	if err := writeEnvFile(envPath, vars); err != nil {
		http.Error(w, fmt.Sprintf("write .env: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func parseEnvFile(path string) ([]envVar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var vars []envVar
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vars = append(vars, envVar{Key: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
	}
	return vars, scanner.Err()
}

func writeEnvFile(path string, vars []envVar) error {
	var b strings.Builder
	for _, v := range vars {
		fmt.Fprintf(&b, "%s=%s\n", v.Key, v.Value)
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}
```

- [ ] **Step 4: Register routes in server.go**

In `internal/api/server.go`, add after line 121 (the `GET /api/apps/{slug}/services` line):

```go
	// Env vars
	s.mux.Handle("GET /api/apps/{slug}/env", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleGetEnv))))
	s.mux.Handle("PUT /api/apps/{slug}/env", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handlePutEnv))))
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/ -run TestHandleGetEnv -v && go test ./internal/api/ -run TestHandlePutEnv -v`
Expected: PASS

- [ ] **Step 6: Add API functions in ui/src/lib/api.js**

Add these to the `api` object (after line 94, the `getAppServices` line):

```js
  // Env vars
  getEnv: (slug) => request('GET', `/apps/${slug}/env`),
  putEnv: (slug, vars) => requestWithToast('PUT', `/apps/${slug}/env`, vars, 'Environment saved'),
```

- [ ] **Step 7: Create EnvEditor component**

Create `ui/src/components/EnvEditor.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'

  let { slug } = $props()

  let vars = $state([])
  let loading = $state(true)
  let saving = $state(false)
  let showValues = $state(false)

  onMount(async () => {
    const { data } = await api.getEnv(slug)
    if (data) vars = data
    loading = false
  })

  function addVar() {
    vars = [...vars, { key: '', value: '' }]
  }

  function removeVar(index) {
    vars = vars.filter((_, i) => i !== index)
  }

  async function save() {
    saving = true
    const filtered = vars.filter(v => v.key.trim() !== '')
    await api.putEnv(slug, filtered)
    vars = filtered
    saving = false
  }
</script>

{#if loading}
  <div class="text-text-muted text-sm">Loading...</div>
{:else}
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <p class="text-xs text-text-muted">
        Variables are saved to a .env file alongside your compose file. Reference them with {'${VAR_NAME}'} in your compose.
      </p>
      <button
        onclick={() => showValues = !showValues}
        class="px-2 py-1 text-xs rounded border transition-colors
          {showValues ? 'border-accent text-accent' : 'border-border text-text-secondary hover:text-text-primary'}"
      >
        {showValues ? 'Hide values' : 'Show values'}
      </button>
    </div>

    <div class="space-y-2">
      {#each vars as v, i}
        <div class="flex items-center gap-2">
          <input
            bind:value={v.key}
            placeholder="KEY"
            class="flex-1 px-2 py-1.5 text-sm bg-surface-0 border border-border rounded font-mono focus:outline-none focus:border-accent"
          />
          <input
            type={showValues ? 'text' : 'password'}
            bind:value={v.value}
            placeholder="value"
            class="flex-1 px-2 py-1.5 text-sm bg-surface-0 border border-border rounded font-mono focus:outline-none focus:border-accent"
          />
          <button
            onclick={() => removeVar(i)}
            class="px-2 py-1.5 text-xs text-danger hover:text-danger/80 border border-border rounded hover:border-danger/50 transition-colors"
          >
            Remove
          </button>
        </div>
      {/each}
    </div>

    <div class="flex items-center gap-2">
      <button
        onclick={addVar}
        class="px-3 py-1.5 text-xs rounded border border-border text-text-secondary hover:text-text-primary transition-colors"
      >
        + Add variable
      </button>
      <button
        onclick={save}
        disabled={saving}
        class="px-3 py-1.5 text-xs rounded bg-accent text-white hover:bg-accent/90 disabled:opacity-50 transition-colors"
      >
        {saving ? 'Saving...' : 'Save'}
      </button>
    </div>
  </div>
{/if}
```

- [ ] **Step 8: Add Environment tab to AppDetail.svelte**

In `ui/src/routes/AppDetail.svelte`:

1. Add import at the top of the `<script>` block:
```js
import EnvEditor from '../components/EnvEditor.svelte'
```

2. Add `'environment'` to the `tabs` array (line 48):
```js
const tabs = ['overview', 'logs', 'environment', 'metrics', 'backups', 'config']
```

3. Add the tab content after the logs tab section (after the `{:else if activeTab === 'logs'}` block, around line 294-295):
```svelte
  {:else if activeTab === 'environment'}
    <EnvEditor {slug} />
```

- [ ] **Step 9: Commit**

```bash
git add internal/api/env.go internal/api/env_test.go internal/api/server.go \
  ui/src/components/EnvEditor.svelte ui/src/routes/AppDetail.svelte ui/src/lib/api.js
git commit -m "feat(api,ui): add env var editor with .env file management"
```

---

### Task 3: Domain Assignment via Compose Label Rewriting

**Files:**
- Create: `internal/api/domain.go`
- Create: `internal/api/domain_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/reconciler/reconciler.go`
- Modify: `ui/src/routes/AppDetail.svelte`
- Modify: `ui/src/lib/api.js`

- [ ] **Step 1: Write domain handler tests**

Create `internal/api/domain_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleUpdateDomain(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	srv := newTestServer(t, st)

	appDir := filepath.Join(srv.appsDir, "myapp")
	os.MkdirAll(appDir, 0755)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	// Write a compose file with an existing domain label
	compose := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: old.example.com
      simpledeploy.port: "80"
`
	os.WriteFile(composePath, []byte(compose), 0644)
	seedApp(t, st, "myapp", composePath)

	body := `{"domain":"new.example.com"}`
	req := authedRequest(t, srv, "PUT", "/api/apps/myapp/domain", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rr.Code, rr.Body.String())
	}

	// Verify compose file was updated
	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "new.example.com") {
		t.Fatalf("compose file not updated: %s", string(data))
	}
	if strings.Contains(string(data), "old.example.com") {
		t.Fatalf("old domain still present: %s", string(data))
	}
}

func TestHandleUpdateDomain_NoExistingLabel(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	srv := newTestServer(t, st)

	appDir := filepath.Join(srv.appsDir, "myapp")
	os.MkdirAll(appDir, 0755)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	compose := `services:
  web:
    image: nginx
`
	os.WriteFile(composePath, []byte(compose), 0644)
	seedApp(t, st, "myapp", composePath)

	body := `{"domain":"app.example.com"}`
	req := authedRequest(t, srv, "PUT", "/api/apps/myapp/domain", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rr.Code, rr.Body.String())
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "app.example.com") {
		t.Fatalf("domain not added: %s", string(data))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run TestHandleUpdateDomain -v`
Expected: Compilation error

- [ ] **Step 3: Implement domain handler with YAML node manipulation**

Create `internal/api/domain.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

type domainRequest struct {
	Domain string `json:"domain"`
}

func (s *Server) handleUpdateDomain(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var req domainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	if err := updateComposeDomain(app.ComposePath, req.Domain); err != nil {
		http.Error(w, fmt.Sprintf("update compose: %v", err), http.StatusInternalServerError)
		return
	}

	// Trigger reconcile to pick up the new domain for proxy routes
	if s.reconciler != nil {
		go s.reconciler.Reconcile(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "domain": req.Domain})
}

// updateComposeDomain uses yaml.v3 node-level manipulation to update/add
// the simpledeploy.domain label in the compose file, preserving formatting.
func updateComposeDomain(composePath, domain string) error {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf("read compose: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("unexpected YAML structure")
	}
	root := doc.Content[0] // mapping node

	// Find "services" key
	servicesNode := findMapValue(root, "services")
	if servicesNode == nil {
		return fmt.Errorf("no services found in compose file")
	}

	// Find first service
	if servicesNode.Kind != yaml.MappingNode || len(servicesNode.Content) < 2 {
		return fmt.Errorf("no services defined")
	}
	firstServiceNode := servicesNode.Content[1] // value of first service

	// Find or create "labels" in the first service
	labelsNode := findMapValue(firstServiceNode, "labels")
	if labelsNode == nil {
		// Add labels mapping to the service
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "labels", Tag: "!!str"}
		labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		firstServiceNode.Content = append(firstServiceNode.Content, keyNode, labelsNode)
	}

	// Find or create "simpledeploy.domain" in labels
	domainValueNode := findMapValue(labelsNode, "simpledeploy.domain")
	if domainValueNode != nil {
		domainValueNode.Value = domain
	} else {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "simpledeploy.domain", Tag: "!!str"}
		valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: domain, Tag: "!!str"}
		labelsNode.Content = append(labelsNode.Content, keyNode, valNode)
	}

	// Also check all other services and update if they have the label
	for i := 2; i < len(servicesNode.Content); i += 2 {
		svcNode := servicesNode.Content[i+1]
		svcLabels := findMapValue(svcNode, "labels")
		if svcLabels == nil {
			continue
		}
		dv := findMapValue(svcLabels, "simpledeploy.domain")
		if dv != nil {
			dv.Value = domain
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	return os.WriteFile(composePath, out, 0644)
}

// findMapValue finds the value node for a given key in a YAML mapping node.
func findMapValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}
```

- [ ] **Step 4: Register route in server.go**

In `internal/api/server.go`, add after the env routes:

```go
	// Domain
	s.mux.Handle("PUT /api/apps/{slug}/domain", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleUpdateDomain))))
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/ -run TestHandleUpdateDomain -v`
Expected: PASS

- [ ] **Step 6: Add API function in ui/src/lib/api.js**

Add to the `api` object:

```js
  updateDomain: (slug, domain) => requestWithToast('PUT', `/apps/${slug}/domain`, { domain }, 'Domain updated'),
```

- [ ] **Step 7: Add domain input to AppDetail.svelte**

In the overview tab section of `AppDetail.svelte`, add a domain editor. Find the app details/info section in the overview tab and add:

```svelte
<!-- Domain editor - add in the overview tab header area -->
<div class="flex items-center gap-2 mt-2">
  <label class="text-xs text-text-muted">Domain:</label>
  <input
    bind:value={editDomain}
    placeholder="example.com"
    class="px-2 py-1 text-sm bg-surface-0 border border-border rounded font-mono focus:outline-none focus:border-accent w-64"
  />
  {#if editDomain !== (app?.Domain || '')}
    <button
      onclick={saveDomain}
      class="px-2 py-1 text-xs rounded bg-accent text-white hover:bg-accent/90 transition-colors"
    >
      Save
    </button>
  {/if}
</div>
```

Add the state and function in the `<script>` block:

```js
let editDomain = $state('')

// Inside loadApp or after app is fetched:
// editDomain = app?.Domain || ''

async function saveDomain() {
  const { error } = await api.updateDomain(slug, editDomain)
  if (!error) await loadApp()
}
```

Initialize `editDomain` after `loadApp()` succeeds by adding `editDomain = app?.Domain || ''` after the app data is set.

- [ ] **Step 8: Commit**

```bash
git add internal/api/domain.go internal/api/domain_test.go internal/api/server.go \
  ui/src/routes/AppDetail.svelte ui/src/lib/api.js
git commit -m "feat(api,ui): add domain assignment via compose label rewriting"
```

---

### Task 4: Deploy Cancellation

**Files:**
- Create: `internal/deployer/tracker.go`
- Create: `internal/deployer/tracker_test.go`
- Create: `internal/api/cancel.go`
- Create: `internal/api/cancel_test.go`
- Modify: `internal/deployer/deployer.go`
- Modify: `internal/reconciler/reconciler.go`
- Modify: `internal/api/server.go`
- Modify: `ui/src/routes/AppDetail.svelte`
- Modify: `ui/src/lib/api.js`

- [ ] **Step 1: Write tracker tests**

Create `internal/deployer/tracker_test.go`:

```go
package deployer

import (
	"context"
	"testing"
)

func TestTracker_TrackAndCancel(t *testing.T) {
	tr := NewTracker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tr.Track("myapp", cancel)

	if !tr.IsDeploying("myapp") {
		t.Fatal("expected myapp to be deploying")
	}

	err := tr.Cancel("myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// context should be cancelled
	if ctx.Err() == nil {
		t.Fatal("expected context to be cancelled")
	}

	if tr.IsDeploying("myapp") {
		t.Fatal("expected myapp to not be deploying after cancel")
	}
}

func TestTracker_CancelNotFound(t *testing.T) {
	tr := NewTracker()

	err := tr.Cancel("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent app")
	}
}

func TestTracker_Done(t *testing.T) {
	tr := NewTracker()

	_, cancel := context.WithCancel(context.Background())
	tr.Track("myapp", cancel)
	tr.Done("myapp")

	if tr.IsDeploying("myapp") {
		t.Fatal("expected myapp to not be deploying after done")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/deployer/ -run TestTracker -v`
Expected: Compilation error

- [ ] **Step 3: Implement tracker**

Create `internal/deployer/tracker.go`:

```go
package deployer

import (
	"context"
	"fmt"
	"sync"
)

// Tracker tracks in-flight deploy operations and supports cancellation.
type Tracker struct {
	mu      sync.Mutex
	flights map[string]context.CancelFunc
}

func NewTracker() *Tracker {
	return &Tracker{flights: make(map[string]context.CancelFunc)}
}

// Track registers a cancellable deploy for the given slug.
func (t *Tracker) Track(slug string, cancel context.CancelFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flights[slug] = cancel
}

// Done removes a completed deploy from tracking.
func (t *Tracker) Done(slug string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.flights, slug)
}

// Cancel cancels an in-flight deploy and removes it from tracking.
func (t *Tracker) Cancel(slug string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	cancel, ok := t.flights[slug]
	if !ok {
		return fmt.Errorf("no in-flight deploy for %q", slug)
	}
	cancel()
	delete(t.flights, slug)
	return nil
}

// IsDeploying returns true if the slug has an in-flight deploy.
func (t *Tracker) IsDeploying(slug string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.flights[slug]
	return ok
}
```

- [ ] **Step 4: Run tracker tests**

Run: `go test ./internal/deployer/ -run TestTracker -v`
Expected: PASS

- [ ] **Step 5: Integrate tracker into Deployer**

Modify `internal/deployer/deployer.go`:

1. Add `Tracker` field to the `Deployer` struct (line 27-29):
```go
type Deployer struct {
	runner  CommandRunner
	Tracker *Tracker
}
```

2. Initialize tracker in `New` (line 31-38):
```go
func New(runner CommandRunner) (*Deployer, error) {
	d := &Deployer{runner: runner, Tracker: NewTracker()}
	_, stderr, err := d.runner.Run(context.Background(), "docker", "compose", "version")
	if err != nil {
		return nil, fmt.Errorf("docker compose not available: %s: %w", stderr, err)
	}
	return d, nil
}
```

3. Wrap `Deploy` method to track/untrack (line 40-54):
```go
func (d *Deployer) Deploy(ctx context.Context, app *compose.AppConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	d.Tracker.Track(app.Name, cancel)
	defer d.Tracker.Done(app.Name)

	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("compose up: %s: %w", stderr, err)
	}
	return nil
}
```

4. Similarly wrap `Pull` and `Restart` methods with tracking. Same pattern: create cancellable context, `d.Tracker.Track(app.Name, cancel)`, defer `d.Tracker.Done(app.Name)`.

5. Add `Cancel` method:
```go
func (d *Deployer) Cancel(ctx context.Context, app *compose.AppConfig) error {
	if err := d.Tracker.Cancel(app.Name); err != nil {
		return err
	}
	// Reconcile state after cancellation
	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("reconcile after cancel: %s: %w", stderr, err)
	}
	return nil
}
```

- [ ] **Step 6: Update AppDeployer interface in reconciler**

In `internal/reconciler/reconciler.go`, add to the `AppDeployer` interface (line 21-30):

```go
type AppDeployer interface {
	Deploy(ctx context.Context, app *compose.AppConfig) error
	Teardown(ctx context.Context, projectName string) error
	Restart(ctx context.Context, app *compose.AppConfig) error
	Stop(ctx context.Context, projectName string) error
	Start(ctx context.Context, projectName string) error
	Pull(ctx context.Context, app *compose.AppConfig, auths []deployer.RegistryAuth) error
	Scale(ctx context.Context, app *compose.AppConfig, scales map[string]int) error
	Status(ctx context.Context, projectName string) ([]deployer.ServiceStatus, error)
	Cancel(ctx context.Context, app *compose.AppConfig) error
}
```

Add `CancelOne` to the reconciler:

```go
func (r *Reconciler) CancelOne(ctx context.Context, slug string) error {
	app, err := r.loadApp(slug)
	if err != nil {
		return err
	}
	return r.deployer.Cancel(ctx, app)
}
```

Note: Check if `loadApp` helper exists in reconciler. If not, extract the compose file loading pattern from existing methods like `RestartOne`.

- [ ] **Step 7: Write cancel API handler tests**

Create `internal/api/cancel_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCancel_NoInFlight(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()
	srv := newTestServer(t, st)
	seedApp(t, st, "myapp", "/tmp/myapp/docker-compose.yml")

	req := authedRequest(t, srv, "POST", "/api/apps/myapp/cancel", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for no in-flight deploy, got %d: %s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 8: Implement cancel handler**

Create `internal/api/cancel.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	if err := s.reconciler.CancelOne(r.Context(), slug); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}
```

- [ ] **Step 9: Add deploying status to app response**

Modify `handleGetApp` in `internal/api/apps.go` to include `deploying` status. The cleanest way: add a `Deploying` field to the JSON response. This requires the API server to have access to the deployer's tracker. Add a method to the `reconciler` interface or pass tracker directly.

Add `CancelOne` and `IsDeploying` to the `reconciler` interface in `internal/api/deploy.go` (line 16-28):

```go
type reconciler interface {
	DeployOne(ctx context.Context, composePath, appName string) error
	RemoveOne(ctx context.Context, appName string) error
	RestartOne(ctx context.Context, slug string) error
	StopOne(ctx context.Context, slug string) error
	StartOne(ctx context.Context, slug string) error
	PullOne(ctx context.Context, slug string) error
	ScaleOne(ctx context.Context, slug string, scales map[string]int) error
	AppServices(ctx context.Context, slug string) ([]deployer.ServiceStatus, error)
	RollbackOne(ctx context.Context, slug string, versionID int64) error
	ListVersions(ctx context.Context, slug string) ([]store.ComposeVersion, error)
	ListDeployEvents(ctx context.Context, slug string) ([]store.DeployEvent, error)
	CancelOne(ctx context.Context, slug string) error
	IsDeploying(slug string) bool
}
```

Add `IsDeploying` to the reconciler in `internal/reconciler/reconciler.go`:

```go
func (r *Reconciler) IsDeploying(slug string) bool {
	if d, ok := r.deployer.(*deployer.Deployer); ok {
		return d.Tracker.IsDeploying(slug)
	}
	return false
}
```

Modify `handleGetApp` in `internal/api/apps.go` to wrap the response:

```go
func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	type appResponse struct {
		store.App
		Deploying bool `json:"deploying"`
	}

	resp := appResponse{
		App:       *app,
		Deploying: s.reconciler != nil && s.reconciler.IsDeploying(slug),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 10: Register cancel route in server.go**

Add after the domain route:

```go
	// Cancel deploy
	s.mux.Handle("POST /api/apps/{slug}/cancel", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleCancel))))
```

- [ ] **Step 11: Run all tests**

Run: `go test ./internal/deployer/ -v && go test ./internal/api/ -v`
Expected: PASS

- [ ] **Step 12: Add API function and UI**

In `ui/src/lib/api.js`, add:

```js
  cancelDeploy: (slug) => requestWithToast('POST', `/apps/${slug}/cancel`, null, 'Deploy cancelled'),
```

In `ui/src/routes/AppDetail.svelte`, add a cancel button in the action buttons area (around line 184-207, the header area with Restart/Stop/Start buttons):

```svelte
{#if app?.deploying}
  <button
    onclick={cancelDeploy}
    class="px-3 py-1.5 text-xs rounded bg-danger text-white hover:bg-danger/90 transition-colors"
  >
    Cancel Deploy
  </button>
{/if}
```

Add the function in the `<script>` block:

```js
async function cancelDeploy() {
  await api.cancelDeploy(slug)
  await loadApp()
}
```

- [ ] **Step 13: Commit**

```bash
git add internal/deployer/tracker.go internal/deployer/tracker_test.go \
  internal/deployer/deployer.go internal/reconciler/reconciler.go \
  internal/api/cancel.go internal/api/cancel_test.go internal/api/apps.go \
  internal/api/server.go ui/src/routes/AppDetail.svelte ui/src/lib/api.js
git commit -m "feat(api,ui): add deploy cancellation with in-flight tracking"
```

---

### Task 5: Final Integration Test

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

- [ ] **Step 2: Build the full project**

Run: `make build`
Expected: Build succeeds

- [ ] **Step 3: Commit any remaining fixes**

If any tests needed fixes, commit them:
```bash
git add -A
git commit -m "fix: integration fixes for new UI features"
```
