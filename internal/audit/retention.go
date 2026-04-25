package audit

import (
	"context"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

// Pruner periodically deletes audit_log rows older than the configured retention.
type Pruner struct {
	store    *store.Store
	interval time.Duration
}

// NewPruner creates a Pruner. Pass interval=0 to disable the Loop.
func NewPruner(s *store.Store, interval time.Duration) *Pruner {
	return &Pruner{store: s, interval: interval}
}

// RunOnce reads retention config and prunes if enabled (days > 0).
func (p *Pruner) RunOnce(ctx context.Context) error {
	days, err := p.store.GetAuditRetentionDays(ctx)
	if err != nil {
		return err
	}
	if days <= 0 {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	_, err = p.store.PruneAudit(ctx, cutoff)
	return err
}

// Loop runs RunOnce at every tick until ctx is cancelled. No-ops if interval <= 0.
func (p *Pruner) Loop(ctx context.Context) {
	if p.interval <= 0 {
		return
	}
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		_ = p.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
