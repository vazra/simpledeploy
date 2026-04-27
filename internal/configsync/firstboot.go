package configsync

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/store"
)

const fsSeededKey = "fs_authoritative_seeded_at"

// RunFirstBootSeedIfNeeded checks system_meta for the seeded marker.
// If absent, writes per-app + global FS files from current DB state and
// records the marker. Idempotent.
func RunFirstBootSeedIfNeeded(ctx context.Context, db *store.Store, s *Syncer, cfg *config.Config) error {
	if _, ok, err := db.GetMeta(fsSeededKey); err != nil {
		return err
	} else if ok {
		return nil
	}

	n, err := db.BackfillBackupConfigUUIDs()
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("[fs-auth] backfilled %d backup_config UUIDs", n)
	}

	apps, err := db.ListAppsWithOptions(store.ListAppsOptions{IncludeArchived: true})
	if err != nil {
		return err
	}
	for _, a := range apps {
		if err := s.WriteAppSidecar(a.Slug); err != nil {
			log.Printf("[fs-auth] write app sidecar %s: %v", a.Slug, err)
		}
	}
	if err := s.WriteGlobal(); err != nil {
		return err
	}
	// Best-effort redacted global; not fatal.
	if err := s.WriteRedactedGlobal(); err != nil {
		log.Printf("[fs-auth] write redacted global: %v", err)
	}
	if err := db.SetMeta(fsSeededKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	log.Printf("[fs-auth] first-boot seed complete; FS is now the source of truth")
	return nil
}

// ReconcileDBFromFS scans the apps directory and applies each per-app sidecar
// to the DB, then applies the global sidecar. Missing files are skipped (not
// treated as deletions) so this is safe to run on every boot.
func (s *Syncer) ReconcileDBFromFS(ctx context.Context) error {
	entries, err := os.ReadDir(s.appsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				continue
			}
			loaded, err := s.LoadAppFromFS(name)
			if err != nil {
				log.Printf("[fs-auth] load %s: %v", name, err)
				continue
			}
			if loaded == nil || loaded.Sidecar == nil {
				continue
			}
			if err := s.ApplyAppSidecar(name, loaded); err != nil {
				log.Printf("[fs-auth] apply %s: %v", name, err)
			}
		}
	}

	g, err := s.LoadGlobalFromFS()
	if err != nil {
		return err
	}
	if g != nil && g.Sidecar != nil {
		if err := s.ApplyGlobalSidecar(g); err != nil {
			log.Printf("[fs-auth] apply global: %v", err)
		}
	}
	return nil
}
