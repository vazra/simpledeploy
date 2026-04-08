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

	now := time.Now().UTC().Truncate(time.Second)
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, MemBytes: 100, Tier: metrics.TierRaw, Timestamp: now.Add(-2 * time.Minute)},
		{AppID: &appID, ContainerID: "c1", CPUPct: 20, MemBytes: 200, Tier: metrics.TierRaw, Timestamp: now.Add(-1 * time.Minute)},
		{AppID: &appID, ContainerID: "c1", CPUPct: 30, MemBytes: 300, Tier: metrics.TierRaw, Timestamp: now},
	}

	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	got, err := s.QueryMetrics(&appID, metrics.TierRaw, now.Add(-10*time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
}

func TestQueryMetricsByApp(t *testing.T) {
	s := newTestStore(t)
	idA := makeApp(t, s, "app-x")
	idB := makeApp(t, s, "app-y")

	now := time.Now().UTC().Truncate(time.Second)
	points := []metrics.MetricPoint{
		{AppID: &idA, ContainerID: "cA", CPUPct: 5, Tier: metrics.TierRaw, Timestamp: now},
		{AppID: &idB, ContainerID: "cB", CPUPct: 7, Tier: metrics.TierRaw, Timestamp: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	got, err := s.QueryMetrics(&idA, metrics.TierRaw, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ContainerID != "cA" {
		t.Errorf("ContainerID = %q, want cA", got[0].ContainerID)
	}
}

func TestQuerySystemMetrics(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	points := []metrics.MetricPoint{
		{AppID: nil, ContainerID: "host", CPUPct: 50, Tier: metrics.TierRaw, Timestamp: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	got, err := s.QueryMetrics(nil, metrics.TierRaw, now.Add(-time.Minute), now.Add(time.Minute))
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
		dur  time.Duration
		want string
	}{
		{30 * time.Minute, metrics.TierRaw},
		{time.Hour, metrics.TierRaw},
		{2 * time.Hour, metrics.Tier1m},
		{24 * time.Hour, metrics.Tier1m},
		{48 * time.Hour, metrics.Tier5m},
		{7 * 24 * time.Hour, metrics.Tier5m},
		{8 * 24 * time.Hour, metrics.Tier1h},
	}
	for _, c := range cases {
		got := SelectTier(c.dur)
		if got != c.want {
			t.Errorf("SelectTier(%v) = %q, want %q", c.dur, got, c.want)
		}
	}
}

func TestPruneMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "prune-app")

	now := time.Now().UTC().Truncate(time.Second)
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Timestamp: now.Add(-2 * time.Hour)},
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Timestamp: now.Add(-1 * time.Hour)},
		{AppID: &appID, ContainerID: "c1", Tier: metrics.TierRaw, Timestamp: now},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	// prune points older than 90 minutes ago (removes first two ... wait no: -2h and -1h both < -90m)
	cutoff := now.Add(-90 * time.Minute)
	n, err := s.PruneMetrics(metrics.TierRaw, cutoff)
	if err != nil {
		t.Fatalf("PruneMetrics: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}

	remaining, err := s.QueryMetrics(&appID, metrics.TierRaw, now.Add(-3*time.Hour), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryMetrics after prune: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %d, want 2", len(remaining))
	}
}

func TestAggregateMetrics(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "agg-app")

	// Insert 3 raw points within the same minute
	base := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	points := []metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", CPUPct: 10, MemBytes: 100, NetRx: 50, Tier: metrics.TierRaw, Timestamp: base},
		{AppID: &appID, ContainerID: "c1", CPUPct: 20, MemBytes: 200, NetRx: 60, Tier: metrics.TierRaw, Timestamp: base.Add(20 * time.Second)},
		{AppID: &appID, ContainerID: "c1", CPUPct: 30, MemBytes: 150, NetRx: 70, Tier: metrics.TierRaw, Timestamp: base.Add(40 * time.Second)},
	}
	if err := s.InsertMetrics(points); err != nil {
		t.Fatalf("InsertMetrics: %v", err)
	}

	olderThan := base.Add(2 * time.Minute)
	if err := s.AggregateMetrics(metrics.TierRaw, metrics.Tier1m, olderThan); err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}

	// query the 1m tier
	got, err := s.QueryMetrics(&appID, metrics.Tier1m, base.Add(-time.Minute), base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("QueryMetrics 1m: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected aggregated rows, got none")
	}
	// all 3 raw points fall in the same 1-minute bucket, so we get 1 aggregated row
	if len(got) != 1 {
		t.Errorf("aggregated rows = %d, want 1", len(got))
	}
	// avg cpu = (10+20+30)/3 = 20
	if got[0].CPUPct != 20 {
		t.Errorf("avg CPUPct = %v, want 20", got[0].CPUPct)
	}
	// max mem = 200
	if got[0].MemBytes != 200 {
		t.Errorf("max MemBytes = %v, want 200", got[0].MemBytes)
	}
	// sum net_rx = 180
	if got[0].NetRx != 180 {
		t.Errorf("sum NetRx = %v, want 180", got[0].NetRx)
	}
}
