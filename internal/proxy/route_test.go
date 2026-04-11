package proxy

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func makeApp(name, domain, port, tls string, services []compose.ServiceConfig) *compose.AppConfig {
	return &compose.AppConfig{
		Name:     name,
		Domain:   domain,
		Port:     port,
		TLS:      tls,
		Services: services,
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

func TestResolveRouteBasic(t *testing.T) {
	app := makeApp("myapp", "example.com", "8080", "auto", []compose.ServiceConfig{
		{Name: "web", Ports: ports("3000", "8080")},
	})
	r, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestResolveRouteNoDomain(t *testing.T) {
	app := makeApp("myapp", "", "", "", []compose.ServiceConfig{
		{Name: "web", Ports: ports("3000", "8080")},
	})
	_, err := ResolveRoute(app)
	if err == nil {
		t.Fatal("expected error for missing domain, got nil")
	}
}

func TestResolveRoutePortLookup(t *testing.T) {
	// Two services with multiple ports; app.Port targets container port 9000
	app := makeApp("api", "api.example.com", "9000", "", []compose.ServiceConfig{
		{Name: "web", Ports: ports("3000", "8080", "4000", "9000")},
	})
	r, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Upstream != "localhost:4000" {
		t.Errorf("Upstream: got %q, want %q", r.Upstream, "localhost:4000")
	}
}

func TestResolveRouteDefaultPort(t *testing.T) {
	// No app.Port label; should use first port mapping's host port
	app := makeApp("svc", "svc.example.com", "", "", []compose.ServiceConfig{
		{Name: "web", Ports: ports("5000", "80", "5001", "443")},
	})
	r, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Upstream != "localhost:5000" {
		t.Errorf("Upstream: got %q, want %q", r.Upstream, "localhost:5000")
	}
}

func TestResolveRouteTLSDefault(t *testing.T) {
	// TLS not set; should default to "auto"
	app := makeApp("svc", "svc.example.com", "", "", []compose.ServiceConfig{
		{Name: "web", Ports: ports("8000", "80")},
	})
	r, err := ResolveRoute(app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.TLS != "auto" {
		t.Errorf("TLS: got %q, want %q", r.TLS, "auto")
	}
}

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
