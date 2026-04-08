package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/users/123", "/users/{id}"},
		{"/posts/abc", "/posts/abc"},
		{"/a/550e8400-e29b-41d4-a716-446655440000/b", "/a/{id}/b"},
		{"/", "/"},
		{"/users/123/orders/456", "/users/{id}/orders/{id}"},
		{"/api/v2/items", "/api/v2/items"},
	}
	for _, c := range cases {
		got := NormalizePath(c.in)
		if got != c.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRequestMetricsModuleInfo(t *testing.T) {
	m := RequestMetrics{}
	info := m.CaddyModule()
	if info.ID != "http.handlers.simpledeploy_metrics" {
		t.Errorf("module ID: got %q, want %q", info.ID, "http.handlers.simpledeploy_metrics")
	}
	if info.New == nil {
		t.Error("New is nil")
	}
}

func TestStatusRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: 200}

	sr.WriteHeader(404)
	if sr.status != 404 {
		t.Errorf("status: got %d, want 404", sr.status)
	}
	// second write should be ignored
	sr.WriteHeader(500)
	if sr.status != 404 {
		t.Errorf("second WriteHeader should not change status: got %d", sr.status)
	}
	if sr.Unwrap() != rec {
		t.Error("Unwrap should return underlying ResponseWriter")
	}
}

// nopHandler is a caddyhttp.Handler that does nothing.
type nopHandler struct{}

func (nopHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) error { return nil }

func TestRequestMetricsServeHTTP(t *testing.T) {
	ch := make(chan RequestStatEvent, 1)
	RequestStatsCh = ch
	defer func() { RequestStatsCh = nil }()

	m := &RequestMetrics{}
	req := httptest.NewRequest("GET", "/users/42", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	if err := m.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Domain != "example.com" {
			t.Errorf("Domain: got %q, want %q", ev.Domain, "example.com")
		}
		if ev.StatusCode != 200 {
			t.Errorf("StatusCode: got %d, want 200", ev.StatusCode)
		}
		if ev.Path != "/users/{id}" {
			t.Errorf("Path: got %q, want %q", ev.Path, "/users/{id}")
		}
		if ev.Method != "GET" {
			t.Errorf("Method: got %q, want GET", ev.Method)
		}
	default:
		t.Fatal("no event sent to channel")
	}
}

func TestRequestMetricsNilChannel(t *testing.T) {
	RequestStatsCh = nil
	m := &RequestMetrics{}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	// should not panic
	if err := m.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
}

// Ensure nopHandler satisfies caddyhttp.Handler at compile time.
var _ caddyhttp.Handler = nopHandler{}
