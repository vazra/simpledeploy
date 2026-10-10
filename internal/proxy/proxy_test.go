package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

		// handlers: [ipaccess, ratelimit, metrics, headers x3, timeouts, reverse_proxy]
		handleList := r["handle"].([]interface{})
		if len(handleList) != 8 {
			t.Fatalf("route[%d] handle: got %d handlers, want 8", i, len(handleList))
		}
		rp := handleList[7].(map[string]interface{})
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

	// Expect ipaccess, ratelimit, metrics, one headers handler per default
	// security header (no HSTS on a plain-HTTP route), timeouts, reverse_proxy.
	wantOrder := []string{"simpledeploy_ipaccess", "simpledeploy_ratelimit", "simpledeploy_metrics", "headers", "headers", "headers", "timeouts", "reverse_proxy"}
	if len(handleList) != len(wantOrder) {
		t.Fatalf("handle: got %d handlers, want %d", len(handleList), len(wantOrder))
	}

	for i, want := range wantOrder {
		h := handleList[i].(map[string]interface{})
		got := h["handler"].(string)
		if got != want {
			t.Errorf("handler[%d]: got %q, want %q", i, got, want)
		}
	}
}

// Each default security header gets its own headers handler that sets it
// only when the app's response lacks it (require with a null value), so app
// values are kept. HSTS only on TLS routes.
func TestBuildConfigSecurityHeadersOnlyWhenMissing(t *testing.T) {
	p := newTestProxy("auto", "ops@example.com")
	p.routes = []Route{
		{Domain: "plain.example.com", Upstream: "a:1", TLS: "off"},
		{Domain: "secure.example.com", Upstream: "b:1", TLS: "auto"},
	}
	routes := getServer(t, parseConfig(t, p))["routes"].([]interface{})
	want := map[string][][2]string{
		"plain.example.com": {
			{"X-Content-Type-Options", "nosniff"},
			{"X-Frame-Options", "SAMEORIGIN"},
			{"Referrer-Policy", "strict-origin-when-cross-origin"},
		},
		"secure.example.com": {
			{"X-Content-Type-Options", "nosniff"},
			{"X-Frame-Options", "SAMEORIGIN"},
			{"Referrer-Policy", "strict-origin-when-cross-origin"},
			{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"},
		},
	}
	for _, rt := range routes {
		route := rt.(map[string]interface{})
		host := route["match"].([]interface{})[0].(map[string]interface{})["host"].([]interface{})[0].(string)
		var got [][2]string
		for _, h := range route["handle"].([]interface{}) {
			hm := h.(map[string]interface{})
			if hm["handler"] != "headers" {
				continue
			}
			resp := hm["response"].(map[string]interface{})
			if _, ok := resp["deferred"]; ok {
				t.Errorf("%s: deferred must not be set (require already defers): %v", host, resp)
			}
			set := resp["set"].(map[string]interface{})
			req := resp["require"].(map[string]interface{})["headers"].(map[string]interface{})
			if len(set) != 1 || len(req) != 1 {
				t.Fatalf("%s: want one header per handler, got set=%v require=%v", host, set, req)
			}
			for name, vals := range set {
				v, ok := req[name]
				if !ok || v != nil {
					t.Errorf("%s %s: require = %v, want {%q: null} (only when missing)", host, name, req, name)
				}
				got = append(got, [2]string{name, vals.([]interface{})[0].(string)})
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(want[host]) {
			t.Errorf("%s headers = %v, want %v", host, got, want[host])
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

	// With tls.mode=local we attach a connection policy and disable
	// implicit HTTP->HTTPS redirects so Caddy does not try to bind :80.
	if pol, ok := server["tls_connection_policies"].([]interface{}); !ok || len(pol) != 1 {
		t.Errorf("tls_connection_policies: got %v, want 1 entry", server["tls_connection_policies"])
	}
	ah, ok := server["automatic_https"].(map[string]interface{})
	if !ok || ah["disable_redirects"] != true {
		t.Errorf("automatic_https.disable_redirects: got %v, want true", server["automatic_https"])
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

func TestBuildConfigTLSAutoStorage(t *testing.T) {
	// Storage must be pinned for auto mode too, otherwise certmagic falls
	// back to $HOME/.local/share/caddy which ProtectHome=true blocks under
	// the shipped systemd unit.
	dataDir := "/tmp/testdata"
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "auto",
		TLSEmail:   "ops@example.com",
		DataDir:    dataDir,
	})
	cfg := parseConfig(t, p)

	storage, ok := cfg["storage"].(map[string]interface{})
	if !ok {
		t.Fatal("storage not set for tls.mode=auto")
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

func TestBuildConfigHTTPListenerRedirects(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr:     ":443",
		HTTPListenAddr: ":80",
		TLSMode:        "local",
	})
	cfg := parseConfig(t, p)
	servers := cfg["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})

	httpSrv, ok := servers["proxy_http"].(map[string]interface{})
	if !ok {
		t.Fatal("expected proxy_http server when http_listen_addr set")
	}
	listen := httpSrv["listen"].([]interface{})
	if len(listen) != 1 || listen[0].(string) != ":80" {
		t.Errorf("proxy_http listen = %v, want [:80]", listen)
	}
	routes := httpSrv["routes"].([]interface{})
	if len(routes) != 1 {
		t.Fatalf("proxy_http routes: got %d, want 1", len(routes))
	}
	handle := routes[0].(map[string]interface{})["handle"].([]interface{})
	sr := handle[0].(map[string]interface{})
	if sr["handler"] != "static_response" {
		t.Errorf("handler = %v, want static_response", sr["handler"])
	}
	loc := sr["headers"].(map[string]interface{})["Location"].([]interface{})
	if loc[0].(string) != "https://{http.request.host}{http.request.uri}" {
		t.Errorf("Location = %v", loc)
	}

	mainSrv := servers["proxy"].(map[string]interface{})
	autoHTTPS := mainSrv["automatic_https"].(map[string]interface{})
	if autoHTTPS["disable_redirects"] != true {
		t.Error("main server should have automatic_https.disable_redirects=true")
	}
}

func TestBuildConfigHTTPListenerIgnoredWhenTLSOff(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr:     ":80",
		HTTPListenAddr: ":8080",
		TLSMode:        "off",
	})
	cfg := parseConfig(t, p)
	servers := cfg["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})
	if _, ok := servers["proxy_http"]; ok {
		t.Error("proxy_http server should not be added when tls mode is off")
	}
}

func TestBuildConfigPerRouteLocalTLSWithGlobalOff(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "off",
		DataDir:    "/tmp/sd-test",
	})
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "vscode1.mac", Upstream: "localhost:8080", TLS: "local"},
	}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	server := getServer(t, cfg)

	// should NOT fully disable TLS -- a route needs it
	autoHTTPS, _ := server["automatic_https"].(map[string]interface{})
	if autoHTTPS["disable"] == true {
		t.Error("automatic_https.disable should not be true when a route uses tls:local")
	}

	// should have tls_connection_policies so Caddy serves TLS
	if pol, ok := server["tls_connection_policies"].([]interface{}); !ok || len(pol) == 0 {
		t.Errorf("tls_connection_policies: got %v, want at least 1 entry", server["tls_connection_policies"])
	}

	// should have internal CA policy scoped to the domain
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
	if !ok || len(policies) == 0 {
		t.Fatal("tls.automation.policies: want at least 1 entry")
	}
	found := false
	for _, p := range policies {
		pol := p.(map[string]interface{})
		subjects, _ := pol["subjects"].([]interface{})
		issuers, _ := pol["issuers"].([]interface{})
		if len(subjects) == 1 && subjects[0] == "vscode1.mac" && len(issuers) == 1 {
			if issuers[0].(map[string]interface{})["module"] == "internal" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected automation policy with subjects=[vscode1.mac] and module=internal")
	}

	// storage should be set for cert persistence
	storage, ok := cfg["storage"].(map[string]interface{})
	if !ok {
		t.Fatal("storage not set")
	}
	if storage["module"] != "file_system" {
		t.Errorf("storage.module: got %v, want file_system", storage["module"])
	}
}

func TestBuildConfigPerRouteLocalTLSWithGlobalAuto(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr: ":443",
		TLSMode:    "auto",
		TLSEmail:   "admin@example.com",
	})
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "vscode1.mac", Upstream: "localhost:8080", TLS: "local"},
	}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	apps := cfg["apps"].(map[string]interface{})
	tlsApp, ok := apps["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("apps.tls not set")
	}
	automation := tlsApp["automation"].(map[string]interface{})
	policies := automation["policies"].([]interface{})

	// expect global ACME policy + per-domain internal policy
	found := false
	for _, p := range policies {
		pol := p.(map[string]interface{})
		subjects, _ := pol["subjects"].([]interface{})
		issuers, _ := pol["issuers"].([]interface{})
		if len(subjects) == 1 && subjects[0] == "vscode1.mac" && len(issuers) == 1 {
			if issuers[0].(map[string]interface{})["module"] == "internal" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected per-domain internal CA policy for vscode1.mac alongside global auto policy")
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

func TestBuildConfigGRPCPathCatchAllSameDomain(t *testing.T) {
	p := newTestProxy("off", "")
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "co.example.com", Upstream: "co:8001", Protocol: "http"},
		{Domain: "co.example.com", Upstream: "co:8000", Protocol: "http", Path: "/ws*"},
		{Domain: "co.example.com", Upstream: "co:50051", Protocol: "grpc"},
	}
	p.mu.Unlock()

	routes := getServer(t, parseConfig(t, p))["routes"].([]interface{})
	if len(routes) != 3 {
		t.Fatalf("routes: got %d, want 3", len(routes))
	}

	lastHandler := func(i int) map[string]interface{} {
		h := routes[i].(map[string]interface{})["handle"].([]interface{})
		return h[len(h)-1].(map[string]interface{})
	}
	matchOf := func(i int) map[string]interface{} {
		return routes[i].(map[string]interface{})["match"].([]interface{})[0].(map[string]interface{})
	}

	wantDial := []string{"co:50051", "co:8000", "co:8001"}
	for i := range routes {
		if routes[i].(map[string]interface{})["terminal"] != true {
			t.Errorf("route[%d] terminal: want true", i)
		}
		dial := lastHandler(i)["upstreams"].([]interface{})[0].(map[string]interface{})["dial"]
		if dial != wantDial[i] {
			t.Fatalf("route[%d] dial = %v, want %s", i, dial, wantDial[i])
		}
	}

	// grpc route: header matcher, grpc-web excluded, h2c transport, flush -1
	gm := matchOf(0)
	ct := gm["header"].(map[string]interface{})["Content-Type"].([]interface{})
	if len(ct) != 1 || ct[0] != "application/grpc*" {
		t.Errorf("grpc header matcher = %v", ct)
	}
	notCT := gm["not"].([]interface{})[0].(map[string]interface{})["header"].(map[string]interface{})["Content-Type"].([]interface{})
	if len(notCT) != 1 || notCT[0] != "application/grpc-web*" {
		t.Errorf("grpc not matcher = %v", notCT)
	}
	rp := lastHandler(0)
	tr := rp["transport"].(map[string]interface{})
	if tr["protocol"] != "http" || tr["versions"].([]interface{})[0] != "h2c" {
		t.Errorf("grpc transport = %v", tr)
	}
	if rp["flush_interval"] != float64(-1) {
		t.Errorf("grpc flush_interval = %v, want -1", rp["flush_interval"])
	}

	// path route
	if pm := matchOf(1)["path"].([]interface{}); len(pm) != 1 || pm[0] != "/ws*" {
		t.Errorf("path matcher = %v", pm)
	}
	if _, ok := lastHandler(1)["transport"]; ok {
		t.Error("path http route must not set transport")
	}

	// catch-all
	cm := matchOf(2)
	if _, ok := cm["path"]; ok {
		t.Error("catch-all must not have path")
	}
	if _, ok := cm["header"]; ok {
		t.Error("catch-all must not have header")
	}
}

func TestBuildConfigTLSOffEnablesH2CListener(t *testing.T) {
	server := getServer(t, parseConfig(t, newTestProxy("off", "")))
	protos, ok := server["protocols"].([]interface{})
	if !ok {
		t.Fatal("protocols not set for plain-HTTP listener")
	}
	want := []string{"h1", "h2", "h2c"}
	if len(protos) != len(want) {
		t.Fatalf("protocols = %v, want %v", protos, want)
	}
	for i := range want {
		if protos[i] != want[i] {
			t.Errorf("protocols[%d] = %v, want %s", i, protos[i], want[i])
		}
	}
}

func TestBuildConfigTLSAutoKeepsDefaultProtocols(t *testing.T) {
	server := getServer(t, parseConfig(t, newTestProxy("auto", "ops@example.com")))
	if _, ok := server["protocols"]; ok {
		t.Errorf("protocols must be unset on TLS listener, got %v", server["protocols"])
	}
}

func TestBuildConfigCustomCertLoadedOncePerDomain(t *testing.T) {
	dir := writeCertFiles(t, "co.example.com")
	p := newTestProxy("custom", "")
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "co.example.com", Upstream: "co:8001", TLS: "custom", CertDir: dir},
		{Domain: "co.example.com", Upstream: "co:50051", TLS: "custom", CertDir: dir, Protocol: "grpc"},
	}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	tlsApp := cfg["apps"].(map[string]interface{})["tls"].(map[string]interface{})
	files := tlsApp["certificates"].(map[string]interface{})["load_files"].([]interface{})
	if len(files) != 1 {
		t.Fatalf("load_files = %d entries, want 1", len(files))
	}
}

func TestBuildConfigExtraListenAddrsSeparateServerNoH3(t *testing.T) {
	extra := []string{":50051"}
	p := NewCaddyProxy(CaddyConfig{
		ListenAddr:       ":443",
		ExtraListenAddrs: extra,
		HTTPListenAddr:   ":80",
		TLSMode:          "auto",
		TLSEmail:         "ops@example.com",
	})
	extra[0] = ":9999" // NewCaddyProxy must copy the slice
	p.mu.Lock()
	p.routes = []Route{{AppSlug: "a", Domain: "co.example.com", Upstream: "co:8001", TLS: "auto"}}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	servers := cfg["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})
	if len(servers) != 3 {
		t.Fatalf("servers = %d, want 3 (proxy, proxy_extra, proxy_http)", len(servers))
	}
	mainSrv := servers["proxy"].(map[string]interface{})
	if listen := mainSrv["listen"].([]interface{}); len(listen) != 1 || listen[0].(string) != ":443" {
		t.Errorf("proxy listen = %v, want [:443]", listen)
	}
	if _, ok := mainSrv["protocols"]; ok {
		t.Error("proxy server protocols must stay default so :443 keeps h3")
	}
	extraSrv := servers["proxy_extra"].(map[string]interface{})
	if listen := extraSrv["listen"].([]interface{}); len(listen) != 1 || listen[0].(string) != ":50051" {
		t.Errorf("proxy_extra listen = %v, want [:50051]", listen)
	}
	if got, _ := json.Marshal(extraSrv["protocols"]); string(got) != `["h1","h2"]` {
		t.Errorf("proxy_extra protocols = %s, want [\"h1\",\"h2\"]", got)
	}
	for _, k := range []string{"routes", "tls_connection_policies", "automatic_https"} {
		a, _ := json.Marshal(mainSrv[k])
		b, _ := json.Marshal(extraSrv[k])
		if mainSrv[k] == nil || string(a) != string(b) {
			t.Errorf("proxy_extra %s = %s, want same as proxy %s", k, b, a)
		}
	}
	httpListen := servers["proxy_http"].(map[string]interface{})["listen"].([]interface{})
	if len(httpListen) != 1 || httpListen[0].(string) != ":80" {
		t.Errorf("proxy_http listen = %v, want [:80]", httpListen)
	}
}

func TestBuildConfigExtraListenAddrsTLSOffKeepsH2C(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":80", ExtraListenAddrs: []string{":50051"}, TLSMode: "off"})
	servers := parseConfig(t, p)["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})
	extraSrv := servers["proxy_extra"].(map[string]interface{})
	if got, _ := json.Marshal(extraSrv["protocols"]); string(got) != `["h1","h2","h2c"]` {
		t.Errorf("proxy_extra protocols = %s, want h1,h2,h2c", got)
	}
}

func TestBuildConfigNoExtraServerWithoutExtras(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "auto"})
	servers := parseConfig(t, p)["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})
	if _, ok := servers["proxy_extra"]; ok {
		t.Error("proxy_extra must not exist without extra listeners")
	}
}

func TestBuildConfigDeterministicAcrossInputOrder(t *testing.T) {
	routes := []Route{
		{AppSlug: "w", Domain: "*.example.com", Upstream: "w:1", TLS: "auto"},
		{AppSlug: "b", Domain: "b.example.com", Upstream: "b:1", TLS: "auto"},
		{AppSlug: "a", Domain: "a.example.com", Upstream: "a:1", TLS: "custom", CertDir: "/c"},
		{AppSlug: "a", Domain: "a.example.com", Upstream: "a:2", TLS: "custom", CertDir: "/c", Path: "/x*"},
		{AppSlug: "c", Domain: "c.example.com", Upstream: "c:1", TLS: "local"},
		{AppSlug: "d", Domain: "d.example.com", Upstream: "d:1", TLS: "local"},
	}
	build := func(rs []Route) string {
		p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", ExtraListenAddrs: []string{":50051"}, TLSMode: "auto", TLSEmail: "e@x.com"})
		p.routes = rs
		b, err := p.BuildConfigJSON()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	want := build(routes)
	rev := make([]Route, len(routes))
	for i, r := range routes {
		rev[len(routes)-1-i] = r
	}
	if got := build(rev); got != want {
		t.Fatalf("config depends on input order:\n%s\n%s", want, got)
	}
	// Wildcard host must come after exact hosts so it cannot shadow them.
	srv := getServer(t, func() map[string]interface{} {
		var m map[string]interface{}
		_ = json.Unmarshal([]byte(want), &m)
		return m
	}())
	rs := srv["routes"].([]interface{})
	last := rs[len(rs)-1].(map[string]interface{})["match"].([]interface{})[0].(map[string]interface{})["host"].([]interface{})[0]
	if last != "*.example.com" {
		t.Errorf("last route host = %v, want *.example.com", last)
	}
}

func TestBuildConfigNoExtraListenAddrs(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "auto"})
	server := getServer(t, parseConfig(t, p))
	listen := server["listen"].([]interface{})
	if len(listen) != 1 || listen[0].(string) != ":443" {
		t.Errorf("listen = %v, want [:443]", listen)
	}
}

// countingLoad replaces caddy.Load with a counter.
func countingLoad(p *CaddyProxy) *int {
	n := 0
	p.load = func([]byte, bool) error { n++; return nil }
	return &n
}

func TestReloadSkipsIdenticalConfig(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "auto"})
	n := countingLoad(p)
	a := Route{AppSlug: "a", Domain: "a.test", Upstream: "a:1", TLS: "auto"}
	b := Route{AppSlug: "b", Domain: "b.test", Upstream: "b:1", TLS: "auto"}
	steps := []struct {
		routes []Route
		want   int
	}{
		{[]Route{a, b}, 1},
		{[]Route{b, a}, 1}, // same routes, other order: skipped
		{[]Route{a}, 2},    // changed: reloads
	}
	for i, st := range steps {
		if err := p.SetRoutes(st.routes); err != nil {
			t.Fatal(err)
		}
		if *n != st.want {
			t.Fatalf("step %d: loads = %d, want %d", i, *n, st.want)
		}
	}
	if err := p.ForceReload(); err != nil {
		t.Fatal(err)
	}
	if *n != 3 {
		t.Fatalf("ForceReload: loads = %d, want 3", *n)
	}
}

func TestReloadOnCustomCertRewrite(t *testing.T) {
	dir := t.TempDir()
	crt := filepath.Join(dir, "c.test.crt")
	key := filepath.Join(dir, "c.test.key")
	for _, f := range []string{crt, key} {
		if err := os.WriteFile(f, []byte("v1"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "auto"})
	n := countingLoad(p)
	routes := []Route{{AppSlug: "c", Domain: "c.test", Upstream: "c:1", TLS: "custom", CertDir: dir}}
	if err := p.SetRoutes(routes); err != nil {
		t.Fatal(err)
	}
	if err := p.SetRoutes(routes); err != nil {
		t.Fatal(err)
	}
	if *n != 1 {
		t.Fatalf("unchanged cert: loads = %d, want 1", *n)
	}
	// Rewrite the cert in place (same path, new content and mtime).
	if err := os.WriteFile(crt, []byte("v2-longer"), 0600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(crt, future, future)
	if err := p.SetRoutes(routes); err != nil {
		t.Fatal(err)
	}
	if *n != 2 {
		t.Fatalf("rewritten cert: loads = %d, want 2", *n)
	}
	// Deleting the cert also changes the key.
	_ = os.Remove(crt)
	if err := p.SetRoutes(routes); err != nil {
		t.Fatal(err)
	}
	if *n != 3 {
		t.Fatalf("deleted cert: loads = %d, want 3", *n)
	}
}

func writeCertFiles(t *testing.T, domain string) string {
	t.Helper()
	dir := t.TempDir()
	for _, ext := range []string{".crt", ".key"} {
		if err := os.WriteFile(filepath.Join(dir, domain+ext), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBuildConfigSkipsMissingCustomCert(t *testing.T) {
	present := writeCertFiles(t, "ok.example.com")
	missing := t.TempDir()
	// Only the key exists for half.example.com: still skipped.
	if err := os.WriteFile(filepath.Join(missing, "half.example.com.key"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	p := newTestProxy("auto", "ops@example.com")
	p.routes = []Route{
		{Domain: "ok.example.com", Upstream: "a:1", TLS: "custom", CertDir: present},
		{Domain: "gone.example.com", Upstream: "b:1", TLS: "custom", CertDir: missing},
		{Domain: "half.example.com", Upstream: "c:1", TLS: "custom", CertDir: missing},
	}
	cfg := parseConfig(t, p)
	tlsApp := cfg["apps"].(map[string]interface{})["tls"].(map[string]interface{})
	files := tlsApp["certificates"].(map[string]interface{})["load_files"].([]interface{})
	if len(files) != 1 || files[0].(map[string]interface{})["tags"].([]interface{})[0] != "ok.example.com" {
		t.Fatalf("load_files = %v, want only ok.example.com", files)
	}
	// Routes for endpoints with missing certs are still served.
	if n := len(getServer(t, cfg)["routes"].([]interface{})); n != 3 {
		t.Errorf("routes = %d, want 3", n)
	}

	// All custom certs missing: no certificates block at all.
	p.routes = p.routes[1:]
	cfg = parseConfig(t, p)
	if _, ok := cfg["apps"].(map[string]interface{})["tls"].(map[string]interface{})["certificates"]; ok {
		t.Error("certificates block must be omitted when every custom cert is missing")
	}
}

func TestBuildConfigExtraServerIsDeepCopy(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", ExtraListenAddrs: []string{":50051"}, TLSMode: "auto"})
	p.routes = []Route{{AppSlug: "a", Domain: "a.test", Upstream: "a:1", TLS: "auto"}}
	cfg := p.buildConfigFrom(p.snapshotRoutes())
	servers := cfg["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})
	mainSrv := servers["proxy"].(map[string]interface{})
	extra := servers["proxy_extra"].(map[string]interface{})
	before, _ := json.Marshal(extra)

	r0 := mainSrv["routes"].([]interface{})[0].(map[string]interface{})
	r0["terminal"] = false
	r0["handle"].([]interface{})[0].(map[string]interface{})["handler"] = "mutated"
	r0["match"].([]interface{})[0].(map[string]interface{})["host"].([]string)[0] = "mutated.test"
	mainSrv["tls_connection_policies"].([]interface{})[0].(map[string]interface{})["mutated"] = true
	mainSrv["automatic_https"].(map[string]interface{})["mutated"] = true

	after, _ := json.Marshal(extra)
	if string(before) != string(after) {
		t.Fatalf("mutating proxy leaked into proxy_extra:\n%s\n%s", before, after)
	}
}

func TestBuildConfigCustomTLSDomainsSkipAutomation(t *testing.T) {
	present := writeCertFiles(t, "ok.example.com")
	missing := t.TempDir()
	for _, mode := range []string{"auto", "local"} {
		p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", ExtraListenAddrs: []string{":50051"}, TLSMode: mode, TLSEmail: "ops@example.com"})
		p.routes = []Route{
			{Domain: "ok.example.com", Upstream: "a:1", TLS: "custom", CertDir: present},
			{Domain: "gone.example.com", Upstream: "b:1", TLS: "custom", CertDir: missing},
			{Domain: "gone.example.com", Upstream: "b:2", TLS: "custom", CertDir: missing, Path: "/x*"},
			{Domain: "auto.example.com", Upstream: "c:1", TLS: mode},
		}
		cfg := parseConfig(t, p)
		servers := cfg["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})
		for _, name := range []string{"proxy", "proxy_extra"} {
			ah := servers[name].(map[string]interface{})["automatic_https"].(map[string]interface{})
			got, _ := json.Marshal(ah["skip_certificates"])
			if string(got) != `["gone.example.com","ok.example.com"]` {
				t.Errorf("%s/%s skip_certificates = %s, want both custom domains only", mode, name, got)
			}
		}
		// No automation policy names a custom domain explicitly.
		tlsApp, _ := cfg["apps"].(map[string]interface{})["tls"].(map[string]interface{})
		raw, _ := json.Marshal(tlsApp["automation"])
		for _, d := range []string{"gone.example.com", "ok.example.com"} {
			if strings.Contains(string(raw), d) {
				t.Errorf("%s: automation mentions custom domain %s: %s", mode, d, raw)
			}
		}
	}
}

// freshRegistries swaps in empty package-level registries for the test.
func freshRegistries(t *testing.T) {
	t.Helper()
	origIP, origRL := IPAccessRules, RateLimiters
	IPAccessRules = newIPAccessRegistry()
	RateLimiters = &RateLimiterRegistry{limiters: make(map[string]*domainLimiter)}
	t.Cleanup(func() { IPAccessRules, RateLimiters = origIP, origRL })
}

func routeApps(rs []Route) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.AppSlug+"@"+r.Domain+r.Path)
	}
	return out
}

func TestClaimDomainsOneAppPerDomain(t *testing.T) {
	routes := []Route{
		{AppSlug: "zeta", Domain: "Shared.example.com"},
		{AppSlug: "alpha", Domain: "shared.example.com."},
		{AppSlug: "alpha", Domain: "shared.example.com", Path: "/api*"}, // same app, other path: kept
		{AppSlug: "zeta", Domain: "zeta.example.com"},
	}
	kept, owners := claimDomains(routes, nil)
	want := []string{"alpha@shared.example.com.", "alpha@shared.example.com/api*", "zeta@zeta.example.com"}
	if got := routeApps(kept); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("kept = %v, want %v", got, want)
	}
	if owners["shared.example.com"] != "alpha" || owners["zeta.example.com"] != "zeta" {
		t.Errorf("owners = %v", owners)
	}

	// The previous owner keeps the domain even when it sorts later.
	kept, owners = claimDomains(routes, map[string]string{"shared.example.com": "zeta"})
	if owners["shared.example.com"] != "zeta" {
		t.Fatalf("owner = %q, want previous owner zeta", owners["shared.example.com"])
	}
	for _, r := range kept {
		if r.AppSlug == "alpha" {
			t.Errorf("alpha route kept on zeta's domain: %+v", r)
		}
	}

	// A previous owner that no longer claims the domain does not block others.
	_, owners = claimDomains(routes, map[string]string{"shared.example.com": "gone"})
	if owners["shared.example.com"] != "alpha" {
		t.Errorf("owner = %q, want alpha", owners["shared.example.com"])
	}
}

func TestClaimDomainsLowestAppIDWins(t *testing.T) {
	routes := []Route{
		{AppSlug: "alpha", AppID: 7, Domain: "shared.example.com"},
		{AppSlug: "zeta", AppID: 3, Domain: "shared.example.com"},
		{AppSlug: "mid", AppID: 3, Domain: "shared.example.com"},
	}
	kept, owners := claimDomains(routes, nil)
	// zeta and mid tie on AppID 3; the slug breaks the tie.
	if owners["shared.example.com"] != "mid" {
		t.Fatalf("owner = %q, want mid (lowest AppID, then slug)", owners["shared.example.com"])
	}
	if got := routeApps(kept); len(got) != 1 || got[0] != "mid@shared.example.com" {
		t.Errorf("kept = %v", got)
	}

	// A previous owner that still claims the domain keeps it.
	_, owners = claimDomains(routes, map[string]string{"shared.example.com": "alpha"})
	if owners["shared.example.com"] != "alpha" {
		t.Errorf("owner = %q, want previous owner alpha", owners["shared.example.com"])
	}
}

func TestSetRoutesOwnerStableAcrossRestart(t *testing.T) {
	freshRegistries(t)
	older := Route{AppSlug: "zeta", AppID: 1, Domain: "shared.test", Upstream: "z:1"}
	newer := Route{AppSlug: "alpha", AppID: 2, Domain: "shared.test", Upstream: "a:1"}

	// Running proxy: zeta claimed the domain first.
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "off"})
	countingLoad(p)
	if err := p.SetRoutes([]Route{older}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetRoutes([]Route{newer, older}); err != nil {
		t.Fatal(err)
	}
	// Restart: a fresh proxy gets the same routes in either order.
	for _, rs := range [][]Route{{newer, older}, {older, newer}} {
		fresh := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "off"})
		countingLoad(fresh)
		if err := fresh.SetRoutes(rs); err != nil {
			t.Fatal(err)
		}
		if got, want := routeApps(fresh.snapshotRoutes()), routeApps(p.snapshotRoutes()); strings.Join(got, ",") != strings.Join(want, ",") || len(got) != 1 || got[0] != "zeta@shared.test" {
			t.Errorf("after restart routes = %v, before = %v, want zeta@shared.test", got, want)
		}
	}
}

func TestSetRoutesLoadFailureRestoresRules(t *testing.T) {
	freshRegistries(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "off"})
	fail := false
	p.load = func([]byte, bool) error {
		if fail {
			return errors.New("load failed")
		}
		return nil
	}
	a := Route{AppSlug: "a", AppID: 1, Domain: "a.test", Upstream: "a:1", AllowedIPs: []string{"10.0.0.1"},
		RateLimit: &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"}}
	if err := p.SetRoutes([]Route{a}); err != nil {
		t.Fatal(err)
	}
	// Use up a's single token so a fresh limiter would be detectable.
	ok := reqFrom("a.test", "10.0.0.1:1")
	if !RateLimiters.Allow("a.test", ok) {
		t.Fatal("first request limited")
	}

	// New set: a is gone, b serves a.test without rules and b.test with an
	// allowlist.
	fail = true
	b := Route{AppSlug: "b", AppID: 2, Domain: "b.test", Upstream: "b:1", AllowedIPs: []string{"10.0.0.2"}}
	if err := p.SetRoutes([]Route{b, {AppSlug: "b", AppID: 2, Domain: "a.test", Upstream: "b:2"}}); err == nil {
		t.Fatal("SetRoutes succeeded with a failing load")
	}

	if got := routeApps(p.snapshotRoutes()); len(got) != 1 || got[0] != "a@a.test" {
		t.Errorf("routes = %v, want previous set", got)
	}
	if p.owners["a.test"] != "a" || len(p.owners) != 1 {
		t.Errorf("owners = %v, want previous owners", p.owners)
	}
	if IPAccessRules.Allowed("a.test", reqFrom("", "5.5.5.5:1")) {
		t.Error("a.test allowlist not restored")
	}
	if !IPAccessRules.Allowed("b.test", reqFrom("", "5.5.5.5:1")) {
		t.Error("b.test allowlist from the failed set still active")
	}
	if RateLimiters.Allow("a.test", ok) {
		t.Error("a.test limiter not restored with its counters")
	}

	// The next successful load applies the new set.
	fail = false
	a2 := a
	a2.AllowedIPs, a2.RateLimit = nil, nil
	if err := p.SetRoutes([]Route{a2, b}); err != nil {
		t.Fatal(err)
	}
	if !IPAccessRules.Allowed("a.test", reqFrom("", "5.5.5.5:1")) || IPAccessRules.Allowed("b.test", reqFrom("", "5.5.5.5:1")) {
		t.Error("rules not applied after a successful load")
	}
}

func TestSetRoutesLaterAppCannotClearOrOverrideRules(t *testing.T) {
	freshRegistries(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "off"})
	countingLoad(p)

	owner := Route{AppSlug: "a", Domain: "secure.test", Upstream: "a:1", AllowedIPs: []string{"10.0.0.1"},
		RateLimit: &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"}}
	// "z" sorts after "a" and claims the same domain (other case) without
	// rules: it must not clear a's allowlist or replace its limiter.
	intruder := Route{AppSlug: "z", Domain: "SECURE.test", Upstream: "z:1",
		RateLimit: &RateLimitConfig{Requests: 1000, Window: time.Minute, By: "ip"}}
	if err := p.SetRoutes([]Route{owner, intruder}); err != nil {
		t.Fatal(err)
	}
	if got := routeApps(p.snapshotRoutes()); len(got) != 1 || got[0] != "a@secure.test" {
		t.Fatalf("routes = %v, want only a's", got)
	}
	if IPAccessRules.Allowed("secure.test", reqFrom("", "5.5.5.5:1")) {
		t.Error("intruder cleared the owner's IP allowlist")
	}
	req := reqFrom("secure.test", "10.0.0.1:1")
	RateLimiters.Allow("secure.test", req)
	if RateLimiters.Allow("secure.test", req) {
		t.Error("intruder replaced the owner's rate limiter")
	}

	// Owner is sticky: "0first" sorts before "a" but arrives later.
	early := Route{AppSlug: "0first", Domain: "secure.test", Upstream: "f:1"}
	if err := p.SetRoutes([]Route{early, owner, intruder}); err != nil {
		t.Fatal(err)
	}
	if got := routeApps(p.snapshotRoutes()); len(got) != 1 || got[0] != "a@secure.test" {
		t.Fatalf("routes = %v, want previous owner a to keep the domain", got)
	}
}

func TestSetRoutesDropsStaleRules(t *testing.T) {
	freshRegistries(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "off"})
	countingLoad(p)
	r := Route{AppSlug: "a", Domain: "a.test", Upstream: "a:1", AllowedIPs: []string{"10.0.0.1"},
		RateLimit: &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"}}
	if err := p.SetRoutes([]Route{r}); err != nil {
		t.Fatal(err)
	}
	r.AllowedIPs, r.RateLimit = nil, nil
	if err := p.SetRoutes([]Route{r}); err != nil {
		t.Fatal(err)
	}
	req := reqFrom("a.test", "5.5.5.5:1")
	if !IPAccessRules.Allowed("a.test", req) || !RateLimiters.Allow("a.test", req) || !RateLimiters.Allow("a.test", req) {
		t.Error("removed allowlist/rate limit must no longer apply")
	}
}

func TestBuildConfigBindsRuleHandlersToRouteDomain(t *testing.T) {
	p := newTestProxy("off", "")
	p.routes = []Route{{Domain: "App.Example.com", Upstream: "a:1"}}
	routes := getServer(t, parseConfig(t, p))["routes"].([]interface{})
	hs := routes[0].(map[string]interface{})["handle"].([]interface{})
	for _, i := range []int{0, 1} {
		if d := hs[i].(map[string]interface{})["domain"]; d != "app.example.com" {
			t.Errorf("handler[%d] domain = %v, want app.example.com", i, d)
		}
	}
}

func TestSetRoutesSkipsInvalidDomainKeepsOthers(t *testing.T) {
	freshRegistries(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "off"})
	countingLoad(p)
	err := p.SetRoutes([]Route{
		{AppSlug: "bad", Domain: "bad_domain!", Upstream: "b:1"},
		{AppSlug: "a", Domain: "a.test", Upstream: "a:1", AllowedIPs: []string{"10.0.0.1"}},
	})
	if err != nil {
		t.Fatalf("SetRoutes: %v (one bad route must not block others)", err)
	}
	if got := routeApps(p.snapshotRoutes()); len(got) != 1 || got[0] != "a@a.test" {
		t.Fatalf("routes = %v, want only a's", got)
	}
	if IPAccessRules.Allowed("a.test", reqFrom("", "5.5.5.5:1")) {
		t.Error("valid app's allowlist not applied")
	}
}

// Every listener keeps a 1 MiB header limit, and the body idle timeout is
// applied per route so gRPC/h2c streams are exempt.
func TestBuildConfigRequestLimits(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", ExtraListenAddrs: []string{":50051"}, HTTPListenAddr: ":80", TLSMode: "auto"})
	p.routes = []Route{
		{Domain: "web.test", Upstream: "web:80"},
		{Domain: "co.test", Upstream: "co:50051", Protocol: "grpc"},
		{Domain: "h2.test", Upstream: "h2:8080", Protocol: "h2c"},
	}
	cfg := parseConfig(t, p)
	servers := cfg["apps"].(map[string]interface{})["http"].(map[string]interface{})["servers"].(map[string]interface{})
	for _, name := range []string{"proxy", "proxy_extra", "proxy_http"} {
		srv := servers[name].(map[string]interface{})
		if got := srv["max_header_bytes"]; got != float64(1<<20) {
			t.Errorf("%s max_header_bytes = %v, want %d", name, got, 1<<20)
		}
	}
	for _, name := range []string{"proxy", "proxy_extra"} {
		srv := servers[name].(map[string]interface{})
		if got := srv["read_idle_timeout"]; got != float64(-1) {
			t.Errorf("%s read_idle_timeout = %v, want -1 (per-route instead)", name, got)
		}
		for _, rt := range srv["routes"].([]interface{}) {
			route := rt.(map[string]interface{})
			host := route["match"].([]interface{})[0].(map[string]interface{})["host"].([]interface{})[0]
			var timeouts map[string]interface{}
			for _, h := range route["handle"].([]interface{}) {
				if hm := h.(map[string]interface{}); hm["handler"] == "timeouts" {
					timeouts = hm
				}
			}
			switch host {
			case "web.test":
				if timeouts == nil || timeouts["read_timeout"] != float64(time.Minute) {
					t.Errorf("%s %s timeouts handler = %v, want read_timeout %d", name, host, timeouts, int64(time.Minute))
				}
			default:
				if timeouts != nil {
					t.Errorf("%s %s: gRPC/h2c route must not get a body idle timeout, got %v", name, host, timeouts)
				}
			}
		}
	}
}
