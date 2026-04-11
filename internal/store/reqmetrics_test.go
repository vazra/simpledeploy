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
