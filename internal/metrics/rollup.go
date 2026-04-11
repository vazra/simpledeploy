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

// ReqMetricsAggregator is implemented by store.Store.
type ReqMetricsAggregator interface {
	AggregateRequestMetrics(sourceTier, destTier string, olderThan time.Time) error
	PruneRequestMetrics(tier string, before time.Time) (int64, error)
}

// ReqMetricsRollupManager handles rollup and pruning for request metrics.
type ReqMetricsRollupManager struct {
	store ReqMetricsAggregator
	tiers []TierConfig
}

func NewReqMetricsRollupManager(st ReqMetricsAggregator, tiers []TierConfig) *ReqMetricsRollupManager {
	return &ReqMetricsRollupManager{store: st, tiers: tiers}
}

// Run calls RunOnce every 60 seconds until ctx is cancelled.
func (rm *ReqMetricsRollupManager) Run(ctx context.Context) {
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

// RunOnce performs one round of aggregation and pruning for request metrics.
func (rm *ReqMetricsRollupManager) RunOnce() error {
	now := time.Now().UTC()

	if err := rm.store.AggregateRequestMetrics(TierRaw, Tier1m, now.Add(-1*time.Minute)); err != nil {
		return err
	}
	if err := rm.store.AggregateRequestMetrics(Tier1m, Tier5m, now.Add(-5*time.Minute)); err != nil {
		return err
	}
	if err := rm.store.AggregateRequestMetrics(Tier5m, Tier1h, now.Add(-1*time.Hour)); err != nil {
		return err
	}
	if err := rm.store.AggregateRequestMetrics(Tier1h, Tier1d, now.Add(-24*time.Hour)); err != nil {
		return err
	}

	for _, tc := range rm.tiers {
		if _, err := rm.store.PruneRequestMetrics(tc.Name, now.Add(-tc.Retention)); err != nil {
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

	// raw -> 1m
	if err := rm.store.AggregateMetrics(TierRaw, Tier1m, now.Add(-1*time.Minute)); err != nil {
		return err
	}
	// 1m -> 5m
	if err := rm.store.AggregateMetrics(Tier1m, Tier5m, now.Add(-5*time.Minute)); err != nil {
		return err
	}
	// 5m -> 1h
	if err := rm.store.AggregateMetrics(Tier5m, Tier1h, now.Add(-1*time.Hour)); err != nil {
		return err
	}
	// 1h -> 1d
	if err := rm.store.AggregateMetrics(Tier1h, Tier1d, now.Add(-24*time.Hour)); err != nil {
		return err
	}

	// Prune each tier based on retention config
	for _, tc := range rm.tiers {
		if _, err := rm.store.PruneMetrics(tc.Name, now.Add(-tc.Retention)); err != nil {
			return err
		}
	}

	return nil
}
