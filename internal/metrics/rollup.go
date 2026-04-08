package metrics

import (
	"context"
	"time"
)

// MetricAggregator is implemented by store.Store.
type MetricAggregator interface {
	AggregateMetrics(sourceTier, destTier string, olderThan time.Time) error
	PruneMetrics(tier string, before time.Time) (int64, error)
}

// ReqStatsAggregator is implemented by store.Store.
type ReqStatsAggregator interface {
	AggregateRequestStats(sourceTier, destTier string, olderThan time.Time) error
	PruneRequestStats(tier string, before time.Time) (int64, error)
}

// ReqStatsRollupManager handles rollup and pruning for request_stats.
type ReqStatsRollupManager struct {
	store ReqStatsAggregator
	tiers []TierConfig
}

func NewReqStatsRollupManager(st ReqStatsAggregator, tiers []TierConfig) *ReqStatsRollupManager {
	return &ReqStatsRollupManager{store: st, tiers: tiers}
}

// Run calls RunOnce every 60 seconds until ctx is cancelled.
func (rm *ReqStatsRollupManager) Run(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = rm.RunOnce()
		}
	}
}

// RunOnce performs one round of aggregation and pruning for request stats.
func (rm *ReqStatsRollupManager) RunOnce() error {
	now := time.Now().UTC()

	if err := rm.store.AggregateRequestStats(TierRaw, Tier1m, now.Add(-2*time.Minute)); err != nil {
		return err
	}
	if err := rm.store.AggregateRequestStats(Tier1m, Tier5m, now.Add(-10*time.Minute)); err != nil {
		return err
	}
	if err := rm.store.AggregateRequestStats(Tier5m, Tier1h, now.Add(-2*time.Hour)); err != nil {
		return err
	}

	for _, tc := range rm.tiers {
		if _, err := rm.store.PruneRequestStats(tc.Name, now.Add(-tc.Retention)); err != nil {
			return err
		}
	}

	return nil
}

type TierConfig struct {
	Name      string
	Retention time.Duration
}

type RollupManager struct {
	store MetricAggregator
	tiers []TierConfig
}

func NewRollupManager(st MetricAggregator, tiers []TierConfig) *RollupManager {
	return &RollupManager{store: st, tiers: tiers}
}

// Run calls RunOnce every 60 seconds until ctx is cancelled.
func (rm *RollupManager) Run(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = rm.RunOnce()
		}
	}
}

// RunOnce performs one round of aggregation and pruning.
func (rm *RollupManager) RunOnce() error {
	now := time.Now().UTC()

	// raw -> 1m: aggregate data older than 2 minutes
	if err := rm.store.AggregateMetrics(TierRaw, Tier1m, now.Add(-2*time.Minute)); err != nil {
		return err
	}

	// 1m -> 5m: aggregate data older than 10 minutes
	if err := rm.store.AggregateMetrics(Tier1m, Tier5m, now.Add(-10*time.Minute)); err != nil {
		return err
	}

	// 5m -> 1h: aggregate data older than 2 hours
	if err := rm.store.AggregateMetrics(Tier5m, Tier1h, now.Add(-2*time.Hour)); err != nil {
		return err
	}

	// Prune each tier based on its retention config
	for _, tc := range rm.tiers {
		if _, err := rm.store.PruneMetrics(tc.Name, now.Add(-tc.Retention)); err != nil {
			return err
		}
	}

	return nil
}
