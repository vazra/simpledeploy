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



func TestRollupRunOnce(t *testing.T) {
	st := &mockAggStore{}
	tiers := []TierConfig{
		{Name: TierRaw, Retention: 2 * time.Hour},
		{Name: Tier1m, Retention: 24 * time.Hour},
		{Name: Tier5m, Retention: 7 * 24 * time.Hour},
		{Name: Tier1h, Retention: 90 * 24 * time.Hour},
	}
	rm := NewRollupManager(st, tiers)

	before := time.Now()
	if err := rm.RunOnce(); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	after := time.Now()

	// check 3 aggregation calls in order
	if len(st.aggregated) != 3 {
		t.Fatalf("got %d AggregateMetrics calls, want 3", len(st.aggregated))
	}
	aggs := st.aggregated
	if aggs[0].sourceTier != TierRaw || aggs[0].destTier != Tier1m {
		t.Errorf("agg[0]: got %s->%s, want raw->1m", aggs[0].sourceTier, aggs[0].destTier)
	}
	if aggs[1].sourceTier != Tier1m || aggs[1].destTier != Tier5m {
		t.Errorf("agg[1]: got %s->%s, want 1m->5m", aggs[1].sourceTier, aggs[1].destTier)
	}
	if aggs[2].sourceTier != Tier5m || aggs[2].destTier != Tier1h {
		t.Errorf("agg[2]: got %s->%s, want 5m->1h", aggs[2].sourceTier, aggs[2].destTier)
	}

	// check olderThan cutoffs are in the right range
	wantCutoffs := []time.Duration{2 * time.Minute, 10 * time.Minute, 2 * time.Hour}
	for i, wantOffset := range wantCutoffs {
		low := before.Add(-wantOffset - time.Second)
		high := after.Add(-wantOffset + time.Second)
		if aggs[i].olderThan.Before(low) || aggs[i].olderThan.After(high) {
			t.Errorf("agg[%d].olderThan = %v, want ~%v ago", i, aggs[i].olderThan, wantOffset)
		}
	}

	// check 4 prune calls (one per tier)
	if len(st.pruned) != len(tiers) {
		t.Fatalf("got %d PruneMetrics calls, want %d", len(st.pruned), len(tiers))
	}
	for i, tc := range tiers {
		if st.pruned[i].tier != tc.Name {
			t.Errorf("prune[%d].tier = %q, want %q", i, st.pruned[i].tier, tc.Name)
		}
	}
}

func TestPruneByRetention(t *testing.T) {
	st := &mockAggStore{}
	// very short retention: 1 minute
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

	// find the raw prune call
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
