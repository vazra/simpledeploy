# Phase 5: Metrics Collection & Storage - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collect system and per-container metrics every 10s, store in SQLite with tiered rollup (raw/1m/5m/1h), expose query API with auto tier selection for dashboard charting.

**Architecture:** A collector goroutine polls Docker stats + gopsutil every 10s, writes to a buffered channel. A writer goroutine batch-inserts from the channel into SQLite. A rollup goroutine aggregates and prunes on schedule. Query API auto-selects tier based on requested time range.

**Tech Stack:** gopsutil (system metrics), Docker API (container stats), existing store/api/docker packages

---

## File Structure

```
internal/metrics/collector.go       - Metrics collector (Docker stats + gopsutil)
internal/metrics/collector_test.go
internal/metrics/writer.go          - Buffered channel writer, batch inserts
internal/metrics/writer_test.go
internal/metrics/rollup.go          - Tiered rollup + pruning
internal/metrics/rollup_test.go
internal/metrics/types.go           - MetricPoint, tier constants

internal/store/metrics.go           - Metrics insert/query methods
internal/store/metrics_test.go
internal/store/migrations/004_metrics.sql

internal/api/metrics.go             - Metrics query endpoints
internal/api/metrics_test.go

cmd/simpledeploy/main.go            - Wire metrics into serve
```

---

### Task 1: Metrics Types and Store

**Files:**
- Create: `internal/metrics/types.go`
- Create: `internal/store/migrations/004_metrics.sql`
- Create: `internal/store/metrics.go`
- Create: `internal/store/metrics_test.go`

#### types.go:
```go
package metrics

import "time"

type MetricPoint struct {
    AppID       *int64  // nil for system metrics
    ContainerID string  // empty for system metrics
    CPUPct      float64
    MemBytes    int64
    MemLimit    int64
    NetRx       int64
    NetTx       int64
    DiskRead    int64
    DiskWrite   int64
    Timestamp   time.Time
    Tier        string  // "raw", "1m", "5m", "1h"
}

const (
    TierRaw = "raw"
    Tier1m  = "1m"
    Tier5m  = "5m"
    Tier1h  = "1h"
)
```

#### Migration 004_metrics.sql:
```sql
CREATE TABLE IF NOT EXISTS metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER REFERENCES apps(id) ON DELETE CASCADE,
    container_id TEXT,
    cpu_pct REAL NOT NULL DEFAULT 0,
    mem_bytes INTEGER NOT NULL DEFAULT 0,
    mem_limit INTEGER NOT NULL DEFAULT 0,
    net_rx INTEGER NOT NULL DEFAULT 0,
    net_tx INTEGER NOT NULL DEFAULT 0,
    disk_read INTEGER NOT NULL DEFAULT 0,
    disk_write INTEGER NOT NULL DEFAULT 0,
    timestamp DATETIME NOT NULL,
    tier TEXT NOT NULL DEFAULT 'raw' CHECK(tier IN ('raw', '1m', '5m', '1h'))
);

CREATE INDEX IF NOT EXISTS idx_metrics_lookup ON metrics(app_id, tier, timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_tier_ts ON metrics(tier, timestamp);
```

#### Store methods in metrics.go:
- `InsertMetrics(points []MetricPoint) error` - batch insert
- `QueryMetrics(appID *int64, tier string, from, to time.Time) ([]MetricPoint, error)`
- `AggregateMetrics(sourceTier, destTier string, from, to time.Time) error` - aggregate and insert rolled-up data
- `PruneMetrics(tier string, before time.Time) (int64, error)` - delete old data, return rows deleted
- `SelectTier(duration time.Duration) string` - auto-select tier based on time range

#### Tests:
- TestInsertAndQueryMetrics
- TestQueryMetricsByApp
- TestSelectTier (last hour=raw, last 24h=1m, last week=5m, beyond=1h)
- TestPruneMetrics
- TestAggregateMetrics

- [ ] Steps: install gopsutil (`go get github.com/shirou/gopsutil/v4@latest`), write tests, implement, commit
- [ ] Commit: `git commit -m "add metrics types, store table, and query methods"`

---

### Task 2: System Metrics Collector

**Files:**
- Create: `internal/metrics/collector.go`
- Create: `internal/metrics/collector_test.go`

#### Collector:
```go
type Collector struct {
    docker docker.Client
    store  *store.Store
    out    chan<- MetricPoint
}

func NewCollector(docker docker.Client, store *store.Store, out chan<- MetricPoint) *Collector

// CollectSystem gathers host CPU, memory, disk, load via gopsutil
func (c *Collector) CollectSystem() (MetricPoint, error)

// CollectContainers gathers per-container stats via Docker API
func (c *Collector) CollectContainers(ctx context.Context) ([]MetricPoint, error)

// Run starts the collection loop (every 10s), sends to channel
func (c *Collector) Run(ctx context.Context, interval time.Duration)
```

CollectSystem uses gopsutil:
- `cpu.Percent(0, false)` for CPU %
- `mem.VirtualMemory()` for memory used/total
- `disk.Usage("/")` for disk
- System metrics have `AppID: nil, ContainerID: ""`

CollectContainers:
- List running containers with simpledeploy.project label
- For each: call Docker stats API (one-shot, stream=false)
- Parse stats JSON for CPU %, memory, network, disk IO
- Map container to app via `simpledeploy.project` label + store lookup

#### Tests:
- TestCollectSystem - verify returns non-zero CPU and memory values
- TestCollectContainersEmpty - mock Docker with no containers, returns empty
- Note: system tests need gopsutil to work (they will on any OS)

- [ ] Steps: write tests, implement, commit
- [ ] Commit: `git commit -m "add metrics collector for system and container stats"`

---

### Task 3: Metrics Writer (buffered channel to SQLite)

**Files:**
- Create: `internal/metrics/writer.go`
- Create: `internal/metrics/writer_test.go`

#### Writer:
```go
type Writer struct {
    store    *store.Store
    in       <-chan MetricPoint
    bufSize  int
}

func NewWriter(store *store.Store, in <-chan MetricPoint, bufSize int) *Writer

// Run reads from channel, batch-inserts when buffer is full or on ticker
func (w *Writer) Run(ctx context.Context, flushInterval time.Duration)
```

Logic:
1. Read from channel into buffer
2. When buffer reaches bufSize OR flushInterval ticker fires: batch insert to store
3. On context cancel: flush remaining buffer, return

#### Tests:
- TestWriterFlushesOnBufferFull
- TestWriterFlushesOnInterval
- TestWriterFlushesOnShutdown

- [ ] Steps: write tests, implement, commit
- [ ] Commit: `git commit -m "add metrics writer with buffered channel and batch inserts"`

---

### Task 4: Rollup and Pruning

**Files:**
- Create: `internal/metrics/rollup.go`
- Create: `internal/metrics/rollup_test.go`

#### Rollup:
```go
type TierConfig struct {
    Name      string
    Retention time.Duration
}

type RollupManager struct {
    store *store.Store
    tiers []TierConfig
}

func NewRollupManager(store *store.Store, tiers []TierConfig) *RollupManager

// Run starts the rollup loop
func (rm *RollupManager) Run(ctx context.Context)

// RunOnce performs one round of rollup + pruning
func (rm *RollupManager) RunOnce() error
```

RunOnce logic:
1. Aggregate raw -> 1m (for data older than 2 minutes)
2. Aggregate 1m -> 5m (for data older than 10 minutes)
3. Aggregate 5m -> 1h (for data older than 2 hours)
4. Prune each tier based on retention config

Aggregation: avg(cpu_pct), max(mem_bytes), last(mem_limit), sum(net_rx), sum(net_tx), sum(disk_read), sum(disk_write)

Run: executes RunOnce every minute.

#### Tests:
- TestRollupRawTo1m
- TestPruneByRetention

- [ ] Steps: write tests, implement, commit
- [ ] Commit: `git commit -m "add metrics rollup and pruning"`

---

### Task 5: Metrics API Endpoints

**Files:**
- Create: `internal/api/metrics.go`
- Create: `internal/api/metrics_test.go`
- Modify: `internal/api/server.go` (add routes)

#### Endpoints:

**GET /api/metrics/system?from=&to=**
- System metrics (app_id IS NULL)
- Auto-selects tier based on time range
- Returns JSON array of metric points

**GET /api/apps/{slug}/metrics?from=&to=**
- Per-app metrics
- Auto-selects tier
- Requires app access (uses existing app access middleware)

Response format:
```json
[
    {"timestamp": "...", "cpu_pct": 12.5, "mem_bytes": 1048576, "mem_limit": 4194304, "net_rx": 1024, "net_tx": 512, "disk_read": 0, "disk_write": 0},
    ...
]
```

Default time range: last 1 hour if not specified.

#### Tests:
- TestSystemMetricsEndpoint
- TestAppMetricsEndpoint
- TestMetricsAutoTierSelection

- [ ] Steps: write tests, implement, add routes, commit
- [ ] Commit: `git commit -m "add metrics query API endpoints"`

---

### Task 6: Wire Metrics into Serve + Tidy

**Files:**
- Modify: `cmd/simpledeploy/main.go`

In runServe, after creating Docker client and store:

```go
// metrics pipeline
metricsCh := make(chan metrics.MetricPoint, 500)
collector := metrics.NewCollector(dc, db, metricsCh)
writer := metrics.NewWriter(db, metricsCh, 100)

// parse tier config from cfg.Metrics.Tiers
tiers := parseTierConfigs(cfg.Metrics.Tiers)
rollup := metrics.NewRollupManager(db, tiers)

go collector.Run(ctx, 10*time.Second)
go writer.Run(ctx, 10*time.Second)
go rollup.Run(ctx)
```

Helper to parse config tiers to metrics.TierConfig:
```go
func parseTierConfigs(cfgTiers []config.MetricsTier) []metrics.TierConfig {
    // parse retention strings like "24h", "7d", "30d", "8760h" to time.Duration
}
```

- [ ] Steps: wire metrics, run full tests, go mod tidy, make build, commit
- [ ] Commit: `git commit -m "wire metrics pipeline into serve command"`

---

## Verification Checklist

- [ ] System metrics collected via gopsutil (CPU, memory, disk)
- [ ] Container metrics collected via Docker stats API
- [ ] Buffered channel writer batch-inserts to SQLite
- [ ] Tiered rollup: raw->1m->5m->1h with configurable retention
- [ ] Pruning removes old data per tier
- [ ] Query API auto-selects tier based on time range
- [ ] `GET /api/metrics/system` returns system metrics
- [ ] `GET /api/apps/{slug}/metrics` returns per-app metrics
- [ ] All tests pass
