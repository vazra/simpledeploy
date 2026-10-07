package compose

import (
	"strings"
	"testing"
)

func TestNormalizeProtocol(t *testing.T) {
	if got := NormalizeProtocol("  GRPC "); got != "grpc" {
		t.Errorf("NormalizeProtocol = %q, want grpc", got)
	}
}

func TestValidProtocol(t *testing.T) {
	for _, p := range []string{"", "http", "h2c", "grpc"} {
		if !ValidProtocol(p) {
			t.Errorf("ValidProtocol(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"tcp", "HTTP", "grpc-web", "h2", "https"} {
		if ValidProtocol(p) {
			t.Errorf("ValidProtocol(%q) = true, want false", p)
		}
	}
}

func TestValidPath(t *testing.T) {
	good := []string{"/", "/ws*", "/api/v1/*", "/pkg.Service/*", "/a-b_c~d%20"}
	for _, p := range good {
		if !ValidPath(p) {
			t.Errorf("ValidPath(%q) = false, want true", p)
		}
	}
	bad := []string{"", "ws", "*", "/ws }", "/a\"b", "/a\nb", "/a?b=1", "/" + strings.Repeat("a", 256)}
	for _, p := range bad {
		if ValidPath(p) {
			t.Errorf("ValidPath(%q) = true, want false", p)
		}
	}
}

func TestEndpointMatchKey(t *testing.T) {
	httpEP := EndpointConfig{Domain: "a.com"}
	explicitHTTP := EndpointConfig{Domain: "a.com", Protocol: "http"}
	h2cEP := EndpointConfig{Domain: "a.com", Protocol: "h2c"}
	grpcEP := EndpointConfig{Domain: "a.com", Protocol: "grpc"}
	pathEP := EndpointConfig{Domain: "a.com", Path: "/ws*"}

	if EndpointMatchKey(httpEP) != EndpointMatchKey(h2cEP) {
		t.Error("http and h2c catch-alls on one domain must collide")
	}
	if EndpointMatchKey(httpEP) != EndpointMatchKey(explicitHTTP) {
		t.Error("empty protocol and http must collide")
	}
	if EndpointMatchKey(httpEP) == EndpointMatchKey(grpcEP) {
		t.Error("grpc and catch-all must not collide")
	}
	if EndpointMatchKey(httpEP) == EndpointMatchKey(pathEP) {
		t.Error("path route and catch-all must not collide")
	}
}

func TestValidateEndpointsOK(t *testing.T) {
	eps := []EndpointConfig{
		{Domain: "co.example.com", Port: "50051", Protocol: "grpc"},
		{Domain: "co.example.com", Port: "8000", Path: "/ws*"},
		{Domain: "co.example.com", Port: "8001"},
		{Domain: "ed.example.com", Port: "80", Protocol: "h2c"},
	}
	if v := ValidateEndpoints(eps); len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
}

func TestValidateEndpointsViolations(t *testing.T) {
	cases := []struct {
		name string
		eps  []EndpointConfig
		want string
	}{
		{"bad protocol", []EndpointConfig{{Domain: "a.com", Service: "web", Index: 2, Protocol: "tcp"}}, `service "web" label simpledeploy.endpoints.2 (a.com): invalid protocol`},
		{"bad path", []EndpointConfig{{Domain: "a.com", Service: "web", Index: 3, Path: "ws"}}, `service "web" label simpledeploy.endpoints.3 (a.com): invalid path`},
		{"duplicate catch-all", []EndpointConfig{{Domain: "a.com", Service: "web", Index: 0, Port: "80"}, {Domain: "a.com", Service: "api", Index: 4, Port: "81", Protocol: "h2c"}}, `service "api" label simpledeploy.endpoints.4 (a.com): duplicates service "web" label simpledeploy.endpoints.0`},
		{"duplicate grpc", []EndpointConfig{{Domain: "a.com", Service: "co", Index: 0, Protocol: "grpc"}, {Domain: "a.com", Service: "co", Index: 1, Protocol: "grpc"}}, `simpledeploy.endpoints.1 (a.com): duplicates service "co" label simpledeploy.endpoints.0`},
		{"duplicate path", []EndpointConfig{{Domain: "a.com", Service: "co", Index: 0, Path: "/ws*"}, {Domain: "a.com", Service: "co", Index: 1, Path: "/ws*"}}, `duplicates service "co" label simpledeploy.endpoints.0`},
		{"tls conflict", []EndpointConfig{{Domain: "a.com", Service: "co", Index: 0, TLS: "auto"}, {Domain: "a.com", Service: "co", Index: 1, Path: "/ws*", TLS: "off"}}, `service "co" label simpledeploy.endpoints.1 (a.com): tls "off" conflicts with tls "auto" of service "co" label simpledeploy.endpoints.0`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := ValidateEndpoints(tc.eps)
			if len(v) != 1 || !strings.Contains(v[0], tc.want) {
				t.Fatalf("got %v, want exactly one violation containing %q", v, tc.want)
			}
		})
	}
}

func TestValidateEndpointsTLSEquivalentValuesOK(t *testing.T) {
	eps := []EndpointConfig{
		{Domain: "a.com", Service: "co", Index: 0, Protocol: "grpc"},
		{Domain: "a.com", Service: "co", Index: 1, Path: "/ws*", TLS: "auto"},
		{Domain: "a.com", Service: "co", Index: 2, TLS: "letsencrypt"},
		{Domain: "b.com", Service: "co", Index: 3, TLS: "off"},
	}
	if v := ValidateEndpoints(eps); len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
}

func TestEffectiveTLS(t *testing.T) {
	for in, want := range map[string]string{"": "auto", "auto": "auto", "letsencrypt": "auto", " Off ": "off", "local": "local", "custom": "custom"} {
		if got := EffectiveTLS(in); got != want {
			t.Errorf("EffectiveTLS(%q) = %q, want %q", in, got, want)
		}
	}
}
