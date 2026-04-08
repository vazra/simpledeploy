package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestRegistry() *RateLimiterRegistry {
	return &RateLimiterRegistry{limiters: make(map[string]*domainLimiter)}
}

func TestRateLimiterRegistryAllow(t *testing.T) {
	reg := newTestRegistry()
	reg.Set("example.com", &RateLimitConfig{
		Requests: 5,
		Window:   time.Minute,
		Burst:    0,
		By:       "ip",
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"

	for i := 0; i < 5; i++ {
		if !reg.Allow("example.com", req) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiterRegistryBlock(t *testing.T) {
	reg := newTestRegistry()
	reg.Set("example.com", &RateLimitConfig{
		Requests: 2,
		Window:   time.Minute,
		By:       "ip",
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"

	reg.Allow("example.com", req)
	reg.Allow("example.com", req)

	if reg.Allow("example.com", req) {
		t.Error("third request should be blocked")
	}
}

func TestRateLimiterUnconfigured(t *testing.T) {
	reg := newTestRegistry()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"

	for i := 0; i < 100; i++ {
		if !reg.Allow("unknown.com", req) {
			t.Fatalf("unconfigured domain should always be allowed (iteration %d)", i)
		}
	}
}

func TestExtractKeyIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:9999"

	key := extractKey("ip", req)
	if key != "192.168.1.1" {
		t.Errorf("extractKey ip: got %q, want %q", key, "192.168.1.1")
	}
}

func TestExtractKeyHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")

	key := extractKey("header:X-Real-IP", req)
	if key != "10.0.0.1" {
		t.Errorf("extractKey header: got %q, want %q", key, "10.0.0.1")
	}
}

func TestExtractKeyPath(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1", nil)
	key := extractKey("path", req)
	if key != "/api/v1" {
		t.Errorf("extractKey path: got %q, want /api/v1", key)
	}
}

func TestExtractKeyDefault(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "5.5.5.5:8080"
	key := extractKey("unknown", req)
	if key != "5.5.5.5:8080" {
		t.Errorf("extractKey default: got %q, want %q", key, "5.5.5.5:8080")
	}
}

func TestRateLimitHandlerModuleInfo(t *testing.T) {
	h := RateLimitHandler{}
	info := h.CaddyModule()
	if info.ID != "http.handlers.simpledeploy_ratelimit" {
		t.Errorf("module ID: got %q, want %q", info.ID, "http.handlers.simpledeploy_ratelimit")
	}
	if info.New == nil {
		t.Error("New is nil")
	}
}

func TestRateLimitHandlerBlocks(t *testing.T) {
	// Save and restore global registry.
	orig := RateLimiters
	defer func() { RateLimiters = orig }()

	RateLimiters = newTestRegistry()
	RateLimiters.Set("limited.com", &RateLimitConfig{
		Requests: 1,
		Window:   time.Minute,
		By:       "ip",
	})

	h := &RateLimitHandler{}
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "limited.com"
	req.RemoteAddr = "1.2.3.4:1234"

	// first request allowed
	w := httptest.NewRecorder()
	if err := h.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("first ServeHTTP: %v", err)
	}
	if w.Code == http.StatusTooManyRequests {
		t.Error("first request should not be rate-limited")
	}

	// second request blocked
	w2 := httptest.NewRecorder()
	if err := h.ServeHTTP(w2, req, nopHandler{}); err != nil {
		t.Fatalf("second ServeHTTP: %v", err)
	}
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: got status %d, want 429", w2.Code)
	}
	if w2.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After header: got %q, want %q", w2.Header().Get("Retry-After"), "60")
	}
}
