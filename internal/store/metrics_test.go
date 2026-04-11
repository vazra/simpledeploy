package store

import (
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

func TestSelectTier(t *testing.T) {
	cases := []struct {
		rangeStr    string
		wantTier    string
		wantInterval int
	}{
		{"1h", metrics.TierRaw, 10},
		{"6h", metrics.Tier1m, 60},
		{"24h", metrics.Tier5m, 300},
		{"1w", metrics.Tier1h, 3600},
		{"1m", metrics.Tier1h, 3600},
		{"1yr", metrics.Tier1d, 86400},
		{"unknown", metrics.TierRaw, 10},
	}
	for _, c := range cases {
		tier, interval := SelectTier(c.rangeStr)
		if tier != c.wantTier {
			t.Errorf("SelectTier(%q) tier = %q, want %q", c.rangeStr, tier, c.wantTier)
		}
		if interval != c.wantInterval {
			t.Errorf("SelectTier(%q) interval = %d, want %d", c.rangeStr, interval, c.wantInterval)
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
