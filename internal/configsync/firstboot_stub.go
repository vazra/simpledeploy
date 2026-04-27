package configsync

import (
	"context"

	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/store"
)

// RunFirstBootSeedIfNeeded is a stub that will be implemented in Task 7.
// Today it returns nil so the bootstrap order is wired correctly.
func RunFirstBootSeedIfNeeded(ctx context.Context, db *store.Store, s *Syncer, cfg *config.Config) error {
	return nil
}

// ReconcileDBFromFS is a stub that will be implemented in Task 7.
func (s *Syncer) ReconcileDBFromFS(ctx context.Context) error {
	return nil
}
