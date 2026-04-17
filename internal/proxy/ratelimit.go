package proxy

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	caddy "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// RateLimiters is the package-level registry used by the Caddy handler.
var RateLimiters = &RateLimiterRegistry{
	limiters: make(map[string]*domainLimiter),
}

// RateLimiterRegistry maps domains to their per-domain limiter.
type RateLimiterRegistry struct {
	mu       sync.RWMutex
	limiters map[string]*domainLimiter
}

// Set registers or replaces the rate limit config for domain.
func (reg *RateLimiterRegistry) Set(domain string, cfg *RateLimitConfig) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.limiters[domain] = &domainLimiter{
		requests: cfg.Requests,
		window:   cfg.Window,
		burst:    cfg.Burst,
		by:       cfg.By,
		buckets:  make(map[string]*rlBucket),
	}
}

// Allow returns true if the request for domain should be allowed.
// Returns true when no limiter is configured for the domain.
func (reg *RateLimiterRegistry) Allow(domain string, r *http.Request) bool {
	reg.mu.RLock()
	limiter, ok := reg.limiters[domain]
	reg.mu.RUnlock()
	if !ok {
		return true
	}
	key := extractKey(limiter.by, r)
	return limiter.allow(key)
}

type domainLimiter struct {
	mu       sync.Mutex
	requests int
	window   time.Duration
	burst    int
	by       string
	buckets  map[string]*rlBucket
}

type rlBucket struct {
	tokens     int
	lastReset  time.Time
	lastAccess time.Time
}

func (d *domainLimiter) allow(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Periodic cleanup: every 1000 calls, evict stale buckets
	if len(d.buckets) > 100 {
		cutoff := now.Add(-2 * d.window)
		for k, v := range d.buckets {
			if v.lastAccess.Before(cutoff) {
				delete(d.buckets, k)
			}
		}
	}

	b, ok := d.buckets[key]
	if !ok {
		b = &rlBucket{tokens: d.requests, lastReset: now}
		d.buckets[key] = b
	}
	b.lastAccess = now

	if now.Sub(b.lastReset) >= d.window {
		b.tokens = d.requests
		b.lastReset = now
	}

	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

func extractKey(by string, r *http.Request) string {
	switch {
	case by == "ip":
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		return host
	case strings.HasPrefix(by, "header:"):
		return r.Header.Get(strings.TrimPrefix(by, "header:"))
	case by == "path":
		return r.URL.Path
	default:
		return r.RemoteAddr
	}
}

// --- Caddy module ---

func init() {
	caddy.RegisterModule(RateLimitHandler{})
}

// RateLimitHandler is a Caddy middleware that enforces per-domain rate limits.
type RateLimitHandler struct{}

func (RateLimitHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.simpledeploy_ratelimit",
		New: func() caddy.Module { return new(RateLimitHandler) },
	}
}

func (h *RateLimitHandler) Provision(_ caddy.Context) error { return nil }
func (h *RateLimitHandler) Validate() error                 { return nil }

func (h *RateLimitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// RateLimiters are keyed by bare domain (Route.Domain). The Host header
	// may include a port (e.g. "example.com:8080" for curl --resolve or when
	// Caddy listens on a non-standard port), so strip it before lookup.
	host := r.Host
	if h2, _, err := net.SplitHostPort(host); err == nil && h2 != "" {
		host = h2
	}
	if !RateLimiters.Allow(host, r) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		return nil
	}
	return next.ServeHTTP(w, r)
}
