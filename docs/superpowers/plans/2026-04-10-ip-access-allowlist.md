# IP Access Allowlist Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-app IP allowlisting via a new Caddy handler module, compose label, API endpoint, and UI.

**Architecture:** New `simpledeploy_ipaccess` Caddy module runs first in the handler pipeline. Allowlist stored as `simpledeploy.access.allow` compose label (single source of truth). API edits the compose file directly (same as domain). UI shows inline editor in app detail overview.

**Tech Stack:** Go stdlib `net` (IP/CIDR parsing), Caddy module API, Svelte 5

---

### Task 1: Add `AccessAllow` to compose label parsing

**Files:**
- Modify: `internal/compose/parser.go:13` (AppConfig struct)
- Modify: `internal/compose/parser.go:60` (LabelConfig struct)
- Modify: `internal/compose/parser.go:143` (ExtractLabels function)
- Modify: `internal/compose/parser.go:117-133` (ParseFile label assignment)
- Test: `internal/compose/parser_test.go`

- [ ] **Step 1: Write failing test for AccessAllow label extraction**

Add to `internal/compose/parser_test.go` inside `TestExtractLabels`:

```go
// Add to the labels map at top of TestExtractLabels:
"simpledeploy.access.allow": "10.0.0.0/8,192.168.1.5",

// Add assertion at the bottom of TestExtractLabels:
if lc.AccessAllow != "10.0.0.0/8,192.168.1.5" {
    t.Errorf("AccessAllow = %q, want %q", lc.AccessAllow, "10.0.0.0/8,192.168.1.5")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/compose/ -run TestExtractLabels -v`
Expected: FAIL - `lc.AccessAllow undefined`

- [ ] **Step 3: Add AccessAllow to AppConfig, LabelConfig, and ExtractLabels**

In `internal/compose/parser.go`, add `AccessAllow string` to `AppConfig` (after `Registries string` on line 28):

```go
AccessAllow     string
```

Add `AccessAllow string` to `LabelConfig` (after `Registries string` on line 71):

```go
AccessAllow     string
```

In `ExtractLabels` function, add to the return struct (after the `Registries` line):

```go
AccessAllow:     labels["simpledeploy.access.allow"],
```

In `ParseFile`, add to the cfg assignment (after `Registries: lc.Registries,` on line 130):

```go
AccessAllow:     lc.AccessAllow,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/compose/ -run TestExtractLabels -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/compose/parser.go internal/compose/parser_test.go
git commit -m "feat(compose): add simpledeploy.access.allow label parsing"
```

---

### Task 2: Add `AllowedIPs` to Route and wire through ResolveRoute

**Files:**
- Modify: `internal/proxy/route.go:12` (Route struct)
- Modify: `internal/proxy/route.go:30` (ResolveRoute function)
- Test: `internal/proxy/route_test.go`

- [ ] **Step 1: Write failing test for AllowedIPs in ResolveRoute**

Add to `internal/proxy/route_test.go`:

```go
func TestResolveRouteWithAllowedIPs(t *testing.T) {
	app := makeApp("secured", "secure.example.com", "8080", "auto", []compose.ServiceConfig{
		{Name: "web", Ports: ports("3000", "8080")},
	})
	app.AccessAllow = "10.0.0.0/8, 192.168.1.5, bad-entry, 172.16.0.0/12"

	r, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// bad-entry should be skipped, 3 valid entries remain
	if len(r.AllowedIPs) != 3 {
		t.Errorf("AllowedIPs: got %d entries, want 3: %v", len(r.AllowedIPs), r.AllowedIPs)
	}
	want := []string{"10.0.0.0/8", "192.168.1.5", "172.16.0.0/12"}
	for i, w := range want {
		if i >= len(r.AllowedIPs) {
			break
		}
		if r.AllowedIPs[i] != w {
			t.Errorf("AllowedIPs[%d]: got %q, want %q", i, r.AllowedIPs[i], w)
		}
	}
}

func TestResolveRouteEmptyAllowedIPs(t *testing.T) {
	app := makeApp("open", "open.example.com", "", "", []compose.ServiceConfig{
		{Name: "web", Ports: ports("5000", "80")},
	})
	// No AccessAllow set
	r, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.AllowedIPs != nil {
		t.Errorf("AllowedIPs: got %v, want nil", r.AllowedIPs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/proxy/ -run TestResolveRouteWithAllowedIPs -v`
Expected: FAIL - `r.AllowedIPs undefined`

- [ ] **Step 3: Add AllowedIPs to Route and parsing logic to ResolveRoute**

In `internal/proxy/route.go`, add to imports:

```go
"log"
"net"
"strings"
```

Add `AllowedIPs` field to Route struct (after `RateLimit`):

```go
AllowedIPs []string // validated IPs and CIDRs
```

Add the following after the rate limit block in `ResolveRoute` (after line 68, before `return route, nil`):

```go
if app.AccessAllow != "" {
    for _, entry := range strings.Split(app.AccessAllow, ",") {
        entry = strings.TrimSpace(entry)
        if entry == "" {
            continue
        }
        if net.ParseIP(entry) != nil {
            route.AllowedIPs = append(route.AllowedIPs, entry)
            continue
        }
        if _, _, err := net.ParseCIDR(entry); err == nil {
            route.AllowedIPs = append(route.AllowedIPs, entry)
            continue
        }
        log.Printf("[proxy] ignoring invalid IP/CIDR in access.allow: %q", entry)
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/proxy/ -run TestResolveRoute -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/route.go internal/proxy/route_test.go
git commit -m "feat(proxy): parse AllowedIPs in ResolveRoute"
```

---

### Task 3: IP access Caddy module and global registry

**Files:**
- Create: `internal/proxy/ipaccess.go`
- Create: `internal/proxy/ipaccess_test.go`

- [ ] **Step 1: Write failing tests for IPAccessRegistry and handler**

Create `internal/proxy/ipaccess_test.go`:

```go
package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPAccessRegistryNoRules(t *testing.T) {
	reg := newIPAccessRegistry()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	if !reg.Allowed("example.com", req) {
		t.Error("no rules for domain should allow all traffic")
	}
}

func TestIPAccessRegistryExactIP(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{"10.0.0.1", "192.168.1.5"})

	allowed := httptest.NewRequest("GET", "/", nil)
	allowed.RemoteAddr = "192.168.1.5:9999"
	if !reg.Allowed("example.com", allowed) {
		t.Error("exact IP match should be allowed")
	}

	blocked := httptest.NewRequest("GET", "/", nil)
	blocked.RemoteAddr = "5.5.5.5:9999"
	if reg.Allowed("example.com", blocked) {
		t.Error("non-matching IP should be blocked")
	}
}

func TestIPAccessRegistryCIDR(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{"10.0.0.0/8"})

	allowed := httptest.NewRequest("GET", "/", nil)
	allowed.RemoteAddr = "10.50.100.200:1234"
	if !reg.Allowed("example.com", allowed) {
		t.Error("IP in CIDR range should be allowed")
	}

	blocked := httptest.NewRequest("GET", "/", nil)
	blocked.RemoteAddr = "11.0.0.1:1234"
	if reg.Allowed("example.com", blocked) {
		t.Error("IP outside CIDR should be blocked")
	}
}

func TestIPAccessRegistryMixedRules(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{"10.0.0.0/8", "203.0.113.5"})

	tests := []struct {
		addr string
		want bool
	}{
		{"10.1.2.3:80", true},
		{"203.0.113.5:80", true},
		{"203.0.113.6:80", false},
		{"192.168.1.1:80", false},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = tt.addr
		got := reg.Allowed("example.com", req)
		if got != tt.want {
			t.Errorf("Allowed(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestIPAccessRegistryEmptyList(t *testing.T) {
	reg := newIPAccessRegistry()
	reg.Set("example.com", []string{})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	if !reg.Allowed("example.com", req) {
		t.Error("empty allowlist should allow all traffic (treated as disabled)")
	}
}

func TestIPAccessHandlerBlocks(t *testing.T) {
	orig := IPAccessRules
	defer func() { IPAccessRules = orig }()

	IPAccessRules = newIPAccessRegistry()
	IPAccessRules.Set("secure.com", []string{"10.0.0.1"})

	h := &IPAccessHandler{}

	// Allowed request
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "secure.com"
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	if err := h.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	if w.Code == http.StatusNotFound {
		t.Error("allowed IP should not get 404")
	}

	// Blocked request
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Host = "secure.com"
	req2.RemoteAddr = "5.5.5.5:1234"
	w2 := httptest.NewRecorder()
	if err := h.ServeHTTP(w2, req2, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	if w2.Code != http.StatusNotFound {
		t.Errorf("blocked IP: got status %d, want 404", w2.Code)
	}
}

func TestIPAccessHandlerNoRules(t *testing.T) {
	orig := IPAccessRules
	defer func() { IPAccessRules = orig }()

	IPAccessRules = newIPAccessRegistry()

	h := &IPAccessHandler{}
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "open.com"
	req.RemoteAddr = "1.2.3.4:5678"
	w := httptest.NewRecorder()
	if err := h.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	if w.Code == http.StatusNotFound {
		t.Error("no rules should allow all traffic")
	}
}

func TestIPAccessHandlerModuleInfo(t *testing.T) {
	h := IPAccessHandler{}
	info := h.CaddyModule()
	if info.ID != "http.handlers.simpledeploy_ipaccess" {
		t.Errorf("module ID: got %q, want %q", info.ID, "http.handlers.simpledeploy_ipaccess")
	}
	if info.New == nil {
		t.Error("New is nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/proxy/ -run TestIPAccess -v`
Expected: FAIL - compilation error, types not defined

- [ ] **Step 3: Implement IPAccessRegistry and IPAccessHandler**

Create `internal/proxy/ipaccess.go`:

```go
package proxy

import (
	"net"
	"net/http"
	"sync"

	caddy "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// IPAccessRules is the package-level registry used by the Caddy handler.
var IPAccessRules = newIPAccessRegistry()

type parsedAllowlist struct {
	ips  []net.IP
	nets []*net.IPNet
}

// ipAccessRegistry maps domains to their parsed allowlists.
type ipAccessRegistry struct {
	mu    sync.RWMutex
	rules map[string]*parsedAllowlist
}

func newIPAccessRegistry() *ipAccessRegistry {
	return &ipAccessRegistry{rules: make(map[string]*parsedAllowlist)}
}

// Set registers or replaces the allowlist for a domain.
// Entries are pre-parsed into net.IP and net.IPNet for fast lookup.
func (reg *ipAccessRegistry) Set(domain string, entries []string) {
	parsed := &parsedAllowlist{}
	for _, entry := range entries {
		if ip := net.ParseIP(entry); ip != nil {
			parsed.ips = append(parsed.ips, ip)
			continue
		}
		if _, ipNet, err := net.ParseCIDR(entry); err == nil {
			parsed.nets = append(parsed.nets, ipNet)
		}
	}
	reg.mu.Lock()
	reg.rules[domain] = parsed
	reg.mu.Unlock()
}

// Remove deletes the allowlist for a domain.
func (reg *ipAccessRegistry) Remove(domain string) {
	reg.mu.Lock()
	delete(reg.rules, domain)
	reg.mu.Unlock()
}

// Allowed returns true if the request should be allowed for the domain.
// Returns true when no rules are configured or the allowlist is empty.
func (reg *ipAccessRegistry) Allowed(domain string, r *http.Request) bool {
	reg.mu.RLock()
	al, ok := reg.rules[domain]
	reg.mu.RUnlock()

	if !ok {
		return true
	}
	// Empty allowlist = no restriction
	if len(al.ips) == 0 && len(al.nets) == 0 {
		return true
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	clientIP := net.ParseIP(host)
	if clientIP == nil {
		return false
	}

	for _, ip := range al.ips {
		if ip.Equal(clientIP) {
			return true
		}
	}
	for _, ipNet := range al.nets {
		if ipNet.Contains(clientIP) {
			return true
		}
	}
	return false
}

// --- Caddy module ---

func init() {
	caddy.RegisterModule(IPAccessHandler{})
}

// IPAccessHandler is a Caddy middleware that enforces per-domain IP allowlists.
type IPAccessHandler struct{}

func (IPAccessHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.simpledeploy_ipaccess",
		New: func() caddy.Module { return new(IPAccessHandler) },
	}
}

func (h *IPAccessHandler) Provision(_ caddy.Context) error { return nil }
func (h *IPAccessHandler) Validate() error                 { return nil }

func (h *IPAccessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if !IPAccessRules.Allowed(r.Host, r) {
		http.NotFound(w, r)
		return nil
	}
	return next.ServeHTTP(w, r)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/proxy/ -run TestIPAccess -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/ipaccess.go internal/proxy/ipaccess_test.go
git commit -m "feat(proxy): add simpledeploy_ipaccess Caddy module"
```

---

### Task 4: Wire IP access into proxy SetRoutes and buildConfig

**Files:**
- Modify: `internal/proxy/proxy.go:43-54` (SetRoutes)
- Modify: `internal/proxy/proxy.go:85-97` (buildConfig handlers)
- Test: `internal/proxy/proxy_test.go`

- [ ] **Step 1: Write failing test for ipaccess handler in Caddy config**

Add to `internal/proxy/proxy_test.go`:

```go
func TestBuildConfigHandlerOrder(t *testing.T) {
	p := newTestProxy("off", "")
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "app.example.com", Upstream: "localhost:3000"},
	}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	server := getServer(t, cfg)
	routes := server["routes"].([]interface{})
	r := routes[0].(map[string]interface{})
	handleList := r["handle"].([]interface{})

	// Expect 4 handlers: ipaccess, ratelimit, metrics, reverse_proxy
	if len(handleList) != 4 {
		t.Fatalf("handle: got %d handlers, want 4", len(handleList))
	}

	wantOrder := []string{"simpledeploy_ipaccess", "simpledeploy_ratelimit", "simpledeploy_metrics", "reverse_proxy"}
	for i, want := range wantOrder {
		h := handleList[i].(map[string]interface{})
		got := h["handler"].(string)
		if got != want {
			t.Errorf("handler[%d]: got %q, want %q", i, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/proxy/ -run TestBuildConfigHandlerOrder -v`
Expected: FAIL - got 3 handlers, want 4

- [ ] **Step 3: Add ipaccess to SetRoutes and buildConfig**

In `internal/proxy/proxy.go` `SetRoutes` method, add IP access rule registration after the rate limit block (after line 52):

```go
if r.AllowedIPs != nil {
    IPAccessRules.Set(r.Domain, r.AllowedIPs)
} else {
    IPAccessRules.Remove(r.Domain)
}
```

In `buildConfig`, update the handlers slice (lines 86-97) to include ipaccess first:

```go
handlers := []interface{}{
    map[string]interface{}{"handler": "simpledeploy_ipaccess"},
    map[string]interface{}{"handler": "simpledeploy_ratelimit"},
    map[string]interface{}{"handler": "simpledeploy_metrics"},
    map[string]interface{}{
        "handler": "reverse_proxy",
        "upstreams": []interface{}{
            map[string]interface{}{
                "dial": r.Upstream,
            },
        },
    },
}
```

- [ ] **Step 4: Run all proxy tests**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/proxy/ -v`
Expected: All PASS (update `TestBuildConfigWithRoutes` handler count assertion from 3 to 4 if it fails)

- [ ] **Step 5: Fix TestBuildConfigWithRoutes if needed**

The existing test at `proxy_test.go:86` checks `len(handleList) != 3`. Update it to `!= 4` and update the reverse_proxy index from `handleList[2]` to `handleList[3]`:

```go
if len(handleList) != 4 {
    t.Fatalf("route[%d] handle: got %d handlers, want 4", i, len(handleList))
}
rp := handleList[3].(map[string]interface{})
```

- [ ] **Step 6: Run all proxy tests again**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/proxy/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/proxy/proxy.go internal/proxy/proxy_test.go
git commit -m "feat(proxy): wire ipaccess handler into Caddy pipeline"
```

---

### Task 5: API endpoint for managing IP allowlist

**Files:**
- Create: `internal/api/access.go`
- Create: `internal/api/access_test.go`
- Modify: `internal/api/server.go:157-158` (add route)

- [ ] **Step 1: Write failing test for PUT /api/apps/{slug}/access**

Create `internal/api/access_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestHandleUpdateAccess(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: app.example.com
      simpledeploy.access.allow: "10.0.0.0/8"
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": "192.168.1.0/24,203.0.113.5"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/myapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if !strings.Contains(string(updated), "192.168.1.0/24,203.0.113.5") {
		t.Errorf("expected new allowlist in compose, got:\n%s", string(updated))
	}
}

func TestHandleUpdateAccess_InvalidIP(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "badapp", Slug: "badapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": "not-an-ip"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/badapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateAccess_ClearAllowlist(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: app.example.com
      simpledeploy.access.allow: "10.0.0.0/8"
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "clrapp", Slug: "clrapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": ""})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/clrapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateAccess_NoExistingLabel(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: app.example.com
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "newapp", Slug: "newapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": "10.0.0.1"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/newapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if !strings.Contains(string(updated), "simpledeploy.access.allow") {
		t.Errorf("expected access.allow label added, got:\n%s", string(updated))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/api/ -run TestHandleUpdateAccess -v`
Expected: FAIL - 404 (route not registered)

- [ ] **Step 3: Implement handler and register route**

Create `internal/api/access.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type accessRequest struct {
	Allow string `json:"allow"`
}

func (s *Server) handleUpdateAccess(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var req accessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate entries when non-empty
	if req.Allow != "" {
		for _, entry := range strings.Split(req.Allow, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if net.ParseIP(entry) == nil {
				if _, _, err := net.ParseCIDR(entry); err != nil {
					http.Error(w, fmt.Sprintf("invalid IP/CIDR: %q", entry), http.StatusBadRequest)
					return
				}
			}
		}
	}

	if err := updateComposeAccessAllow(app.ComposePath, req.Allow); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	if s.reconciler != nil {
		go s.reconciler.Reconcile(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "allow": req.Allow})
}

func updateComposeAccessAllow(composePath, allow string) error {
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
	root := doc.Content[0]

	servicesNode := findMapValue(root, "services")
	if servicesNode == nil {
		return fmt.Errorf("no services found in compose file")
	}
	if servicesNode.Kind != yaml.MappingNode || len(servicesNode.Content) < 2 {
		return fmt.Errorf("no services defined")
	}

	const labelKey = "simpledeploy.access.allow"

	// Find existing label value nodes
	var accessNodes []*yaml.Node
	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcNode := servicesNode.Content[i]
		labelsNode := findMapValue(svcNode, "labels")
		if labelsNode == nil {
			continue
		}
		av := findMapValue(labelsNode, labelKey)
		if av != nil {
			accessNodes = append(accessNodes, av)
		}
	}

	// If clearing and label exists, do surgical replacement with empty string
	if allow == "" && len(accessNodes) > 0 {
		lines := splitLines(data)
		for _, node := range accessNodes {
			lineIdx := node.Line - 1
			if lineIdx < 0 || lineIdx >= len(lines) {
				continue
			}
			lines[lineIdx] = replaceAccessValue(lines[lineIdx], node.Value, "")
		}
		return os.WriteFile(composePath, joinLines(lines), 0644)
	}

	// If existing label found, do surgical replacement
	if len(accessNodes) > 0 {
		lines := splitLines(data)
		for _, node := range accessNodes {
			lineIdx := node.Line - 1
			if lineIdx < 0 || lineIdx >= len(lines) {
				continue
			}
			lines[lineIdx] = replaceAccessValue(lines[lineIdx], node.Value, allow)
		}
		return os.WriteFile(composePath, joinLines(lines), 0644)
	}

	// No allow value and clearing: nothing to do
	if allow == "" {
		return nil
	}

	// No existing label: add to first service's labels
	firstServiceNode := servicesNode.Content[1]
	labelsNode := findMapValue(firstServiceNode, "labels")
	if labelsNode == nil {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "labels", Tag: "!!str"}
		labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		firstServiceNode.Content = append(firstServiceNode.Content, keyNode, labelsNode)
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: labelKey, Tag: "!!str"}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: allow, Tag: "!!str"}
	labelsNode.Content = append(labelsNode.Content, keyNode, valNode)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	return os.WriteFile(composePath, out, 0644)
}

func replaceAccessValue(line, oldVal, newVal string) string {
	for _, pattern := range []string{
		fmt.Sprintf(`"%s"`, oldVal),
		fmt.Sprintf(`'%s'`, oldVal),
		oldVal,
	} {
		if idx := strings.Index(line, pattern); idx >= 0 {
			replacement := newVal
			if strings.HasPrefix(pattern, `"`) {
				replacement = fmt.Sprintf(`"%s"`, newVal)
			} else if strings.HasPrefix(pattern, `'`) {
				replacement = fmt.Sprintf(`'%s'`, newVal)
			}
			return line[:idx] + replacement + line[idx+len(pattern):]
		}
	}
	return line
}
```

In `internal/api/server.go`, add the route after the domain route (after line 158):

```go
// IP access
s.mux.Handle("PUT /api/apps/{slug}/access", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleUpdateAccess))))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/api/ -run TestHandleUpdateAccess -v`
Expected: All PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/access.go internal/api/access_test.go internal/api/server.go
git commit -m "feat(api): add PUT /api/apps/{slug}/access endpoint"
```

---

### Task 6: Expose access.allow in app API response

**Files:**
- Modify: `internal/api/apps.go:23-40` (handleGetApp)

- [ ] **Step 1: Write failing test**

Add to `internal/api/apps_test.go`:

```go
func TestGetAppIncludesAccessAllow(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	s.UpsertApp(&store.App{Name: "ipapp", Slug: "ipapp", ComposePath: "/tmp/test.yml", Status: "running"}, map[string]string{
		"simpledeploy.access.allow": "10.0.0.0/8,192.168.1.5",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/apps/ipapp", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	labels, ok := resp["Labels"].(map[string]interface{})
	if !ok {
		t.Fatal("Labels not in response or not a map")
	}
	if labels["simpledeploy.access.allow"] != "10.0.0.0/8,192.168.1.5" {
		t.Errorf("access.allow = %v, want %q", labels["simpledeploy.access.allow"], "10.0.0.0/8,192.168.1.5")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/api/ -run TestGetAppIncludesAccessAllow -v`
Expected: FAIL - Labels not in response

- [ ] **Step 3: Add Labels to app response**

In `internal/api/apps.go`, modify `handleGetApp` to include labels:

```go
func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	labels, _ := s.store.GetAppLabels(slug)

	type appResponse struct {
		store.App
		Deploying bool              `json:"deploying"`
		Labels    map[string]string `json:"Labels,omitempty"`
	}
	resp := appResponse{
		App:       *app,
		Deploying: s.reconciler != nil && s.reconciler.IsDeploying(slug),
		Labels:    labels,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && go test ./internal/api/ -run TestGetAppIncludesAccessAllow -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/apps.go internal/api/apps_test.go
git commit -m "feat(api): include labels in GET /api/apps/{slug} response"
```

---

### Task 7: UI - API client and IP access section in AppDetail

**Files:**
- Modify: `ui/src/lib/api.js:127` (add updateAccess)
- Modify: `ui/src/routes/AppDetail.svelte`

- [ ] **Step 1: Add API method**

In `ui/src/lib/api.js`, add after the `updateDomain` line (line 127):

```javascript
updateAccess: (slug, allow) => requestWithToast('PUT', `/apps/${slug}/access`, { allow }, 'IP allowlist updated'),
```

- [ ] **Step 2: Add state and handler to AppDetail.svelte**

In `ui/src/routes/AppDetail.svelte`, add state variable after `editDomain` (after line 44):

```javascript
let editAccessAllow = $state('')
```

In the `startPolling` callback, after `editDomain = app?.Domain || ''` (after line 62), add:

```javascript
editAccessAllow = app?.Labels?.['simpledeploy.access.allow'] || ''
```

In `loadApp`, after `editDomain = app?.Domain || ''` (after line 90), add:

```javascript
editAccessAllow = app?.Labels?.['simpledeploy.access.allow'] || ''
```

Add handler function after `saveDomain` (after line 219):

```javascript
async function saveAccessAllow() {
    const { error } = await api.updateAccess(slug, editAccessAllow)
    if (!error) await loadApp()
}
```

- [ ] **Step 3: Add IP access section to template**

In the template, after the Domain `</div>` block (after line 325), add:

```svelte
<div>
  <span class="text-xs text-text-muted font-medium">IP Allowlist</span>
  <div class="flex items-center gap-2 mt-1">
    <input
      bind:value={editAccessAllow}
      placeholder="e.g. 10.0.0.0/8, 192.168.1.5"
      class="px-2 py-1 text-sm bg-surface-0 border border-border/50 rounded-lg font-mono focus:outline-none focus:border-accent w-80"
    />
    {#if editAccessAllow !== (app?.Labels?.['simpledeploy.access.allow'] || '')}
      <button
        onclick={saveAccessAllow}
        class="px-2 py-1 text-xs rounded bg-accent text-white hover:bg-accent/90 transition-colors"
      >
        Save
      </button>
    {/if}
  </div>
  <p class="text-xs text-text-muted mt-1">
    {#if editAccessAllow}
      Only these IPs/CIDRs can access this app
    {:else}
      All traffic allowed (no restriction)
    {/if}
  </p>
</div>
```

- [ ] **Step 4: Build UI to verify no errors**

Run: `cd /Users/ameen/dev/vazra/simpledeploy/ui && npm run build`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add ui/src/lib/api.js ui/src/routes/AppDetail.svelte
git commit -m "feat(ui): add IP allowlist editor to app detail"
```

---

### Task 8: Update compose-labels docs

**Files:**
- Modify: `docs/compose-labels.md`

- [ ] **Step 1: Add access.allow to compose-labels docs**

Add an "Access Control" section to `docs/compose-labels.md` documenting:

| Label | Description | Example |
|-------|-------------|---------|
| `simpledeploy.access.allow` | Comma-separated IPs/CIDRs. Only matching IPs can reach the app. Empty or absent = all traffic allowed. | `10.0.0.0/8,203.0.113.5` |

- [ ] **Step 2: Commit**

```bash
git add docs/compose-labels.md
git commit -m "docs: add simpledeploy.access.allow label"
```
