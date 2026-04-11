package store

import (
	"database/sql"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
)

func makeApp(t *testing.T, s *Store, slug string) int64 {
	t.Helper()
	app := &App{
		Name:        slug,
		Slug:        slug,
		ComposePath: "/apps/" + slug + "/docker-compose.yml",
		Status:      "running",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp %q: %v", slug, err)
	}
	return app.ID
}

func TestInsertAndQueryMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "app-a")

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, MemBytes: 100, Tier: metrics.TierRaw, Ts: now - 120},
		{AppID: &appID, ContainerID: "c1", CPUPct: 20, MemBytes: 200, Tier: metrics.TierRaw, Ts: now - 60},
		{AppID: &appID, ContainerID: "c1", CPUPct: 30, MemBytes: 300, Tier: metrics.TierRaw, Ts: now},
	}

	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	// "1h" range queries raw tier
	got, intervalSec, err := s.QueryMetrics(&appID, "1h")
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if intervalSec != 10 {
		t.Errorf("intervalSec = %d, want 10", intervalSec)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
}

func TestQueryMetricsByApp(t *testing.T) {
	s := newTestStore(t)
	idA := makeApp(t, s, "app-x")
	idB := makeApp(t, s, "app-y")

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{AppID: &idA, ContainerID: "cA", CPUPct: 5, Tier: metrics.TierRaw, Ts: now},
		{AppID: &idB, ContainerID: "cB", CPUPct: 7, Tier: metrics.TierRaw, Ts: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	got, _, err := s.QueryMetrics(&idA, "1h")
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].CPUPct != 5 {
		t.Errorf("CPUPct = %v, want 5", got[0].CPUPct)
	}
}

func TestQuerySystemMetrics(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{AppID: nil, ContainerID: "host", CPUPct: 50, Tier: metrics.TierRaw, Ts: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	got, _, err := s.QueryMetrics(nil, "1h")
	if err != nil {
		t.Fatalf("QueryMetrics nil appID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].AppID != nil {
		t.Errorf("AppID = %v, want nil", got[0].AppID)
	}
	if got[0].CPUPct != 50 {
		t.Errorf("CPUPct = %v, want 50", got[0].CPUPct)
	}
}

func TestSelectTiers(t *testing.T) {
	cases := []struct {
		rangeStr     string
		wantTiers    []string
		wantInterval int
	}{
		{"1h", []string{metrics.TierRaw}, 10},
		{"6h", []string{metrics.Tier1m, metrics.TierRaw}, 60},
		{"24h", []string{metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 300},
		{"1w", []string{metrics.Tier1h, metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 3600},
		{"1m", []string{metrics.Tier1h, metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 3600},
		{"1yr", []string{metrics.Tier1d, metrics.Tier1h, metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 86400},
		{"unknown", []string{metrics.TierRaw}, 10},
	}
	for _, c := range cases {
		tiers, interval := SelectTiers(c.rangeStr)
		if len(tiers) != len(c.wantTiers) {
			t.Errorf("SelectTiers(%q) tiers = %v, want %v", c.rangeStr, tiers, c.wantTiers)
		} else {
			for i := range tiers {
				if tiers[i] != c.wantTiers[i] {
					t.Errorf("SelectTiers(%q) tiers = %v, want %v", c.rangeStr, tiers, c.wantTiers)
					break
				}
			}
		}
		if interval != c.wantInterval {
			t.Errorf("SelectTiers(%q) interval = %d, want %d", c.rangeStr, interval, c.wantInterval)
		}
	}
}

func TestPruneMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "prune-app")

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Ts: now - 7200},  // 2h ago
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Ts: now - 3600},  // 1h ago
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Ts: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	// prune points older than 90 minutes ago (removes first one)
	cutoff := time.Now().Add(-90 * time.Minute)
	n, err := s.PruneMetrics(metrics.TierRaw, cutoff)
	if err != nil {
		t.Fatalf("PruneMetrics: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}
}

func TestAggregateMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "agg-app")

	// Insert 3 raw points within the same minute
	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, MemBytes: 100, NetRx: 50, Tier: metrics.TierRaw, Ts: base},
		{AppID: &appID, ContainerID: "c1", CPUPct: 20, MemBytes: 200, NetRx: 60, Tier: metrics.TierRaw, Ts: base + 20},
		{AppID: &appID, ContainerID: "c1", CPUPct: 30, MemBytes: 150, NetRx: 70, Tier: metrics.TierRaw, Ts: base + 40},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	olderThan := time.Unix(base+120, 0)
	if err := s.AggregateMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}

	// query the 1m tier directly to check aggregation
	rows, err := s.db.Query(`
		SELECT cpu_pct, mem_bytes, net_rx FROM metrics WHERE tier = ? AND app_id = ?
	`, metrics.Tier1m, appID)
	if err != nil {
		t.Fatalf("query 1m: %v", err)
	}
	defer rows.Close()

	var results []metrics.MetricPoint
	for rows.Next() {
		var p metrics.MetricPoint
		if err := rows.Scan(&p.CPUPct, &p.MemBytes, &p.NetRx); err != nil {
			t.Fatalf("scan: %v", err)
		}
		results = append(results, p)
	}
	if len(results) != 1 {
		t.Fatalf("aggregated rows = %d, want 1", len(results))
	}
	// avg cpu = (10+20+30)/3 = 20
	if results[0].CPUPct != 20 {
		t.Errorf("avg CPUPct = %v, want 20", results[0].CPUPct)
	}
	// avg mem_bytes = (100+200+150)/3 = 150
	if results[0].MemBytes != 150 {
		t.Errorf("avg MemBytes = %v, want 150", results[0].MemBytes)
	}
	// avg net_rx = (50+60+70)/3 = 60
	if results[0].NetRx != 60 {
		t.Errorf("avg NetRx = %v, want 60", results[0].NetRx)
	}

	// raw rows should be deleted
	var rawCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE tier = ?`, metrics.TierRaw).Scan(&rawCount)
	if rawCount != 0 {
		t.Errorf("raw rows remaining = %d, want 0", rawCount)
	}
}

func TestQueryMetricsPerContainer(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "multi-container")

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "web-1", CPUPct: 10, Tier: metrics.TierRaw, Ts: now - 30},
		{AppID: &appID, ContainerID: "web-2", CPUPct: 20, Tier: metrics.TierRaw, Ts: now - 30},
		{AppID: &appID, ContainerID: "web-1", CPUPct: 15, Tier: metrics.TierRaw, Ts: now},
		{AppID: &appID, ContainerID: "web-2", CPUPct: 25, Tier: metrics.TierRaw, Ts: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	got, _, err := s.QueryMetrics(&appID, "1h")
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
	// ordered by container_id, ts: web-1 first, then web-2
	if got[0].ContainerID != "web-1" {
		t.Errorf("got[0].ContainerID = %q, want web-1", got[0].ContainerID)
	}
	if got[2].ContainerID != "web-2" {
		t.Errorf("got[2].ContainerID = %q, want web-2", got[2].ContainerID)
	}
}

func TestInsertMetricsEmpty(t *testing.T) {
	s := newTestStore(t)
	err := s.InsertMetrics([]metrics.MetricPoint{})
	if err != nil {
		t.Errorf("InsertMetrics(empty) error = %v, want nil", err)
	}
}

func TestInsertMetricsDefaultTier(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "default-tier")

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, Tier: "", Ts: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	var tierStored string
	err := s.db.QueryRow(`SELECT tier FROM metrics WHERE app_id = ?`, appID).Scan(&tierStored)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if tierStored != metrics.TierRaw {
		t.Errorf("stored tier = %q, want %q", tierStored, metrics.TierRaw)
	}
}

func TestInsertMetricsAllFields(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "all-fields")

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{
			AppID:       &appID,
			ContainerID: "c1",
			CPUPct:      42.5,
			MemBytes:    1024,
			MemLimit:    2048,
			NetRx:       512.0,
			NetTx:       256.0,
			DiskRead:    100.0,
			DiskWrite:   50.0,
			Ts:          now,
			Tier:        metrics.TierRaw,
		},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	var p metrics.MetricPoint
	var dbAppID sql.NullInt64
	err := s.db.QueryRow(
		`SELECT app_id, container_id, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write, ts, tier FROM metrics WHERE app_id = ?`,
		appID,
	).Scan(&dbAppID, &p.ContainerID, &p.CPUPct, &p.MemBytes, &p.MemLimit, &p.NetRx, &p.NetTx, &p.DiskRead, &p.DiskWrite, &p.Ts, &p.Tier)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}

	if p.CPUPct != 42.5 {
		t.Errorf("CPUPct = %v, want 42.5", p.CPUPct)
	}
	if p.MemBytes != 1024 {
		t.Errorf("MemBytes = %d, want 1024", p.MemBytes)
	}
	if p.MemLimit != 2048 {
		t.Errorf("MemLimit = %d, want 2048", p.MemLimit)
	}
	if p.NetRx != 512.0 {
		t.Errorf("NetRx = %v, want 512.0", p.NetRx)
	}
	if p.NetTx != 256.0 {
		t.Errorf("NetTx = %v, want 256.0", p.NetTx)
	}
	if p.DiskRead != 100.0 {
		t.Errorf("DiskRead = %v, want 100.0", p.DiskRead)
	}
	if p.DiskWrite != 50.0 {
		t.Errorf("DiskWrite = %v, want 50.0", p.DiskWrite)
	}
}

func TestQueryMetricsEmptyResult(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "no-metrics")

	got, _, err := s.QueryMetrics(&appID, "1h")
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestQueryMetricsAllRanges(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "range-test")

	now := time.Now().Unix()
	// Insert points across different times and tiers
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, Tier: metrics.TierRaw, Ts: now - 365*86400},
		{AppID: &appID, ContainerID: "c1", CPUPct: 20, Tier: metrics.Tier1h, Ts: now - 30*86400},
		{AppID: &appID, ContainerID: "c1", CPUPct: 30, Tier: metrics.Tier5m, Ts: now - 7*86400},
		{AppID: &appID, ContainerID: "c1", CPUPct: 40, Tier: metrics.Tier1m, Ts: now - 3600},
		{AppID: &appID, ContainerID: "c1", CPUPct: 50, Tier: metrics.TierRaw, Ts: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	ranges := []string{"1h", "6h", "24h", "1w", "1m", "1yr", "unknown"}
	for _, r := range ranges {
		got, interval, err := s.QueryMetrics(&appID, r)
		if err != nil {
			t.Fatalf("QueryMetrics(%q): %v", r, err)
		}
		if len(got) == 0 {
			t.Errorf("QueryMetrics(%q) returned 0 points", r)
		}
		if interval <= 0 {
			t.Errorf("QueryMetrics(%q) interval = %d, want > 0", r, interval)
		}
	}
}

func TestRangeToDuration(t *testing.T) {
	cases := []struct {
		rangeStr string
		want     time.Duration
	}{
		{"1h", time.Hour},
		{"6h", 6 * time.Hour},
		{"24h", 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"1m", 30 * 24 * time.Hour},
		{"1yr", 365 * 24 * time.Hour},
		{"unknown", time.Hour},
	}
	for _, c := range cases {
		got := rangeToDuration(c.rangeStr)
		if got != c.want {
			t.Errorf("rangeToDuration(%q) = %v, want %v", c.rangeStr, got, c.want)
		}
	}
}

func TestTimeBucketAcrossRanges(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "bucket-ranges")

	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	points := []metrics.MetricPoint{
		// Insert points within the same minute - all will bucket together
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, Tier: metrics.TierRaw, Ts: base},
		{AppID: &appID, ContainerID: "c1", CPUPct: 20, Tier: metrics.TierRaw, Ts: base + 20},
		{AppID: &appID, ContainerID: "c1", CPUPct: 30, Tier: metrics.TierRaw, Ts: base + 45},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	// Aggregate to 1m tier - all 3 points should bucket into same minute
	olderThan := time.Unix(base+120, 0)
	if err := s.AggregateMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}

	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE tier = ? AND app_id = ?`, metrics.Tier1m, appID).Scan(&count)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 bucketed point for 1m tier, got %d", count)
	}
}

func TestValidateMetricsTier(t *testing.T) {
	validTiers := []string{
		metrics.TierRaw, metrics.Tier1m, metrics.Tier5m, metrics.Tier1h, metrics.Tier1d,
	}
	for _, tier := range validTiers {
		if err := validateMetricsTier(tier); err != nil {
			t.Errorf("validateMetricsTier(%q) error = %v, want nil", tier, err)
		}
	}

	invalidTiers := []string{"invalid", "2h", "10m", ""}
	for _, tier := range invalidTiers {
		if err := validateMetricsTier(tier); err == nil {
			t.Errorf("validateMetricsTier(%q) error = nil, want error", tier)
		}
	}
}

func TestAggregateMetricsMultipleApps(t *testing.T) {
	s := newTestStore(t)
	appA := makeApp(t, s, "app-a")
	appB := makeApp(t, s, "app-b")

	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	points := []metrics.MetricPoint{
		{AppID: &appA, ContainerID: "c1", CPUPct: 10, MemBytes: 100, Tier: metrics.TierRaw, Ts: base},
		{AppID: &appA, ContainerID: "c1", CPUPct: 20, MemBytes: 200, Tier: metrics.TierRaw, Ts: base + 20},
		{AppID: &appB, ContainerID: "c2", CPUPct: 30, MemBytes: 300, Tier: metrics.TierRaw, Ts: base},
		{AppID: &appB, ContainerID: "c2", CPUPct: 40, MemBytes: 400, Tier: metrics.TierRaw, Ts: base + 20},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	olderThan := time.Unix(base+60, 0)
	if err := s.AggregateMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}

	var countA, countB int
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE tier = ? AND app_id = ?`, metrics.Tier1m, appA).Scan(&countA)
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE tier = ? AND app_id = ?`, metrics.Tier1m, appB).Scan(&countB)

	if countA != 1 {
		t.Errorf("countA = %d, want 1", countA)
	}
	if countB != 1 {
		t.Errorf("countB = %d, want 1", countB)
	}
}

func TestAggregateMetricsNoData(t *testing.T) {
	s := newTestStore(t)
	olderThan := time.Now()
	if err := s.AggregateMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateMetrics with no data: %v", err)
	}
}

func TestAggregateMetricsInvalidTier(t *testing.T) {
	s := newTestStore(t)
	olderThan := time.Now()
	if err := s.AggregateMetrics("invalid", metrics.Tier1m, olderThan); err == nil {
		t.Error("AggregateMetrics with invalid source tier: want error, got nil")
	}
	if err := s.AggregateMetrics(metrics.TierRaw, "invalid", olderThan); err == nil {
		t.Error("AggregateMetrics with invalid dest tier: want error, got nil")
	}
}

func TestAggregateMetricsMaxMemLimit(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "max-mem")

	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, MemBytes: 100, MemLimit: 1024, Tier: metrics.TierRaw, Ts: base},
		{AppID: &appID, ContainerID: "c1", CPUPct: 20, MemBytes: 200, MemLimit: 2048, Tier: metrics.TierRaw, Ts: base + 20},
		{AppID: &appID, ContainerID: "c1", CPUPct: 30, MemBytes: 150, MemLimit: 1536, Tier: metrics.TierRaw, Ts: base + 40},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	olderThan := time.Unix(base+60, 0)
	if err := s.AggregateMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}

	var memLimit int64
	err := s.db.QueryRow(`SELECT mem_limit FROM metrics WHERE tier = ? AND app_id = ?`, metrics.Tier1m, appID).Scan(&memLimit)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if memLimit != 2048 {
		t.Errorf("mem_limit = %d, want 2048 (max)", memLimit)
	}
}

func TestPruneMetricsNoRows(t *testing.T) {
	s := newTestStore(t)
	cutoff := time.Now()
	n, err := s.PruneMetrics(metrics.TierRaw, cutoff)
	if err != nil {
		t.Fatalf("PruneMetrics: %v", err)
	}
	if n != 0 {
		t.Errorf("pruned = %d, want 0", n)
	}
}

func TestPruneMetricsMultipleTiers(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "prune-tiers")

	now := time.Now().Unix()
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Ts: now - 7200},
		{AppID: &appID, ContainerID: "c1", Tier: metrics.Tier1m, Ts: now - 7200},
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Ts: now},
		{AppID: &appID, ContainerID: "c1", Tier: metrics.Tier1m, Ts: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	cutoff := time.Now().Add(-90 * time.Minute)
	n, err := s.PruneMetrics(metrics.TierRaw, cutoff)
	if err != nil {
		t.Fatalf("PruneMetrics raw: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned raw = %d, want 1", n)
	}

	var count1m int
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE tier = ?`, metrics.Tier1m).Scan(&count1m)
	if count1m != 2 {
		t.Errorf("1m tier count = %d, want 2", count1m)
	}
}

