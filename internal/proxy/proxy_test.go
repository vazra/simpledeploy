package proxy

import (
	"encoding/json"
	"testing"
)

// --- CaddyProxy config builder tests ---

func newTestProxy(tlsMode, tlsEmail string) *CaddyProxy {
	return NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    tlsMode,
		TLSEmail:   tlsEmail,
	})
}

func parseConfig(t *testing.T, p *CaddyProxy) map[string]interface{} {
	t.Helper()
	data, err := p.BuildConfigJSON()
	if err != nil {
		t.Fatalf("BuildConfigJSON: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return cfg
}

func getServer(t *testing.T, cfg map[string]interface{}) map[string]interface{} {
	t.Helper()
	apps := cfg["apps"].(map[string]interface{})
	http := apps["http"].(map[string]interface{})
	servers := http["servers"].(map[string]interface{})
	return servers["proxy"].(map[string]interface{})
}

func TestBuildConfigNoRoutes(t *testing.T) {
	p := newTestProxy("off", "")
	cfg := parseConfig(t, p)
	server := getServer(t, cfg)

	listen := server["listen"].([]interface{})
	if len(listen) != 1 || listen[0].(string) != ":443" {
		t.Errorf("listen: got %v, want [\":443\"]", listen)
	}

	routes := server["routes"].([]interface{})
	if len(routes) != 0 {
		t.Errorf("routes: got %d entries, want 0", len(routes))
	}
}

func TestBuildConfigWithRoutes(t *testing.T) {
	p := newTestProxy("off", "")
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "app1.example.com", Upstream: "localhost:3000"},
		{Domain: "app2.example.com", Upstream: "localhost:4000"},
	}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	server := getServer(t, cfg)
	routes := server["routes"].([]interface{})

	if len(routes) != 2 {
		t.Fatalf("routes: got %d, want 2", len(routes))
	}

	wantDomains := []string{"app1.example.com", "app2.example.com"}
	wantDials := []string{"localhost:3000", "localhost:4000"}

	for i, entry := range routes {
		r := entry.(map[string]interface{})
		match := r["match"].([]interface{})[0].(map[string]interface{})
		host := match["host"].([]interface{})[0].(string)
		if host != wantDomains[i] {
			t.Errorf("route[%d] host: got %q, want %q", i, host, wantDomains[i])
		}

		// handlers: [ipaccess, ratelimit, metrics, reverse_proxy]
		handleList := r["handle"].([]interface{})
		if len(handleList) != 4 {
			t.Fatalf("route[%d] handle: got %d handlers, want 4", i, len(handleList))
		}
		rp := handleList[3].(map[string]interface{})
		dial := rp["upstreams"].([]interface{})[0].(map[string]interface{})["dial"].(string)
		if dial != wantDials[i] {
			t.Errorf("route[%d] dial: got %q, want %q", i, dial, wantDials[i])
		}
	}
}

func TestBuildConfigTLSOff(t *testing.T) {
	p := newTestProxy("off", "")
	cfg := parseConfig(t, p)
	server := getServer(t, cfg)

	autoHTTPS, ok := server["automatic_https"].(map[string]interface{})
	if !ok {
		t.Fatal("automatic_https not set")
	}
	if autoHTTPS["disable"] != true {
		t.Errorf("automatic_https.disable: got %v, want true", autoHTTPS["disable"])
	}
}

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

func TestBuildConfigTLSLocal(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "local",
	})
	cfg := parseConfig(t, p)
	server := getServer(t, cfg)

	if _, ok := server["automatic_https"]; ok {
		t.Error("automatic_https should not be set for local TLS mode")
	}

	apps := cfg["apps"].(map[string]interface{})
	tlsApp, ok := apps["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("apps.tls not set")
	}
	automation, ok := tlsApp["automation"].(map[string]interface{})
	if !ok {
		t.Fatal("tls.automation not set")
	}
	policies, ok := automation["policies"].([]interface{})
	if !ok || len(policies) != 1 {
		t.Fatalf("tls.automation.policies: got %v, want 1 entry", policies)
	}
	policy := policies[0].(map[string]interface{})
	issuers, ok := policy["issuers"].([]interface{})
	if !ok || len(issuers) != 1 {
		t.Fatalf("policy.issuers: got %v, want 1 entry", issuers)
	}
	issuer := issuers[0].(map[string]interface{})
	if issuer["module"] != "internal" {
		t.Errorf("issuer.module: got %v, want \"internal\"", issuer["module"])
	}
}

func TestBuildConfigTLSLocalStorage(t *testing.T) {
	dataDir := "/tmp/testdata"
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "local",
		DataDir:    dataDir,
	})
	cfg := parseConfig(t, p)

	storage, ok := cfg["storage"].(map[string]interface{})
	if !ok {
		t.Fatal("storage not set")
	}
	if storage["module"] != "file_system" {
		t.Errorf("storage.module: got %v, want \"file_system\"", storage["module"])
	}
	wantRoot := dataDir + "/caddy"
	if storage["root"] != wantRoot {
		t.Errorf("storage.root: got %v, want %q", storage["root"], wantRoot)
	}
}

func TestBuildConfigMixedLocalAndOff(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "local",
		DataDir:    "/tmp/sd-test",
	})
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "app.home.lan", Upstream: "localhost:3000", TLS: "local"},
		{Domain: "plain.home.lan", Upstream: "localhost:4000", TLS: "off"},
	}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	server := getServer(t, cfg)
	routes := server["routes"].([]interface{})
	if len(routes) != 2 {
		t.Fatalf("routes: got %d, want 2", len(routes))
	}

	apps := cfg["apps"].(map[string]interface{})
	tlsCfg, ok := apps["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("expected tls config")
	}
	automation := tlsCfg["automation"].(map[string]interface{})
	policies := automation["policies"].([]interface{})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}
}

// --- MockProxy tests ---

func TestMockProxySetRoutes(t *testing.T) {
	m := NewMockProxy()

	routes := []Route{
		{Domain: "a.example.com", Upstream: "localhost:1000"},
		{Domain: "b.example.com", Upstream: "localhost:2000"},
	}
	if err := m.SetRoutes(routes); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}

	if !m.HasRoute("a.example.com") {
		t.Error("expected HasRoute(a.example.com) = true")
	}
	if !m.HasRoute("b.example.com") {
		t.Error("expected HasRoute(b.example.com) = true")
	}
	if m.HasRoute("c.example.com") {
		t.Error("expected HasRoute(c.example.com) = false")
	}

	got := m.Routes()
	if len(got) != 2 {
		t.Fatalf("Routes: got %d, want 2", len(got))
	}
}
