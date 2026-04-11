package store

import (
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
)

func TestInsertAndQueryRequestMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-app-a")

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: now - 120, Tier: metrics.TierRaw, Count: 10, ErrorCount: 1, AvgLatency: 5.0, MaxLatency: 20.0},
		{AppID: appID, Ts: now - 60, Tier: metrics.TierRaw, Count: 20, ErrorCount: 2, AvgLatency: 8.0, MaxLatency: 30.0},
		{AppID: appID, Ts: now, Tier: metrics.TierRaw, Count: 15, ErrorCount: 0, AvgLatency: 3.0, MaxLatency: 10.0},
	}

	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	got, intervalSec, err := s.QueryRequestMetrics(appID, "1h")
	if err != nil {
		t.Fatalf("QueryRequestMetrics: %v", err)
	}
	if intervalSec != 10 {
		t.Errorf("intervalSec = %d, want 10", intervalSec)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
}

func TestQueryRequestMetricsByApp(t *testing.T) {
	s := newTestStore(t)
	idA := makeApp(t, s, "reqm-app-x")
	idB := makeApp(t, s, "reqm-app-y")

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: idA, Ts: now, Tier: metrics.TierRaw, Count: 5, AvgLatency: 2.0, MaxLatency: 5.0},
		{AppID: idB, Ts: now, Tier: metrics.TierRaw, Count: 7, AvgLatency: 4.0, MaxLatency: 8.0},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	got, _, err := s.QueryRequestMetrics(idA, "1h")
	if err != nil {
		t.Fatalf("QueryRequestMetrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Count != 5 {
		t.Errorf("Count = %d, want 5", got[0].Count)
	}
}

func TestPruneRequestMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-prune-app")

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: now - 7200, Tier: metrics.TierRaw, Count: 10, AvgLatency: 5.0, MaxLatency: 20.0},
		{AppID: appID, Ts: now - 1800, Tier: metrics.TierRaw, Count: 20, AvgLatency: 8.0, MaxLatency: 30.0},
		{AppID: appID, Ts: now, Tier: metrics.TierRaw, Count: 15, AvgLatency: 3.0, MaxLatency: 10.0},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	cutoff := time.Now().Add(-1 * time.Hour)
	n, err := s.PruneRequestMetrics(metrics.TierRaw, cutoff)
	if err != nil {
		t.Fatalf("PruneRequestMetrics: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}
}

func TestAggregateRequestMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-agg-app")

	// 3 raw points within the same minute
	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: base, Tier: metrics.TierRaw, Count: 100, ErrorCount: 5, AvgLatency: 10.0, MaxLatency: 50.0},
		{AppID: appID, Ts: base + 20, Tier: metrics.TierRaw, Count: 200, ErrorCount: 10, AvgLatency: 20.0, MaxLatency: 80.0},
		{AppID: appID, Ts: base + 40, Tier: metrics.TierRaw, Count: 300, ErrorCount: 15, AvgLatency: 30.0, MaxLatency: 60.0},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	olderThan := time.Unix(base+120, 0)
	if err := s.AggregateRequestMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateRequestMetrics: %v", err)
	}

	// query aggregated tier directly
	rows, err := s.db.Query(`
		SELECT count, error_count, avg_latency, max_latency
		FROM request_metrics WHERE tier = ? AND app_id = ?
	`, metrics.Tier1m, appID)
	if err != nil {
		t.Fatalf("query 1m: %v", err)
	}
	defer rows.Close()

	type result struct {
		count, errorCount int64
		avgLatency, maxLatency float64
	}
	var results []result
	for rows.Next() {
		var r result
		if err := rows.Scan(&r.count, &r.errorCount, &r.avgLatency, &r.maxLatency); err != nil {
			t.Fatalf("scan: %v", err)
		}
		results = append(results, r)
	}
	if len(results) != 1 {
		t.Fatalf("aggregated rows = %d, want 1", len(results))
	}
	r := results[0]
	// sum(count) = 100+200+300 = 600
	if r.count != 600 {
		t.Errorf("sum count = %d, want 600", r.count)
	}
	// sum(error_count) = 5+10+15 = 30
	if r.errorCount != 30 {
		t.Errorf("sum error_count = %d, want 30", r.errorCount)
	}
	// weighted avg latency = (10*100 + 20*200 + 30*300) / 600 = (1000+4000+9000)/600 = 23.333...
	wantAvg := 14000.0 / 600.0
	if r.avgLatency < wantAvg-0.01 || r.avgLatency > wantAvg+0.01 {
		t.Errorf("weighted avg latency = %v, want ~%v", r.avgLatency, wantAvg)
	}
	// max(max_latency) = 80
	if r.maxLatency != 80 {
		t.Errorf("max latency = %v, want 80", r.maxLatency)
	}

	// raw rows should be deleted
	var rawCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM request_metrics WHERE tier = ?`, metrics.TierRaw).Scan(&rawCount)
	if rawCount != 0 {
		t.Errorf("raw rows remaining = %d, want 0", rawCount)
	}
}

func TestInsertRequestMetricsEmpty(t *testing.T) {
	s := newTestStore(t)
	err := s.InsertRequestMetrics([]metrics.RequestMetricPoint{})
	if err != nil {
		t.Errorf("InsertRequestMetrics(empty) error = %v, want nil", err)
	}
}

func TestInsertRequestMetricsDefaultTier(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-default-tier")

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: now, Tier: "", Count: 100},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	var tierStored string
	err := s.db.QueryRow(`SELECT tier FROM request_metrics WHERE app_id = ?`, appID).Scan(&tierStored)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if tierStored != metrics.TierRaw {
		t.Errorf("stored tier = %q, want %q", tierStored, metrics.TierRaw)
	}
}

func TestInsertRequestMetricsAllFields(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-all-fields")

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: now, Tier: metrics.TierRaw, Count: 1000, ErrorCount: 50, AvgLatency: 125.5, MaxLatency: 500.0},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	var p metrics.RequestMetricPoint
	err := s.db.QueryRow(
		`SELECT app_id, ts, tier, count, error_count, avg_latency, max_latency FROM request_metrics WHERE app_id = ?`,
		appID,
	).Scan(&p.AppID, &p.Ts, &p.Tier, &p.Count, &p.ErrorCount, &p.AvgLatency, &p.MaxLatency)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}

	if p.Count != 1000 {
		t.Errorf("Count = %d, want 1000", p.Count)
	}
	if p.ErrorCount != 50 {
		t.Errorf("ErrorCount = %d, want 50", p.ErrorCount)
	}
	if p.AvgLatency != 125.5 {
		t.Errorf("AvgLatency = %v, want 125.5", p.AvgLatency)
	}
	if p.MaxLatency != 500.0 {
		t.Errorf("MaxLatency = %v, want 500.0", p.MaxLatency)
	}
}

func TestQueryRequestMetricsEmptyResult(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-empty")

	got, _, err := s.QueryRequestMetrics(appID, "1h")
	if err != nil {
		t.Fatalf("QueryRequestMetrics: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestQueryRequestMetricsAllRanges(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-ranges")

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: now - 365*86400, Tier: metrics.TierRaw, Count: 100},
		{AppID: appID, Ts: now - 30*86400, Tier: metrics.Tier1h, Count: 200},
		{AppID: appID, Ts: now - 7*86400, Tier: metrics.Tier5m, Count: 300},
		{AppID: appID, Ts: now - 3600, Tier: metrics.Tier1m, Count: 400},
		{AppID: appID, Ts: now, Tier: metrics.TierRaw, Count: 500},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	ranges := []string{"1h", "6h", "24h", "1w", "1m", "1yr", "unknown"}
	for _, r := range ranges {
		got, interval, err := s.QueryRequestMetrics(appID, r)
		if err != nil {
			t.Fatalf("QueryRequestMetrics(%q): %v", r, err)
		}
		if len(got) == 0 {
			t.Errorf("QueryRequestMetrics(%q) returned 0 points", r)
		}
		if interval <= 0 {
			t.Errorf("QueryRequestMetrics(%q) interval = %d, want > 0", r, interval)
		}
	}
}

func TestAggregateRequestMetricsNoData(t *testing.T) {
	s := newTestStore(t)
	olderThan := time.Now()
	if err := s.AggregateRequestMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateRequestMetrics with no data: %v", err)
	}
}

func TestAggregateRequestMetricsInvalidTier(t *testing.T) {
	s := newTestStore(t)
	olderThan := time.Now()
	if err := s.AggregateRequestMetrics("invalid", metrics.Tier1m, olderThan); err == nil {
		t.Error("AggregateRequestMetrics with invalid source tier: want error, got nil")
	}
	if err := s.AggregateRequestMetrics(metrics.TierRaw, "invalid", olderThan); err == nil {
		t.Error("AggregateRequestMetrics with invalid dest tier: want error, got nil")
	}
}

func TestAggregateRequestMetricsMultipleApps(t *testing.T) {
	s := newTestStore(t)
	appA := makeApp(t, s, "reqm-app-a")
	appB := makeApp(t, s, "reqm-app-b")

	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appA, Ts: base, Tier: metrics.TierRaw, Count: 100},
		{AppID: appA, Ts: base + 20, Tier: metrics.TierRaw, Count: 200},
		{AppID: appB, Ts: base, Tier: metrics.TierRaw, Count: 150},
		{AppID: appB, Ts: base + 20, Tier: metrics.TierRaw, Count: 250},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	olderThan := time.Unix(base+60, 0)
	if err := s.AggregateRequestMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateRequestMetrics: %v", err)
	}

	var countA, countB int
	s.db.QueryRow(`SELECT COUNT(*) FROM request_metrics WHERE tier = ? AND app_id = ?`, metrics.Tier1m, appA).Scan(&countA)
	s.db.QueryRow(`SELECT COUNT(*) FROM request_metrics WHERE tier = ? AND app_id = ?`, metrics.Tier1m, appB).Scan(&countB)

	if countA != 1 {
		t.Errorf("countA = %d, want 1", countA)
	}
	if countB != 1 {
		t.Errorf("countB = %d, want 1", countB)
	}
}

func TestAggregateRequestMetricsWeightedAveragePrecision(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-weighted")

	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: base, Tier: metrics.TierRaw, Count: 200, ErrorCount: 0, AvgLatency: 100, MaxLatency: 150},
		{AppID: appID, Ts: base + 20, Tier: metrics.TierRaw, Count: 100, ErrorCount: 0, AvgLatency: 50, MaxLatency: 75},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	olderThan := time.Unix(base+60, 0)
	if err := s.AggregateRequestMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateRequestMetrics: %v", err)
	}

	var avgLatency float64
	err := s.db.QueryRow(`SELECT avg_latency FROM request_metrics WHERE tier = ?`, metrics.Tier1m).Scan(&avgLatency)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	// (100*200 + 50*100) / 300 = 25000 / 300 = 83.333...
	expected := 83.33333333333333
	if avgLatency != expected {
		t.Errorf("AvgLatency = %v, want %v", avgLatency, expected)
	}
}

func TestPruneRequestMetricsNoRows(t *testing.T) {
	s := newTestStore(t)
	cutoff := time.Now()
	n, err := s.PruneRequestMetrics(metrics.TierRaw, cutoff)
	if err != nil {
		t.Fatalf("PruneRequestMetrics: %v", err)
	}
	if n != 0 {
		t.Errorf("pruned = %d, want 0", n)
	}
}

func TestPruneRequestMetricsMultipleTiers(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "reqm-prune-tiers")

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: appID, Ts: now - 7200, Tier: metrics.TierRaw, Count: 100},
		{AppID: appID, Ts: now - 7200, Tier: metrics.Tier1m, Count: 200},
		{AppID: appID, Ts: now, Tier: metrics.TierRaw, Count: 300},
		{AppID: appID, Ts: now, Tier: metrics.Tier1m, Count: 400},
	}
	if err := s.InsertRequestMetrics(points); err != nil {
		t.Fatalf("InsertRequestMetrics: %v", err)
	}

	cutoff := time.Now().Add(-90 * time.Minute)
	n, err := s.PruneRequestMetrics(metrics.TierRaw, cutoff)
	if err != nil {
		t.Fatalf("PruneRequestMetrics raw: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned raw = %d, want 1", n)
	}

	var count1m int
	s.db.QueryRow(`SELECT COUNT(*) FROM request_metrics WHERE tier = ?`, metrics.Tier1m).Scan(&count1m)
	if count1m != 2 {
		t.Errorf("1m tier count = %d, want 2", count1m)
	}
}
