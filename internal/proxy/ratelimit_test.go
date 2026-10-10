package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRateLimiterRegistryHostVariants(t *testing.T) {
	reg := newTestRegistry()
	reg.Set("Limited.Example.com", &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"}) // mixed-case configured domain
	reg.Set("*.wild.example.com", &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"})

	cases := []struct{ first, second string }{
		{"limited.example.com", "LIMITED.EXAMPLE.COM"},       // uppercase host
		{"Limited.example.com:8443", "limited.example.com."}, // host:port, trailing dot
		{"a.wild.example.com", "B.Wild.Example.com:443"},     // wildcard (shared limiter)
	}
	for i, tc := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3." + string(rune('1'+i)) + ":1"
		if !reg.Allow(tc.first, req) {
			t.Fatalf("%s: first request should be allowed", tc.first)
		}
		if reg.Allow(tc.second, req) {
			t.Errorf("%s: second request should be limited (rule skipped)", tc.second)
		}
	}
}

func TestRateLimiterAllowForIsExact(t *testing.T) {
	reg := newTestRegistry()
	reg.Set("foo.*.com", &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"})
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1"
	reg.AllowFor("FOO.*.com", req)
	if reg.AllowFor("foo.*.com", req) {
		t.Error("AllowFor must use the bound route's limiter")
	}
	if !reg.AllowFor("foo.example.com", req) {
		t.Error("AllowFor must not resolve wildcards")
	}
}

func TestRateLimiterReplace(t *testing.T) {
	reg := newTestRegistry()
	cfg := &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1"

	reg.Replace(map[string]*RateLimitConfig{"a.com": cfg, "b.com": cfg})
	reg.Allow("a.com", req)
	// Same config: bucket state survives a route refresh.
	reg.Replace(map[string]*RateLimitConfig{"A.com": {Requests: 1, Window: time.Minute, By: "ip"}})
	if reg.Allow("a.com", req) {
		t.Error("unchanged config must keep counters across Replace")
	}
	if !reg.Allow("b.com", req) {
		t.Error("domain dropped from Replace must lose its limiter")
	}
	// Changed config: fresh limiter.
	reg.Replace(map[string]*RateLimitConfig{"a.com": {Requests: 2, Window: time.Minute, By: "ip"}})
	if !reg.Allow("a.com", req) {
		t.Error("changed config must start a fresh limiter")
	}
}

func TestDomainLimiterBucketCapEvictsExpiredThenOldest(t *testing.T) {
	now := time.Unix(1000, 0)
	d := newDomainLimiter(&RateLimitConfig{Requests: 2, Window: time.Minute, By: "path"})
	d.maxBuckets = 3
	d.now = func() time.Time { return now }

	d.allow("k1")
	now = now.Add(50 * time.Second)
	d.allow("k2")
	d.allow("k3")
	d.allow("k3") // k3 now has 0 tokens

	// k1 idle for a full window: evicted first, k2/k3 kept.
	now = now.Add(20 * time.Second)
	d.allow("k4")
	if _, ok := d.buckets["k1"]; ok {
		t.Error("expired bucket k1 should be evicted first")
	}
	for _, k := range []string{"k2", "k3", "k4"} {
		if _, ok := d.buckets[k]; !ok {
			t.Errorf("bucket %s evicted, want kept", k)
		}
	}

	// Nothing expired: least recently used (k2) goes.
	now = now.Add(time.Second)
	d.allow("k3") // touch k3 (still limited)
	d.allow("k5")
	if _, ok := d.buckets["k2"]; ok {
		t.Error("least recently used bucket k2 should be evicted")
	}
	if len(d.buckets) != 3 || d.lru.Len() != 3 {
		t.Errorf("buckets = %d (lru %d), want 3", len(d.buckets), d.lru.Len())
	}
	if d.allow("k3") {
		t.Error("k3 kept its state and must still be limited")
	}
}

func TestDomainLimiterBoundedUnderKeyFlood(t *testing.T) {
	d := newDomainLimiter(&RateLimitConfig{Requests: 5, Window: time.Minute, By: "path"})
	for i := 0; i < maxRateLimitBuckets*2; i++ {
		d.allow(fmt.Sprintf("/p/%d", i))
	}
	if len(d.buckets) > maxRateLimitBuckets || d.lru.Len() != len(d.buckets) {
		t.Fatalf("buckets = %d (lru %d), want <= %d", len(d.buckets), d.lru.Len(), maxRateLimitBuckets)
	}
}

func TestDomainLimiterHashesLongKeys(t *testing.T) {
	d := newDomainLimiter(&RateLimitConfig{Requests: 1, Window: time.Minute, By: "header:X-Key"})
	long := strings.Repeat("a", 64<<10)
	d.allow(long)
	if d.allow(long) {
		t.Error("same long key must share a bucket")
	}
	for k := range d.buckets {
		if len(k) > maxRateLimitKeyLen {
			t.Errorf("stored key length %d, want <= %d", len(k), maxRateLimitKeyLen)
		}
	}
}

func TestRateLimitHandlerHostWithPortAndCase(t *testing.T) {
	orig := RateLimiters
	defer func() { RateLimiters = orig }()
	RateLimiters = newTestRegistry()
	RateLimiters.Set("Limited.com", &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"})

	for _, h := range []*RateLimitHandler{{}, {Domain: "limited.com"}} {
		RateLimiters.Set("Limited.com", &RateLimitConfig{Requests: 1, Window: time.Minute, By: "ip"})
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:1"
		req.Host = "LIMITED.com:8443"
		_ = h.ServeHTTP(httptest.NewRecorder(), req, nopHandler{})
		w := httptest.NewRecorder()
		_ = h.ServeHTTP(w, req, nopHandler{})
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("handler %+v: second request status %d, want 429", h, w.Code)
		}
	}
}
