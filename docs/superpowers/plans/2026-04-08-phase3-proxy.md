# Phase 3: Reverse Proxy & TLS - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed Caddy as a reverse proxy. Apps with `simpledeploy.domain` labels get automatic HTTPS routing. TLS modes: auto (ACME), custom (cert/key), off. Management API stays on its own port.

**Architecture:** Caddy runs embedded in the same process, configured via JSON config. When apps are deployed/removed, the proxy rebuilds the Caddy config and reloads. A `Proxy` interface allows mock testing. The reconciler calls proxy.SetRoutes after each reconciliation to update routing.

**Tech Stack:** Caddy v2 (embedded library), existing reconciler/deployer/compose packages

---

## File Structure

```
internal/proxy/proxy.go           - Proxy interface, CaddyProxy implementation, config builder
internal/proxy/proxy_test.go      - Config builder tests, mock proxy tests
internal/proxy/route.go           - Route types, upstream resolution from compose AppConfig
internal/proxy/route_test.go      - Route resolution tests
internal/proxy/mock.go            - MockProxy for reconciler tests

internal/reconciler/reconciler.go - Add proxy field, call SetRoutes after reconcile
internal/reconciler/reconciler_test.go - Update tests with mock proxy

cmd/simpledeploy/main.go          - Wire proxy into serve command
```

---

### Task 1: Route Types and Upstream Resolution

**Files:**
- Create: `internal/proxy/route.go`
- Create: `internal/proxy/route_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/proxy/route_test.go`:

```go
package proxy

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestResolveRouteBasic(t *testing.T) {
	app := &compose.AppConfig{
		Name:   "myapp",
		Domain: "myapp.example.com",
		Port:   "80",
		TLS:    "auto",
		Services: []compose.ServiceConfig{
			{
				Name:  "web",
				Image: "nginx:latest",
				Ports: []compose.PortMapping{
					{Host: "8080", Container: "80"},
				},
			},
		},
	}

	route, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("ResolveRoute() error: %v", err)
	}

	if route.Domain != "myapp.example.com" {
		t.Errorf("Domain = %q, want myapp.example.com", route.Domain)
	}
	if route.Upstream != "localhost:8080" {
		t.Errorf("Upstream = %q, want localhost:8080", route.Upstream)
	}
	if route.TLS != "auto" {
		t.Errorf("TLS = %q, want auto", route.TLS)
	}
	if route.AppSlug != "myapp" {
		t.Errorf("AppSlug = %q, want myapp", route.AppSlug)
	}
}

func TestResolveRouteNoDomain(t *testing.T) {
	app := &compose.AppConfig{
		Name:     "myapp",
		Services: []compose.ServiceConfig{{Name: "web", Image: "nginx:latest"}},
	}

	_, err := ResolveRoute(app)
	if err == nil {
		t.Fatal("expected error for app without domain")
	}
}

func TestResolveRoutePortLookup(t *testing.T) {
	app := &compose.AppConfig{
		Name:   "myapp",
		Domain: "myapp.example.com",
		Port:   "3000",
		Services: []compose.ServiceConfig{
			{
				Name: "web",
				Ports: []compose.PortMapping{
					{Host: "9090", Container: "3000"},
					{Host: "9091", Container: "3001"},
				},
			},
		},
	}

	route, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("ResolveRoute() error: %v", err)
	}
	if route.Upstream != "localhost:9090" {
		t.Errorf("Upstream = %q, want localhost:9090", route.Upstream)
	}
}

func TestResolveRouteDefaultPort(t *testing.T) {
	// no simpledeploy.port label, use first port mapping
	app := &compose.AppConfig{
		Name:   "myapp",
		Domain: "myapp.example.com",
		Services: []compose.ServiceConfig{
			{
				Name:  "web",
				Ports: []compose.PortMapping{{Host: "5000", Container: "5000"}},
			},
		},
	}

	route, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("ResolveRoute() error: %v", err)
	}
	if route.Upstream != "localhost:5000" {
		t.Errorf("Upstream = %q, want localhost:5000", route.Upstream)
	}
}

func TestResolveRouteTLSDefault(t *testing.T) {
	app := &compose.AppConfig{
		Name:   "myapp",
		Domain: "myapp.example.com",
		Services: []compose.ServiceConfig{
			{Name: "web", Ports: []compose.PortMapping{{Host: "80", Container: "80"}}},
		},
	}

	route, _ := ResolveRoute(app)
	if route.TLS != "auto" {
		t.Errorf("TLS = %q, want auto (default)", route.TLS)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/proxy/ -v`
Expected: FAIL

- [ ] **Step 3: Implement route types and resolution**

Create `internal/proxy/route.go`:

```go
package proxy

import (
	"fmt"

	"github.com/vazra/simpledeploy/internal/compose"
)

type Route struct {
	AppSlug  string
	Domain   string
	Upstream string // "localhost:{port}"
	TLS      string // "auto", "custom", "off"
}

// ResolveRoute builds a proxy Route from a compose AppConfig.
// Returns error if app has no domain label or no resolvable port.
func ResolveRoute(app *compose.AppConfig) (*Route, error) {
	if app.Domain == "" {
		return nil, fmt.Errorf("app %q has no simpledeploy.domain label", app.Name)
	}

	hostPort, err := resolveHostPort(app)
	if err != nil {
		return nil, fmt.Errorf("resolve port for %q: %w", app.Name, err)
	}

	tls := app.TLS
	if tls == "" {
		tls = "auto"
	}

	return &Route{
		AppSlug:  app.Name,
		Domain:   app.Domain,
		Upstream: fmt.Sprintf("localhost:%s", hostPort),
		TLS:      tls,
	}, nil
}

// resolveHostPort finds the host port that maps to the container port
// specified by simpledeploy.port label. If no label, uses the first port mapping found.
func resolveHostPort(app *compose.AppConfig) (string, error) {
	targetPort := app.Port

	for _, svc := range app.Services {
		for _, p := range svc.Ports {
			if targetPort == "" {
				return p.Host, nil
			}
			if p.Container == targetPort {
				return p.Host, nil
			}
		}
	}

	return "", fmt.Errorf("no port mapping found for container port %q", targetPort)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/proxy/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/
git commit -m "add proxy route types and upstream resolution"
```

---

### Task 2: Proxy Interface, Mock, and Caddy Config Builder

**Files:**
- Create: `internal/proxy/proxy.go`
- Create: `internal/proxy/mock.go`
- Add to: `internal/proxy/proxy_test.go`

- [ ] **Step 1: Add Caddy dependency**

```bash
go get github.com/caddyserver/caddy/v2@latest
```

This will take a while and add many transitive dependencies.

- [ ] **Step 2: Create mock proxy**

Create `internal/proxy/mock.go`:

```go
package proxy

import "sync"

type MockProxy struct {
	mu     sync.Mutex
	routes []Route
	running bool
}

func NewMockProxy() *MockProxy {
	return &MockProxy{}
}

func (m *MockProxy) SetRoutes(routes []Route) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = make([]Route, len(routes))
	copy(m.routes, routes)
	return nil
}

func (m *MockProxy) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	return nil
}

func (m *MockProxy) Routes() []Route {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Route, len(m.routes))
	copy(result, m.routes)
	return result
}

func (m *MockProxy) HasRoute(domain string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.routes {
		if r.Domain == domain {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Implement Proxy interface and CaddyProxy**

Create `internal/proxy/proxy.go`:

```go
package proxy

import (
	"encoding/json"
	"fmt"
	"sync"

	caddy "github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
)

type Proxy interface {
	SetRoutes(routes []Route) error
	Stop() error
}

type CaddyProxy struct {
	mu       sync.Mutex
	routes   []Route
	listenAddr string
	tlsMode  string
	tlsEmail string
}

type CaddyConfig struct {
	ListenAddr string // e.g. ":443"
	TLSMode    string // "auto", "custom", "off"
	TLSEmail   string // ACME account email
}

func NewCaddyProxy(cfg CaddyConfig) *CaddyProxy {
	return &CaddyProxy{
		listenAddr: cfg.ListenAddr,
		tlsMode:    cfg.TLSMode,
		tlsEmail:   cfg.TLSEmail,
	}
}

func (p *CaddyProxy) SetRoutes(routes []Route) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.routes = routes
	return p.reload()
}

func (p *CaddyProxy) Stop() error {
	return caddy.Stop()
}

func (p *CaddyProxy) reload() error {
	cfg := p.buildConfig()
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal caddy config: %w", err)
	}
	return caddy.Load(cfgJSON, true)
}

// buildConfig creates the Caddy JSON config from current routes.
func (p *CaddyProxy) buildConfig() map[string]interface{} {
	routes := make([]map[string]interface{}, 0, len(p.routes))

	for _, r := range p.routes {
		route := map[string]interface{}{
			"match": []map[string]interface{}{
				{"host": []string{r.Domain}},
			},
			"handle": []map[string]interface{}{
				{
					"handler":   "reverse_proxy",
					"upstreams": []map[string]string{{"dial": r.Upstream}},
				},
			},
		}
		routes = append(routes, route)
	}

	server := map[string]interface{}{
		"listen": []string{p.listenAddr},
		"routes": routes,
	}

	// TLS configuration
	if p.tlsMode == "off" {
		server["automatic_https"] = map[string]interface{}{
			"disable": true,
		}
	}

	config := map[string]interface{}{
		"admin": map[string]interface{}{
			"disabled": true,
		},
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": map[string]interface{}{
					"proxy": server,
				},
			},
		},
	}

	// ACME email for auto TLS
	if p.tlsMode == "auto" && p.tlsEmail != "" {
		config["apps"].(map[string]interface{})["tls"] = map[string]interface{}{
			"automation": map[string]interface{}{
				"policies": []map[string]interface{}{
					{
						"issuers": []map[string]interface{}{
							{
								"module": "acme",
								"email":  p.tlsEmail,
							},
						},
					},
				},
			},
		}
	}

	return config
}

// BuildConfigJSON is exported for testing the config builder.
func (p *CaddyProxy) BuildConfigJSON() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return json.MarshalIndent(p.buildConfig(), "", "  ")
}
```

- [ ] **Step 4: Write config builder tests**

Add to `internal/proxy/proxy_test.go`:

```go
func TestBuildConfigNoRoutes(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "auto",
		TLSEmail:   "admin@example.com",
	})

	data, err := p.BuildConfigJSON()
	if err != nil {
		t.Fatalf("BuildConfigJSON() error: %v", err)
	}

	var cfg map[string]interface{}
	json.Unmarshal(data, &cfg)

	apps := cfg["apps"].(map[string]interface{})
	httpApp := apps["http"].(map[string]interface{})
	servers := httpApp["servers"].(map[string]interface{})
	proxy := servers["proxy"].(map[string]interface{})

	listen := proxy["listen"].([]interface{})
	if listen[0] != ":443" {
		t.Errorf("listen = %v, want :443", listen[0])
	}

	routes := proxy["routes"].([]interface{})
	if len(routes) != 0 {
		t.Errorf("routes len = %d, want 0", len(routes))
	}
}

func TestBuildConfigWithRoutes(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "auto",
	})
	p.routes = []Route{
		{AppSlug: "app1", Domain: "app1.example.com", Upstream: "localhost:3000", TLS: "auto"},
		{AppSlug: "app2", Domain: "app2.example.com", Upstream: "localhost:4000", TLS: "auto"},
	}

	data, _ := p.BuildConfigJSON()

	var cfg map[string]interface{}
	json.Unmarshal(data, &cfg)

	apps := cfg["apps"].(map[string]interface{})
	httpApp := apps["http"].(map[string]interface{})
	servers := httpApp["servers"].(map[string]interface{})
	proxy := servers["proxy"].(map[string]interface{})
	routes := proxy["routes"].([]interface{})

	if len(routes) != 2 {
		t.Fatalf("routes len = %d, want 2", len(routes))
	}

	r1 := routes[0].(map[string]interface{})
	match := r1["match"].([]interface{})
	hosts := match[0].(map[string]interface{})["host"].([]interface{})
	if hosts[0] != "app1.example.com" {
		t.Errorf("route 1 host = %v, want app1.example.com", hosts[0])
	}

	handle := r1["handle"].([]interface{})
	handler := handle[0].(map[string]interface{})
	if handler["handler"] != "reverse_proxy" {
		t.Errorf("handler = %v, want reverse_proxy", handler["handler"])
	}
	upstreams := handler["upstreams"].([]interface{})
	upstream := upstreams[0].(map[string]interface{})
	if upstream["dial"] != "localhost:3000" {
		t.Errorf("upstream dial = %v, want localhost:3000", upstream["dial"])
	}
}

func TestBuildConfigTLSOff(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":80",
		TLSMode:    "off",
	})

	data, _ := p.BuildConfigJSON()

	var cfg map[string]interface{}
	json.Unmarshal(data, &cfg)

	apps := cfg["apps"].(map[string]interface{})
	httpApp := apps["http"].(map[string]interface{})
	servers := httpApp["servers"].(map[string]interface{})
	proxy := servers["proxy"].(map[string]interface{})

	autoHTTPS := proxy["automatic_https"].(map[string]interface{})
	if autoHTTPS["disable"] != true {
		t.Error("expected automatic_https.disable = true for TLS off")
	}
}

func TestMockProxySetRoutes(t *testing.T) {
	m := NewMockProxy()

	m.SetRoutes([]Route{
		{Domain: "app.example.com", Upstream: "localhost:3000"},
	})

	if !m.HasRoute("app.example.com") {
		t.Error("expected route for app.example.com")
	}
	if m.HasRoute("other.example.com") {
		t.Error("unexpected route for other.example.com")
	}

	routes := m.Routes()
	if len(routes) != 1 {
		t.Errorf("routes len = %d, want 1", len(routes))
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/proxy/ -v`
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/proxy/ go.mod go.sum
git commit -m "add Caddy proxy with config builder and mock"
```

---

### Task 3: Wire Proxy into Reconciler

**Files:**
- Modify: `internal/reconciler/reconciler.go`
- Modify: `internal/reconciler/reconciler_test.go`

- [ ] **Step 1: Add proxy to Reconciler**

Update `Reconciler` struct and `New` to accept an optional `proxy.Proxy`:

```go
type Reconciler struct {
    store    *store.Store
    deployer *deployer.Deployer
    proxy    proxy.Proxy // can be nil
    appsDir  string
}

func New(st *store.Store, d *deployer.Deployer, p proxy.Proxy, appsDir string) *Reconciler {
    return &Reconciler{store: st, deployer: d, proxy: p, appsDir: appsDir}
}
```

- [ ] **Step 2: Update Reconcile to call SetRoutes**

After deploying/removing apps, build routes from all current desired apps and call `proxy.SetRoutes`:

```go
func (r *Reconciler) Reconcile(ctx context.Context) error {
    desired, err := r.scanAppsDir()
    // ... existing deploy/remove logic ...

    // update proxy routes
    if r.proxy != nil {
        r.updateProxyRoutes(desired)
    }
    return nil
}

func (r *Reconciler) updateProxyRoutes(apps map[string]*compose.AppConfig) {
    var routes []proxy.Route
    for _, app := range apps {
        route, err := proxy.ResolveRoute(app)
        if err != nil {
            continue // app without domain, skip
        }
        routes = append(routes, *route)
    }
    if err := r.proxy.SetRoutes(routes); err != nil {
        fmt.Fprintf(os.Stderr, "reconciler: update proxy routes: %v\n", err)
    }
}
```

Also update `DeployOne` and `RemoveOne` to trigger a full route rebuild by calling `Reconcile` or rebuilding routes from the store.

- [ ] **Step 3: Update all call sites**

Update all places that call `reconciler.New()` to pass the proxy parameter:
- `cmd/simpledeploy/main.go`: pass `nil` for now (Task 5 will wire the real proxy)
- `internal/reconciler/reconciler_test.go`: pass `proxy.NewMockProxy()`

- [ ] **Step 4: Add proxy assertions to reconciler tests**

Add to existing tests:

```go
func TestReconcileUpdatesProxyRoutes(t *testing.T) {
    r, _, _, appsDir := newTestEnv(t)
    mockProxy := r.proxy.(*proxy.MockProxy)

    writeComposeFile(t, appsDir, "myapp")

    r.Reconcile(context.Background())

    if !mockProxy.HasRoute("myapp.example.com") {
        t.Error("expected proxy route for myapp.example.com")
    }
}

func TestReconcileRemoveUpdatesProxy(t *testing.T) {
    r, _, _, appsDir := newTestEnv(t)
    mockProxy := r.proxy.(*proxy.MockProxy)

    writeComposeFile(t, appsDir, "myapp")
    r.Reconcile(context.Background())

    os.RemoveAll(filepath.Join(appsDir, "myapp"))
    r.Reconcile(context.Background())

    if mockProxy.HasRoute("myapp.example.com") {
        t.Error("expected proxy route removed for myapp.example.com")
    }
}
```

Update `newTestEnv` to create and inject a MockProxy.

- [ ] **Step 5: Run all tests**

Run: `go test ./... -timeout 30s`
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/reconciler/ cmd/simpledeploy/main.go
git commit -m "wire proxy into reconciler for route updates"
```

---

### Task 4: Wire Proxy into Serve Command

**Files:**
- Modify: `cmd/simpledeploy/main.go`

- [ ] **Step 1: Create and start CaddyProxy in runServe**

Update `runServe` to create a `CaddyProxy` and pass it to the reconciler:

```go
func runServe(cmd *cobra.Command, args []string) error {
    // ... existing config, store, docker setup ...

    // create proxy
    var prx proxy.Proxy
    proxyCfg := proxy.CaddyConfig{
        ListenAddr: cfg.ListenAddr,
        TLSMode:    cfg.TLS.Mode,
        TLSEmail:   cfg.TLS.Email,
    }
    caddyProxy := proxy.NewCaddyProxy(proxyCfg)
    prx = caddyProxy

    dep := deployer.New(dc)
    rec := reconciler.New(db, dep, prx, cfg.AppsDir)

    // ... rest of serve (watcher, API server) ...

    // stop proxy on shutdown
    defer caddyProxy.Stop()
}
```

- [ ] **Step 2: Verify build**

Run: `make build`
Expected: builds (binary will be larger due to Caddy)

Run: `ls -lh bin/simpledeploy`
Note the binary size increase.

- [ ] **Step 3: Commit**

```bash
git add cmd/simpledeploy/main.go
git commit -m "wire Caddy proxy into serve command"
```

---

### Task 5: Tidy and Full Verification

**Files:**
- No new files

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -timeout 30s`
Expected: all tests pass

- [ ] **Step 2: Tidy dependencies**

Run: `go mod tidy`

- [ ] **Step 3: Verify clean build**

Run: `make clean && make build && ls -lh bin/simpledeploy`

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "tidy dependencies for Phase 3"
```

---

## Verification Checklist

- [ ] `proxy.ResolveRoute()` maps compose AppConfig to Route (domain + upstream)
- [ ] Caddy config builder produces valid JSON with routes, TLS settings
- [ ] TLS off mode disables automatic HTTPS
- [ ] TLS auto mode configures ACME with email
- [ ] MockProxy records routes for testing
- [ ] Reconciler updates proxy routes after deploy/remove
- [ ] Serve command starts CaddyProxy
- [ ] All tests pass
- [ ] Binary builds successfully
