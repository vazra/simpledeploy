# Phase 6: API Metrics & Rate Limiting - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track every proxied request (status code, latency, method, path) per app. Per-app rate limiting via compose labels. Store request stats in SQLite with tiered rollup. Dashboard-ready query API.

**Architecture:** Custom Caddy handler modules registered via Go's init(). A metrics handler wraps each route to record request data to a package-level channel. A rate limiter handler checks token buckets per key. Both are added to the Caddy JSON config route chain. Request stats use the same tiered rollup pattern as container metrics.

**Tech Stack:** Caddy module system (caddyhttp.Handler), existing proxy/store packages

---

## File Structure

```
internal/proxy/reqmetrics.go        - Caddy request metrics handler module
internal/proxy/ratelimit.go         - Caddy rate limiter handler module
internal/proxy/reqmetrics_test.go
internal/proxy/ratelimit_test.go
internal/proxy/proxy.go             - Update buildConfig to include handlers

internal/store/reqstats.go          - request_stats insert/query/rollup
internal/store/reqstats_test.go
internal/store/migrations/005_request_stats.sql

internal/api/reqstats.go            - Request stats query endpoints
internal/api/reqstats_test.go

cmd/simpledeploy/main.go            - Wire request stats writer
```

---

### Task 1: Request Stats Store

**Files:**
- Create: `internal/store/migrations/005_request_stats.sql`
- Create: `internal/store/reqstats.go`
- Create: `internal/store/reqstats_test.go`

#### Migration:
```sql
CREATE TABLE IF NOT EXISTS request_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER REFERENCES apps(id) ON DELETE CASCADE,
    timestamp DATETIME NOT NULL,
    status_code INTEGER NOT NULL,
    latency_ms REAL NOT NULL,
    method TEXT NOT NULL,
    path_pattern TEXT NOT NULL,
    tier TEXT NOT NULL DEFAULT 'raw' CHECK(tier IN ('raw', '1m', '5m', '1h'))
);

CREATE INDEX IF NOT EXISTS idx_reqstats_lookup ON request_stats(app_id, tier, timestamp);
```

#### Types + methods:
```go
type RequestStat struct {
    AppID       int64
    Timestamp   time.Time
    StatusCode  int
    LatencyMs   float64
    Method      string
    PathPattern string
    Tier        string
}

func (s *Store) InsertRequestStats(stats []RequestStat) error
func (s *Store) QueryRequestStats(appID int64, tier string, from, to time.Time) ([]RequestStat, error)
func (s *Store) AggregateRequestStats(sourceTier, destTier string, olderThan time.Time) error
func (s *Store) PruneRequestStats(tier string, before time.Time) (int64, error)
```

AggregateRequestStats groups by app_id, method, path_pattern, time bucket. Aggregates: count, avg latency, status code distribution (store as separate rows per status code group: 2xx, 4xx, 5xx).

Actually, simpler approach: aggregate as count + avg_latency per (app, method, path, time_bucket, status_code_group). The status_code in aggregated rows represents the group (200 for 2xx, 400 for 4xx, 500 for 5xx).

#### Tests:
- TestInsertAndQueryRequestStats
- TestPruneRequestStats
- TestAggregateRequestStats

- [ ] Commit: `git commit -m "add request stats store with tiered rollup"`

---

### Task 2: Request Metrics Caddy Handler

**Files:**
- Create: `internal/proxy/reqmetrics.go`
- Create: `internal/proxy/reqmetrics_test.go`

#### Caddy module registration:

```go
package proxy

import (
    "net/http"
    "time"

    "github.com/caddyserver/caddy/v2"
    "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// Package-level channel for request metrics. Set before Caddy starts.
var RequestStatsCh chan<- RequestStatEvent

type RequestStatEvent struct {
    Domain     string
    StatusCode int
    LatencyMs  float64
    Method     string
    Path       string
}

func init() {
    caddy.RegisterModule(RequestMetrics{})
}

type RequestMetrics struct{}

func (RequestMetrics) CaddyModule() caddy.ModuleInfo {
    return caddy.ModuleInfo{
        ID:  "http.handlers.simpledeploy_metrics",
        New: func() caddy.Module { return new(RequestMetrics) },
    }
}

func (m *RequestMetrics) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
    start := time.Now()
    rw := &statusRecorder{ResponseWriter: w, status: 200}
    err := next.ServeHTTP(rw, r)
    latency := time.Since(start).Seconds() * 1000

    if RequestStatsCh != nil {
        RequestStatsCh <- RequestStatEvent{
            Domain:     r.Host,
            StatusCode: rw.status,
            LatencyMs:  latency,
            Method:     r.Method,
            Path:       r.URL.Path,
        }
    }
    return err
}

type statusRecorder struct {
    http.ResponseWriter
    status int
}

func (r *statusRecorder) WriteHeader(code int) {
    r.status = code
    r.ResponseWriter.WriteHeader(code)
}
```

#### Path normalization helper:
```go
func NormalizePath(path string, patterns []string) string {
    // If patterns are configured, try to match.
    // Otherwise, replace numeric segments: /users/123 -> /users/{id}
    // Simple regex: replace path segments that are all digits with {id}
}
```

#### Update buildConfig in proxy.go:

Add the metrics handler before the reverse_proxy handler in each route:
```json
{
    "match": [{"host": ["app.example.com"]}],
    "handle": [
        {"handler": "simpledeploy_metrics"},
        {"handler": "reverse_proxy", "upstreams": [{"dial": "localhost:3000"}]}
    ]
}
```

#### Tests:
- TestNormalizePath - /users/123 -> /users/{id}, /posts/abc -> /posts/abc (non-numeric kept)
- TestRequestMetricsModule - verify CaddyModule() returns correct info

Note: Testing the full Caddy handler pipeline is complex. Test the NormalizePath helper and module registration. The handler behavior is tested via integration in later phases.

- [ ] Commit: `git commit -m "add Caddy request metrics handler module"`

---

### Task 3: Rate Limiter Caddy Handler

**Files:**
- Create: `internal/proxy/ratelimit.go`
- Create: `internal/proxy/ratelimit_test.go`

#### Package-level rate limiter registry:
```go
var RateLimiters = &RateLimiterRegistry{
    limiters: make(map[string]*domainLimiter),
}

type RateLimiterRegistry struct {
    mu       sync.RWMutex
    limiters map[string]*domainLimiter
}

type domainLimiter struct {
    requests int
    window   time.Duration
    burst    int
    by       string // "ip", "header:X-API-Key", "path"
    buckets  map[string]*tokenBucket
    mu       sync.Mutex
}

type tokenBucket struct {
    tokens    int
    lastReset time.Time
}
```

#### Caddy module:
```go
func init() {
    caddy.RegisterModule(RateLimitHandler{})
}

type RateLimitHandler struct{}

func (RateLimitHandler) CaddyModule() caddy.ModuleInfo {
    return caddy.ModuleInfo{
        ID:  "http.handlers.simpledeploy_ratelimit",
        New: func() caddy.Module { return new(RateLimitHandler) },
    }
}

func (h *RateLimitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
    domain := r.Host
    if !RateLimiters.Allow(domain, r) {
        w.Header().Set("Retry-After", "60")
        w.WriteHeader(http.StatusTooManyRequests)
        return nil
    }
    return next.ServeHTTP(w, r)
}
```

#### Configure rate limits from routes:

Add to CaddyProxy:
```go
func (c *CaddyProxy) SetRoutes(routes []Route) error {
    c.mu.Lock()
    c.routes = routes
    c.mu.Unlock()

    // update rate limiter config per domain
    for _, r := range routes {
        if r.RateLimit != nil {
            RateLimiters.Set(r.Domain, r.RateLimit)
        }
    }
    return c.reload()
}
```

Add RateLimit to Route:
```go
type Route struct {
    AppSlug   string
    Domain    string
    Upstream  string
    TLS       string
    RateLimit *RateLimitConfig // can be nil
}

type RateLimitConfig struct {
    Requests int
    Window   time.Duration
    Burst    int
    By       string
}
```

Update ResolveRoute to extract rate limit labels from compose AppConfig.

Update buildConfig to add rate limit handler before metrics handler:
```json
"handle": [
    {"handler": "simpledeploy_ratelimit"},
    {"handler": "simpledeploy_metrics"},
    {"handler": "reverse_proxy", ...}
]
```

#### Tests:
- TestRateLimiterRegistryAllow - under limit passes
- TestRateLimiterRegistryBlock - over limit returns false
- TestRateLimiterRegistryByIP - different IPs get separate buckets

- [ ] Commit: `git commit -m "add Caddy rate limiter handler with per-domain config"`

---

### Task 4: Request Stats Writer + Rollup Integration

**Files:**
- Create: `internal/metrics/reqwriter.go`
- Create: `internal/metrics/reqwriter_test.go`

#### RequestStatsWriter:
```go
type RequestStatsWriter struct {
    store     RequestStatsInserter
    in        <-chan proxy.RequestStatEvent
    appLookup func(domain string) (int64, error) // domain -> app_id
    bufSize   int
}

type RequestStatsInserter interface {
    InsertRequestStats(stats []store.RequestStat) error
}

func NewRequestStatsWriter(st RequestStatsInserter, in <-chan proxy.RequestStatEvent, appLookup func(string) (int64, error), bufSize int) *RequestStatsWriter

func (w *RequestStatsWriter) Run(ctx context.Context, flushInterval time.Duration)
```

Converts RequestStatEvent -> store.RequestStat by looking up app_id from domain.
Applies path normalization before storing.

Also update the RollupManager to handle request_stats (add AggregateRequestStats + PruneRequestStats to the interface and RunOnce).

#### Tests:
- TestRequestStatsWriterFlush

- [ ] Commit: `git commit -m "add request stats writer and rollup integration"`

---

### Task 5: Request Stats API + Wire Everything

**Files:**
- Create: `internal/api/reqstats.go`
- Create: `internal/api/reqstats_test.go`
- Modify: `internal/api/server.go`
- Modify: `cmd/simpledeploy/main.go`

#### Endpoints:

**GET /api/apps/{slug}/requests?from=&to=**
- Per-app request stats (rate, latency, status distribution)
- Auto-selects tier
- Returns aggregated stats

Response:
```json
{
    "total_requests": 1234,
    "avg_latency_ms": 45.2,
    "status_codes": {"2xx": 1100, "4xx": 100, "5xx": 34},
    "points": [
        {"timestamp": "...", "count": 10, "avg_latency_ms": 42.0, "error_rate": 0.05}
    ]
}
```

#### Wiring in main.go:
```go
// request stats pipeline
reqStatsCh := make(chan proxy.RequestStatEvent, 1000)
proxy.RequestStatsCh = reqStatsCh

domainLookup := func(domain string) (int64, error) {
    // look up app by domain in store
}
reqWriter := metrics.NewRequestStatsWriter(db, reqStatsCh, domainLookup, 200)
go reqWriter.Run(ctx, 5*time.Second)
```

Set the channel before the proxy starts. Pass rate limit config from routes.

- [ ] Run full test suite, tidy, build
- [ ] Commit: `git commit -m "add request stats API and wire metrics pipeline"`

---

## Verification Checklist

- [ ] request_stats table with tiered rollup
- [ ] Custom Caddy metrics handler records every proxied request
- [ ] Path normalization (/users/123 -> /users/{id})
- [ ] Per-app rate limiting via compose labels
- [ ] Rate limited requests return 429 with Retry-After
- [ ] Request stats API endpoint
- [ ] All tests pass
