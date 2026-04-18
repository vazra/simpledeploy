package proxy

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func makeApp(name string, endpoints []compose.EndpointConfig, services []compose.ServiceConfig) *compose.AppConfig {
	return &compose.AppConfig{
		Name:      name,
		Endpoints: endpoints,
		Services:  services,
	}
}

func ports(mappings ...string) []compose.PortMapping {
	// mappings: pairs of host, container
	var pms []compose.PortMapping
	for i := 0; i+1 < len(mappings); i += 2 {
		pms = append(pms, compose.PortMapping{Host: mappings[i], Container: mappings[i+1]})
	}
	return pms
}

func TestResolveRoutesBasic(t *testing.T) {
	app := makeApp("myapp",
		[]compose.EndpointConfig{
			{Domain: "example.com", Port: "8080", TLS: "auto", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("3000", "8080")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	r := routes[0]
	if r.AppSlug != "myapp" {
		t.Errorf("AppSlug: got %q, want %q", r.AppSlug, "myapp")
	}
	if r.Domain != "example.com" {
		t.Errorf("Domain: got %q, want %q", r.Domain, "example.com")
	}
	if r.Upstream != "localhost:3000" {
		t.Errorf("Upstream: got %q, want %q", r.Upstream, "localhost:3000")
	}
	if r.TLS != "auto" {
		t.Errorf("TLS: got %q, want %q", r.TLS, "auto")
	}
}

func TestResolveRoutesNoEndpoints(t *testing.T) {
	app := makeApp("myapp", nil, []compose.ServiceConfig{
		{Name: "web", Ports: ports("3000", "8080")},
	})
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 0 {
		t.Errorf("len(routes) = %d, want 0", len(routes))
	}
}

func TestResolveRoutesMultiEndpoint(t *testing.T) {
	app := makeApp("multi",
		[]compose.EndpointConfig{
			{Domain: "web.example.com", Port: "3000", TLS: "auto", Service: "web"},
			{Domain: "api.example.com", Port: "8080", TLS: "custom", Service: "api"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("3000", "3000")},
			{Name: "api", Ports: ports("4000", "8080")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("len(routes) = %d, want 2", len(routes))
	}
	if routes[0].Domain != "web.example.com" {
		t.Errorf("routes[0].Domain = %q, want %q", routes[0].Domain, "web.example.com")
	}
	if routes[0].Upstream != "localhost:3000" {
		t.Errorf("routes[0].Upstream = %q, want %q", routes[0].Upstream, "localhost:3000")
	}
	if routes[1].Domain != "api.example.com" {
		t.Errorf("routes[1].Domain = %q, want %q", routes[1].Domain, "api.example.com")
	}
	if routes[1].Upstream != "localhost:4000" {
		t.Errorf("routes[1].Upstream = %q, want %q", routes[1].Upstream, "localhost:4000")
	}
	if routes[1].TLS != "custom" {
		t.Errorf("routes[1].TLS = %q, want %q", routes[1].TLS, "custom")
	}
}

func TestResolveRoutesTLSDefault(t *testing.T) {
	app := makeApp("svc",
		[]compose.EndpointConfig{
			{Domain: "svc.example.com", Port: "80", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("8000", "80")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].TLS != "auto" {
		t.Errorf("TLS: got %q, want %q", routes[0].TLS, "auto")
	}
}

func TestResolveRoutesDockerNetwork(t *testing.T) {
	// No host port mapping for container port
	app := makeApp("internal",
		[]compose.EndpointConfig{
			{Domain: "app.example.com", Port: "3000", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("5000", "5000")}, // different port, no match
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	// Falls back to Docker network address
	if routes[0].Upstream != "web:3000" {
		t.Errorf("Upstream: got %q, want %q", routes[0].Upstream, "web:3000")
	}
}

func TestResolveRoutesWithAllowedIPs(t *testing.T) {
	app := makeApp("secured",
		[]compose.EndpointConfig{
			{Domain: "secure.example.com", Port: "8080", TLS: "auto", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("3000", "8080")},
		},
	)
	app.AccessAllow = "10.0.0.0/8, 192.168.1.5, bad-entry, 172.16.0.0/12"

	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	// bad-entry should be skipped, 3 valid entries remain
	r := routes[0]
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

func TestResolveRoutesEmptyAllowedIPs(t *testing.T) {
	app := makeApp("open",
		[]compose.EndpointConfig{
			{Domain: "open.example.com", Port: "80", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("5000", "80")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].AllowedIPs != nil {
		t.Errorf("AllowedIPs: got %v, want nil", routes[0].AllowedIPs)
	}
}

func TestResolveRoutesLocalTLS(t *testing.T) {
	app := makeApp("localapp",
		[]compose.EndpointConfig{
			{Domain: "app.home.lan", Port: "3000", TLS: "local", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("3000", "3000")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].TLS != "local" {
		t.Errorf("TLS = %q, want %q", routes[0].TLS, "local")
	}
}

func TestResolveEndpointUpstreamDefault(t *testing.T) {
	// With SIMPLEDEPLOY_UPSTREAM_HOST unset, upstream uses "localhost".
	t.Setenv("SIMPLEDEPLOY_UPSTREAM_HOST", "")
	app := makeApp("hostdefault",
		[]compose.EndpointConfig{
			{Domain: "example.com", Port: "8080", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("3000", "8080")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].Upstream != "localhost:3000" {
		t.Errorf("Upstream: got %q, want %q", routes[0].Upstream, "localhost:3000")
	}
}

func TestResolveEndpointUpstreamRewrite(t *testing.T) {
	// With SIMPLEDEPLOY_UPSTREAM_HOST set, upstream uses the override host.
	t.Setenv("SIMPLEDEPLOY_UPSTREAM_HOST", "host.docker.internal")
	app := makeApp("hostrewrite",
		[]compose.EndpointConfig{
			{Domain: "example.com", Port: "8080", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("3000", "8080")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].Upstream != "host.docker.internal:3000" {
		t.Errorf("Upstream: got %q, want %q", routes[0].Upstream, "host.docker.internal:3000")
	}

	// Docker-DNS fallback (<service>:<port>) is unaffected by the rewrite.
	app2 := makeApp("fallback",
		[]compose.EndpointConfig{
			{Domain: "app.example.com", Port: "3000", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("5000", "5000")}, // no match for 3000
		},
	)
	routes2, err := ResolveRoutes(app2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes2) != 1 {
		t.Fatalf("len(routes2) = %d, want 1", len(routes2))
	}
	if routes2[0].Upstream != "web:3000" {
		t.Errorf("Upstream: got %q, want %q", routes2[0].Upstream, "web:3000")
	}
}

func TestResolveEndpointUpstreamRewriteFirstMapping(t *testing.T) {
	// With no endpoint.Port set, resolver falls through to the "first host
	// mapping" branch. That branch also uses upstreamHost(); exercise it.
	t.Setenv("SIMPLEDEPLOY_UPSTREAM_HOST", "host.docker.internal")
	app := makeApp("firstmap",
		[]compose.EndpointConfig{
			{Domain: "example.com", Service: "web"}, // no Port
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("4242", "80")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].Upstream != "host.docker.internal:4242" {
		t.Errorf("Upstream: got %q, want %q", routes[0].Upstream, "host.docker.internal:4242")
	}
}

func TestResolveRoutesSkipsEmptyDomain(t *testing.T) {
	app := makeApp("partial",
		[]compose.EndpointConfig{
			{Domain: "", Port: "3000", Service: "web"},
			{Domain: "real.example.com", Port: "3000", Service: "web"},
		},
		[]compose.ServiceConfig{
			{Name: "web", Ports: ports("3000", "3000")},
		},
	)
	routes, err := ResolveRoutes(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].Domain != "real.example.com" {
		t.Errorf("Domain: got %q, want %q", routes[0].Domain, "real.example.com")
	}
}
