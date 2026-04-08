package store

import (
	"testing"
	"time"
)

func TestInsertAndQueryRequestStats(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "stat-app-a")

	now := time.Now().UTC().Truncate(time.Second)
	stats := []RequestStat{
		{AppID: appID, Timestamp: now.Add(-2 * time.Minute), StatusCode: 200, LatencyMs: 10.0, Method: "GET", PathPattern: "/api/v1/users", Tier: "raw"},
		{AppID: appID, Timestamp: now.Add(-1 * time.Minute), StatusCode: 200, LatencyMs: 15.0, Method: "POST", PathPattern: "/api/v1/items", Tier: "raw"},
		{AppID: appID, Timestamp: now, StatusCode: 500, LatencyMs: 200.0, Method: "GET", PathPattern: "/api/v1/fail", Tier: "raw"},
	}

	if err := s.InsertRequestStats(stats); err != nil {
		t.Fatalf("InsertRequestStats: %v", err)
	}

	got, err := s.QueryRequestStats(appID, "raw", now.Add(-10*time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryRequestStats: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
}

func TestQueryRequestStatsByApp(t *testing.T) {
	s := newTestStore(t)
	idA := makeApp(t, s, "stat-app-x")
	idB := makeApp(t, s, "stat-app-y")

	now := time.Now().UTC().Truncate(time.Second)
	stats := []RequestStat{
		{AppID: idA, Timestamp: now, StatusCode: 200, LatencyMs: 5.0, Method: "GET", PathPattern: "/a", Tier: "raw"},
		{AppID: idB, Timestamp: now, StatusCode: 200, LatencyMs: 7.0, Method: "GET", PathPattern: "/b", Tier: "raw"},
	}
	if err := s.InsertRequestStats(stats); err != nil {
		t.Fatalf("InsertRequestStats: %v", err)
	}

	got, err := s.QueryRequestStats(idA, "raw", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryRequestStats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].PathPattern != "/a" {
		t.Errorf("PathPattern = %q, want /a", got[0].PathPattern)
	}
}

func TestPruneRequestStats(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "stat-prune-app")

	now := time.Now().UTC().Truncate(time.Second)
	stats := []RequestStat{
		{AppID: appID, Timestamp: now.Add(-2 * time.Hour), StatusCode: 200, LatencyMs: 10.0, Method: "GET", PathPattern: "/old1", Tier: "raw"},
		{AppID: appID, Timestamp: now.Add(-30 * time.Minute), StatusCode: 200, LatencyMs: 12.0, Method: "GET", PathPattern: "/new1", Tier: "raw"},
		{AppID: appID, Timestamp: now, StatusCode: 200, LatencyMs: 8.0, Method: "GET", PathPattern: "/new2", Tier: "raw"},
	}
	if err := s.InsertRequestStats(stats); err != nil {
		t.Fatalf("InsertRequestStats: %v", err)
	}

	cutoff := now.Add(-1 * time.Hour)
	n, err := s.PruneRequestStats("raw", cutoff)
	if err != nil {
		t.Fatalf("PruneRequestStats: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}

	remaining, err := s.QueryRequestStats(appID, "raw", now.Add(-3*time.Hour), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryRequestStats after prune: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %d, want 2", len(remaining))
	}
}

func TestAggregateRequestStats(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "stat-agg-app")

	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	stats := []RequestStat{
		{AppID: appID, Timestamp: base, StatusCode: 200, LatencyMs: 10.0, Method: "GET", PathPattern: "/api", Tier: "raw"},
		{AppID: appID, Timestamp: base.Add(20 * time.Second), StatusCode: 200, LatencyMs: 20.0, Method: "GET", PathPattern: "/api", Tier: "raw"},
		{AppID: appID, Timestamp: base.Add(40 * time.Second), StatusCode: 200, LatencyMs: 30.0, Method: "GET", PathPattern: "/api", Tier: "raw"},
	}
	if err := s.InsertRequestStats(stats); err != nil {
		t.Fatalf("InsertRequestStats: %v", err)
	}

	olderThan := base.Add(2 * time.Minute)
	if err := s.AggregateRequestStats("raw", "1m", olderThan); err != nil {
		t.Fatalf("AggregateRequestStats: %v", err)
	}

	got, err := s.QueryRequestStats(appID, "1m", base.Add(-time.Minute), base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("QueryRequestStats 1m: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected aggregated rows, got none")
	}
	if len(got) != 1 {
		t.Errorf("aggregated rows = %d, want 1", len(got))
	}
	// avg latency = (10+20+30)/3 = 20
	if got[0].LatencyMs != 20.0 {
		t.Errorf("avg LatencyMs = %v, want 20.0", got[0].LatencyMs)
	}
	// status group = (200/100)*100 = 200
	if got[0].StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", got[0].StatusCode)
	}
}
