package metrics

import (
	"testing"
	"time"
)

// mockAggStore implements MetricAggregator for tests.
type mockAggStore struct {
	aggregated []aggregateCall
	pruned     []pruneCall
}

type aggregateCall struct {
	sourceTier, destTier string
	olderThan            time.Time
}

type pruneCall struct {
	tier   string
	before time.Time
}

func (m *mockAggStore) AggregateMetrics(sourceTier, destTier string, olderThan time.Time) error {
	m.aggregated = append(m.aggregated, aggregateCall{sourceTier, destTier, olderThan})
	return nil
}

func (m *mockAggStore) PruneMetrics(tier string, before time.Time) (int64, error) {
	m.pruned = append(m.pruned, pruneCall{tier, before})
	return 0, nil
}

// mockReqAggStore implements ReqMetricsAggregator for tests.
type mockReqAggStore struct {
	aggregated []aggregateCall
	pruned     []pruneCall
}

func (m *mockReqAggStore) AggregateRequestMetrics(sourceTier, destTier string, olderThan time.Time) error {
	m.aggregated = append(m.aggregated, aggregateCall{sourceTier, destTier, olderThan})
	return nil
}

func (m *mockReqAggStore) PruneRequestMetrics(tier string, before time.Time) (int64, error) {
	m.pruned = append(m.pruned, pruneCall{tier, before})
	return 0, nil
}

func TestRollupRunOnce(t *testing.T) {
	st := &mockAggStore{}
	tiers := []TierConfig{
		{Name: TierRaw, Retention: 2 * time.Hour},
		{Name: Tier1m, Retention: 24 * time.Hour},
		{Name: Tier5m, Retention: 7 * 24 * time.Hour},
		{Name: Tier1h, Retention: 90 * 24 * time.Hour},
		{Name: Tier1d, Retention: 365 * 24 * time.Hour},
	}
	rm := NewRollupManager(st, tiers)

	before := time.Now()
	if err := rm.RunOnce(); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	after := time.Now()

	// 4 aggregation calls: raw->1m, 1m->5m, 5m->1h, 1h->1d
	if len(st.aggregated) != 4 {
		t.Fatalf("got %d AggregateMetrics calls, want 4", len(st.aggregated))
	}
	aggs := st.aggregated
	wantTransitions := [][2]string{
		{TierRaw, Tier1m},
		{Tier1m, Tier5m},
		{Tier5m, Tier1h},
		{Tier1h, Tier1d},
	}
	for i, wt := range wantTransitions {
		if aggs[i].sourceTier != wt[0] || aggs[i].destTier != wt[1] {
			t.Errorf("agg[%d]: got %s->%s, want %s->%s", i, aggs[i].sourceTier, aggs[i].destTier, wt[0], wt[1])
		}
	}

	// check cutoffs
	wantCutoffs := []time.Duration{60 * time.Minute, 6 * time.Hour, 24 * time.Hour, 30 * 24 * time.Hour}
	for i, wantOffset := range wantCutoffs {
		low := before.Add(-wantOffset - time.Second)
		high := after.Add(-wantOffset + time.Second)
		if aggs[i].olderThan.Before(low) || aggs[i].olderThan.After(high) {
			t.Errorf("agg[%d].olderThan = %v, want ~%v ago", i, aggs[i].olderThan, wantOffset)
		}
	}

	// check prune calls (one per tier)
	if len(st.pruned) != len(tiers) {
		t.Fatalf("got %d PruneMetrics calls, want %d", len(st.pruned), len(tiers))
	}
	for i, tc := range tiers {
		if st.pruned[i].tier != tc.Name {
			t.Errorf("prune[%d].tier = %q, want %q", i, st.pruned[i].tier, tc.Name)
		}
	}
}

func TestReqMetricsRollupRunOnce(t *testing.T) {
	st := &mockReqAggStore{}
	tiers := []TierConfig{
		{Name: TierRaw, Retention: 2 * time.Hour},
		{Name: Tier1m, Retention: 24 * time.Hour},
		{Name: Tier5m, Retention: 7 * 24 * time.Hour},
		{Name: Tier1h, Retention: 90 * 24 * time.Hour},
		{Name: Tier1d, Retention: 365 * 24 * time.Hour},
	}
	rm := NewReqMetricsRollupManager(st, tiers)

	before := time.Now()
	if err := rm.RunOnce(); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	after := time.Now()

	// 4 aggregation calls
	if len(st.aggregated) != 4 {
		t.Fatalf("got %d AggregateRequestMetrics calls, want 4", len(st.aggregated))
	}
	aggs := st.aggregated
	wantTransitions := [][2]string{
		{TierRaw, Tier1m},
		{Tier1m, Tier5m},
		{Tier5m, Tier1h},
		{Tier1h, Tier1d},
	}
	for i, wt := range wantTransitions {
		if aggs[i].sourceTier != wt[0] || aggs[i].destTier != wt[1] {
			t.Errorf("agg[%d]: got %s->%s, want %s->%s", i, aggs[i].sourceTier, aggs[i].destTier, wt[0], wt[1])
		}
	}

	// check cutoffs
	wantCutoffs := []time.Duration{60 * time.Minute, 6 * time.Hour, 24 * time.Hour, 30 * 24 * time.Hour}
	for i, wantOffset := range wantCutoffs {
		low := before.Add(-wantOffset - time.Second)
		high := after.Add(-wantOffset + time.Second)
		if aggs[i].olderThan.Before(low) || aggs[i].olderThan.After(high) {
			t.Errorf("agg[%d].olderThan = %v, want ~%v ago", i, aggs[i].olderThan, wantOffset)
		}
	}

	// check prune calls
	if len(st.pruned) != len(tiers) {
		t.Fatalf("got %d PruneRequestMetrics calls, want %d", len(st.pruned), len(tiers))
	}
	for i, tc := range tiers {
		if st.pruned[i].tier != tc.Name {
			t.Errorf("prune[%d].tier = %q, want %q", i, st.pruned[i].tier, tc.Name)
		}
	}
}

func TestPruneByRetention(t *testing.T) {
	st := &mockAggStore{}
	tiers := []TierConfig{
		{Name: TierRaw, Retention: time.Minute},
	}
	rm := NewRollupManager(st, tiers)

	before := time.Now()
	if err := rm.RunOnce(); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	after := time.Now()

	if len(st.pruned) == 0 {
		t.Fatal("expected at least one PruneMetrics call")
	}

	var rawPrune *pruneCall
	for i := range st.pruned {
		if st.pruned[i].tier == TierRaw {
			rawPrune = &st.pruned[i]
			break
		}
	}
	if rawPrune == nil {
		t.Fatal("no PruneMetrics call for raw tier")
	}

	low := before.Add(-time.Minute - time.Second)
	high := after.Add(-time.Minute + time.Second)
	if rawPrune.before.Before(low) || rawPrune.before.After(high) {
		t.Errorf("raw prune cutoff = %v, want ~1m ago", rawPrune.before)
	}
}
