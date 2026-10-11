package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/store"
)

// initConfigSync runs the boot-time sidecar steps that come before git sync
// and the reconciler: disaster-recovery import of config.yml into an empty
// DB, orphan sidecar pruning, the mutation hook and the one-time backfill.
// App sidecars are imported later, by the reconciler, once it re-creates
// each app row from its folder.
func initConfigSync(db *store.Store, syncer *configsync.Syncer, dataDir string) {
	// Rehydrate global config if DB is empty (first boot after DR).
	if imported, err := syncer.ImportGlobalIfEmpty(); err != nil {
		log.Printf("[configsync] global import failed: %v", err)
	} else if imported {
		log.Printf("[configsync] imported global sidecar into empty DB")
	}

	// Prune sidecar files left in folders that are no longer apps (e.g. from
	// prior delete cycles before this feature existed). App folders with a
	// compose file keep theirs even without a DB row.
	if pruned, err := syncer.PruneOrphanSidecars(); err != nil {
		log.Printf("[configsync] prune orphan sidecars: %v", err)
	} else if len(pruned) > 0 {
		log.Printf("[configsync] pruned orphan sidecars for: %v", pruned)
	}

	// Install mutation hook so future mutations update sidecars.
	// FS-authoritative state writes are eventually consistent: this hook fires
	// after a successful DB commit and schedules a debounced FS write (500ms).
	// If the FS write fails, the reconciler watcher (internal/reconciler) re-
	// applies on the next file edit. Contract: DB success => FS reflects soon.
	// See docs/operations/state-on-disk.md.
	db.SetMutationHook(func(scope store.MutationScope, slug string) {
		switch scope {
		case store.ScopeApp:
			if slug != "" {
				syncer.ScheduleAppWrite(slug)
			}
		case store.ScopeGlobal:
			syncer.ScheduleGlobalWrite()
		}
	})

	// First-boot sidecar backfill: write sidecars for all existing DB state
	// so that upgrades from pre-configsync installs get sidecars on first boot.
	backfillMarker := filepath.Join(dataDir, ".configsync_backfill_v1")
	if _, err := os.Stat(backfillMarker); os.IsNotExist(err) {
		allApps, listErr := db.ListApps()
		if listErr != nil {
			log.Printf("[configsync] backfill: list apps failed: %v (skipping)", listErr)
		} else {
			allOK := true
			if backfillErr := syncer.WriteGlobal(); backfillErr != nil {
				log.Printf("[configsync] backfill: write global failed: %v", backfillErr)
				allOK = false
			}
			for _, a := range allApps {
				if werr := syncer.WriteAppSidecar(a.Slug); werr != nil {
					log.Printf("[configsync] backfill: write sidecar for %s: %v", a.Slug, werr)
					allOK = false
				}
			}
			if allOK {
				log.Printf("[configsync] first-boot sidecar backfill: wrote %d apps + global", len(allApps))
				if werr := os.WriteFile(backfillMarker, nil, 0600); werr != nil {
					log.Printf("[configsync] backfill: write marker failed: %v", werr)
				}
			}
		}
	}
}

// reloadConfigFromFS runs the one-time FS-authoritative seed, then applies
// the sidecars on disk to the DB. Runs after git sync is wired (it decides
// whether access lists are applied) and before the reconciler starts.
func reloadConfigFromFS(ctx context.Context, db *store.Store, syncer *configsync.Syncer, cfg *config.Config) error {
	// FS-authoritative seed (only on first boot post-upgrade).
	if err := configsync.RunFirstBootSeedIfNeeded(ctx, db, syncer, cfg); err != nil {
		return fmt.Errorf("fs-auth seed: %w", err)
	}
	// Reconcile DB cache from FS files (idempotent on every boot).
	if err := syncer.ReconcileDBFromFS(ctx); err != nil {
		return fmt.Errorf("fs-auth reload: %w", err)
	}
	return nil
}
