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

		// handlers: [ipaccess, ratelimit, metrics, headers, reverse_proxy]
		handleList := r["handle"].([]interface{})
		if len(handleList) != 5 {
			t.Fatalf("route[%d] handle: got %d handlers, want 5", i, len(handleList))
		}
		rp := handleList[4].(map[string]interface{})
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

	// Expect 5 handlers: ipaccess, ratelimit, metrics, headers, reverse_proxy
	if len(handleList) != 5 {
		t.Fatalf("handle: got %d handlers, want 5", len(handleList))
	}

	wantOrder := []string{"simpledeploy_ipaccess", "simpledeploy_ratelimit", "simpledeploy_metrics", "headers", "reverse_proxy"}
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
	p := newTestProxy("custom", "")
	p.mu.Lock()
	p.routes = []Route{
		{Domain: "co.example.com", Upstream: "co:8001", TLS: "custom", CertDir: "/certs"},
		{Domain: "co.example.com", Upstream: "co:50051", TLS: "custom", CertDir: "/certs", Protocol: "grpc"},
	}
	p.mu.Unlock()

	cfg := parseConfig(t, p)
	tlsApp := cfg["apps"].(map[string]interface{})["tls"].(map[string]interface{})
	files := tlsApp["certificates"].(map[string]interface{})["load_files"].([]interface{})
	if len(files) != 1 {
		t.Fatalf("load_files = %d entries, want 1", len(files))
	}
}

func TestBuildConfigExtraListenAddrsShareHTTPSServer(t *testing.T) {
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
	if len(servers) != 2 {
		t.Fatalf("servers = %d, want 2 (proxy + proxy_http, no separate server for extras)", len(servers))
	}
	mainSrv := servers["proxy"].(map[string]interface{})
	listen := mainSrv["listen"].([]interface{})
	if len(listen) != 2 || listen[0].(string) != ":443" || listen[1].(string) != ":50051" {
		t.Errorf("proxy listen = %v, want [:443 :50051]", listen)
	}
	if _, ok := mainSrv["tls_connection_policies"]; !ok {
		t.Error("proxy server must keep tls_connection_policies so the extra listener terminates TLS")
	}
	if mainSrv["automatic_https"].(map[string]interface{})["disable_redirects"] != true {
		t.Error("proxy server must keep automatic_https.disable_redirects=true")
	}
	if len(mainSrv["routes"].([]interface{})) != 1 {
		t.Errorf("proxy routes = %d, want 1", len(mainSrv["routes"].([]interface{})))
	}
	httpListen := servers["proxy_http"].(map[string]interface{})["listen"].([]interface{})
	if len(httpListen) != 1 || httpListen[0].(string) != ":80" {
		t.Errorf("proxy_http listen = %v, want [:80]", httpListen)
	}
	// HTTP/3 only on the main listener; extras serve h1/h2.
	lp, ok := mainSrv["listen_protocols"].([]interface{})
	if !ok || len(lp) != 2 {
		t.Fatalf("listen_protocols = %v, want 2 entries", mainSrv["listen_protocols"])
	}
	if lp[0] != nil {
		t.Errorf("listen_protocols[0] = %v, want null (server default)", lp[0])
	}
	if got, _ := json.Marshal(lp[1]); string(got) != `["h1","h2"]` {
		t.Errorf("listen_protocols[1] = %s, want [\"h1\",\"h2\"]", got)
	}
	if _, ok := mainSrv["protocols"]; ok {
		t.Error("proxy server protocols must stay default so :443 keeps h3")
	}
}

func TestBuildConfigNoListenProtocolsWithoutExtras(t *testing.T) {
	p := NewCaddyProxy(CaddyConfig{ListenAddr: ":443", TLSMode: "auto"})
	server := getServer(t, parseConfig(t, p))
	if _, ok := server["listen_protocols"]; ok {
		t.Error("listen_protocols must be unset without extra listeners")
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
