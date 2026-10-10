package proxy

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	caddy "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const (
	// maxRateLimitBuckets caps the buckets one domain limiter keeps, so
	// client-chosen keys (by=path, by=header:...) cannot grow memory
	// without bound.
	maxRateLimitBuckets = 10000
	// maxRateLimitKeyLen is the longest key stored as-is; longer keys (huge
	// header values or paths) are stored as their SHA-256.
	maxRateLimitKeyLen = 128
)

// RateLimiters is the package-level registry used by the Caddy handler.
var RateLimiters = &RateLimiterRegistry{
	limiters: make(map[string]*domainLimiter),
}

// RateLimiterRegistry maps normalized route domains to their limiter.
type RateLimiterRegistry struct {
	mu       sync.RWMutex
	limiters map[string]*domainLimiter
}

// Set registers or replaces the rate limit config for domain
// (case-insensitive; wildcard domains such as "*.example.com" are allowed).
func (reg *RateLimiterRegistry) Set(domain string, cfg *RateLimitConfig) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.limiters[normalizeDomain(domain)] = newDomainLimiter(cfg)
}

// Remove deletes the limiter for domain.
func (reg *RateLimiterRegistry) Remove(domain string) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	delete(reg.limiters, normalizeDomain(domain))
}

// Replace atomically swaps the whole set of limiters for cfgs (domain ->
// config). Domains not in cfgs lose their limiter. A domain whose config is
// unchanged keeps its limiter, so a route refresh does not reset counters.
func (reg *RateLimiterRegistry) Replace(cfgs map[string]*RateLimitConfig) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	next := make(map[string]*domainLimiter, len(cfgs))
	for domain, cfg := range cfgs {
		key := normalizeDomain(domain)
		if cur, ok := reg.limiters[key]; ok && cur.sameConfig(cfg) {
			next[key] = cur
			continue
		}
		next[key] = newDomainLimiter(cfg)
	}
	reg.limiters = next
}

// snapshot returns a copy of the current limiter set.
func (reg *RateLimiterRegistry) snapshot() map[string]*domainLimiter {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return maps.Clone(reg.limiters)
}

// restore swaps in a limiter set taken by snapshot.
func (reg *RateLimiterRegistry) restore(limiters map[string]*domainLimiter) {
	if limiters == nil {
		limiters = make(map[string]*domainLimiter)
	}
	reg.mu.Lock()
	reg.limiters = limiters
	reg.mu.Unlock()
}

// Allow returns true if the request should be allowed for host (a request
// Host header; port and case are ignored). host is matched against the
// registered domains the way Caddy's host matcher routes it: exact domain
// first, then the most specific wildcard.
// Returns true when no limiter is configured for the domain.
func (reg *RateLimiterRegistry) Allow(host string, r *http.Request) bool {
	reg.mu.RLock()
	limiter, ok := lookupHost(reg.limiters, host)
	reg.mu.RUnlock()
	if !ok {
		return true
	}
	return limiter.allow(extractKey(limiter.by, r))
}

// AllowFor is like Allow but uses the limiter registered for exactly the
// route domain (no wildcard resolution). Used by handlers bound to a route.
func (reg *RateLimiterRegistry) AllowFor(domain string, r *http.Request) bool {
	reg.mu.RLock()
	limiter, ok := reg.limiters[normalizeDomain(domain)]
	reg.mu.RUnlock()
	if !ok {
		return true
	}
	return limiter.allow(extractKey(limiter.by, r))
}

type domainLimiter struct {
	mu         sync.Mutex
	requests   int
	window     time.Duration
	burst      int
	by         string
	maxBuckets int
	now        func() time.Time
	buckets    map[string]*list.Element // values are *rlBucket
	lru        *list.List               // front = most recently used
}

type rlBucket struct {
	key        string
	tokens     int
	lastReset  time.Time
	lastAccess time.Time
}

func newDomainLimiter(cfg *RateLimitConfig) *domainLimiter {
	return &domainLimiter{
		requests:   cfg.Requests,
		window:     cfg.Window,
		burst:      cfg.Burst,
		by:         cfg.By,
		maxBuckets: maxRateLimitBuckets,
		now:        time.Now,
		buckets:    make(map[string]*list.Element),
		lru:        list.New(),
	}
}

func (d *domainLimiter) sameConfig(cfg *RateLimitConfig) bool {
	return cfg != nil && d.requests == cfg.Requests && d.window == cfg.Window && d.burst == cfg.Burst && d.by == cfg.By
}

func (d *domainLimiter) allow(key string) bool {
	if len(key) > maxRateLimitKeyLen {
		sum := sha256.Sum256([]byte(key))
		key = "sha256:" + hex.EncodeToString(sum[:])
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()

	// Drop buckets idle for two windows, least recently used first. Only
	// stale buckets are visited, so this stays cheap per request.
	d.evictIdle(now.Add(-2 * d.window))

	var b *rlBucket
	if el, ok := d.buckets[key]; ok {
		b = el.Value.(*rlBucket)
		d.lru.MoveToFront(el)
	} else {
		if len(d.buckets) >= d.maxBuckets {
			// Full. Buckets idle for a whole window would restart with full
			// tokens anyway, so dropping them loses nothing; if none are,
			// drop the least recently used.
			d.evictIdle(now.Add(-d.window))
			for len(d.buckets) >= d.maxBuckets && d.lru.Len() > 0 {
				d.removeElement(d.lru.Back())
			}
		}
		b = &rlBucket{key: key, tokens: d.requests, lastReset: now}
		d.buckets[key] = d.lru.PushFront(b)
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

// evictIdle removes buckets last used before cutoff, oldest first.
func (d *domainLimiter) evictIdle(cutoff time.Time) {
	for el := d.lru.Back(); el != nil; el = d.lru.Back() {
		if !el.Value.(*rlBucket).lastAccess.Before(cutoff) {
			return
		}
		d.removeElement(el)
	}
}

func (d *domainLimiter) removeElement(el *list.Element) {
	delete(d.buckets, el.Value.(*rlBucket).key)
	d.lru.Remove(el)
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
type RateLimitHandler struct {
	// Domain is the route domain whose limiter applies. buildConfig sets it
	// so the handler uses its own route's limiter even when several route
	// domains (e.g. two wildcards) match the request host. Empty falls back
	// to resolving the request Host against the registry.
	Domain string `json:"domain,omitempty"`
}

func (RateLimitHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.simpledeploy_ratelimit",
		New: func() caddy.Module { return new(RateLimitHandler) },
	}
}

func (h *RateLimitHandler) Provision(_ caddy.Context) error { return nil }
func (h *RateLimitHandler) Validate() error                 { return nil }

func (h *RateLimitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	var allowed bool
	if h.Domain != "" {
		allowed = RateLimiters.AllowFor(h.Domain, r)
	} else {
		// Host may carry a port (curl --resolve, non-standard listener) or
		// differ in case from the route domain; Allow normalizes both.
		allowed = RateLimiters.Allow(r.Host, r)
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		return nil
	}
	return next.ServeHTTP(w, r)
}
