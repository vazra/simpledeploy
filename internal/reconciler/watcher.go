package reconciler

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceDuration = time.Second

// sidecar file basenames recognized by the watcher.
const (
	appSidecarBase     = "simpledeploy.yml"
	appSecretsBase     = "simpledeploy.secrets.yml"
	globalSidecarBase  = "config.yml"
	globalSecretsBase  = "secrets.yml"
)

// Watch runs an fsnotify watcher on appsDir (and dataDir, for global sidecar
// edits) and reconciles on changes. An initial Reconcile runs immediately.
// Per-app sidecar edits route to a debounced ApplyAppSidecar call so the DB
// follows FS-authoritative state. Returns nil when ctx is cancelled.
func (r *Reconciler) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer w.Close()

	if err := w.Add(r.appsDir); err != nil {
		return fmt.Errorf("watch apps dir: %w", err)
	}
	// Add each existing app subdir so sidecar edits inside fire events.
	if entries, err := os.ReadDir(r.appsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			sub := filepath.Join(r.appsDir, e.Name())
			if err := w.Add(sub); err != nil {
				log.Printf("[reconciler] watch %s: %v", sub, err)
			}
		}
	}

	// Watch the data dir for global sidecar edits. Different from appsDir.
	dataDir := r.dataDirForWatcher()
	if dataDir != "" && dataDir != r.appsDir {
		if err := w.Add(dataDir); err != nil {
			log.Printf("[reconciler] watch data dir %s: %v", dataDir, err)
		}
	}

	// initial reconcile
	if err := r.Reconcile(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "reconciler: initial reconcile: %v\n", err)
	}

	var (
		mu             sync.Mutex
		pendingSlugs   = make(map[string]struct{})
		pendingGlobal  bool
		reconcileTimer *time.Timer
		sidecarTimer   *time.Timer
	)

	flushSidecars := func() {
		mu.Lock()
		slugs := pendingSlugs
		global := pendingGlobal
		pendingSlugs = make(map[string]struct{})
		pendingGlobal = false
		mu.Unlock()

		if r.syncer == nil {
			return
		}
		for slug := range slugs {
			loaded, err := r.syncer.LoadAppFromFS(slug)
			if err != nil {
				log.Printf("[reconciler] watcher LoadAppFromFS %s: %v", slug, err)
				continue
			}
			if loaded == nil || loaded.Sidecar == nil {
				continue
			}
			if err := r.syncer.ApplyAppSidecar(slug, loaded); err != nil {
				log.Printf("[reconciler] watcher ApplyAppSidecar %s: %v", slug, err)
				continue
			}
			log.Printf("[reconciler] watcher applied app sidecar: %s", slug)
		}
		if global {
			loaded, err := r.syncer.LoadGlobalFromFS()
			if err != nil {
				log.Printf("[reconciler] watcher LoadGlobalFromFS: %v", err)
			} else if loaded != nil && loaded.Sidecar != nil {
				if err := r.syncer.ApplyGlobalSidecar(loaded); err != nil {
					log.Printf("[reconciler] watcher ApplyGlobalSidecar: %v", err)
				} else {
					log.Printf("[reconciler] watcher applied global sidecar")
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			path := event.Name
			base := filepath.Base(path)

			// If a new app dir was created at the apps dir root, add it
			// to the watch set so sidecar edits inside fire events.
			if event.Op&fsnotify.Create != 0 {
				if filepath.Dir(path) == r.appsDir {
					if info, err := os.Stat(path); err == nil && info.IsDir() {
						if err := w.Add(path); err != nil {
							log.Printf("[reconciler] watch new app dir %s: %v", path, err)
						}
					}
				}
			}

			// Classify the event.
			isAppSidecar := false
			isGlobalSidecar := false
			var slug string
			parent := filepath.Dir(path)
			if parent == dataDir && (base == globalSidecarBase || base == globalSecretsBase) {
				isGlobalSidecar = true
			} else if filepath.Dir(parent) == r.appsDir && (base == appSidecarBase || base == appSecretsBase) {
				isAppSidecar = true
				slug = filepath.Base(parent)
			}

			// Sidecar writes that originated from configsync itself
			// (e.g. an API mutation -> debounced WriteAppSidecar) are
			// already in sync with the DB; re-applying them races with
			// in-flight DB mutations and reverts state. Skip events for
			// self-writes within a short window.
			if (isAppSidecar || isGlobalSidecar) && r.syncer != nil && r.syncer.IsSelfWrite(path, 5*time.Second) {
				continue
			}

			if isAppSidecar && slug != "" {
				mu.Lock()
				pendingSlugs[slug] = struct{}{}
				mu.Unlock()
				if sidecarTimer != nil {
					sidecarTimer.Stop()
				}
				sidecarTimer = time.AfterFunc(debounceDuration, flushSidecars)
				continue
			}
			if isGlobalSidecar {
				mu.Lock()
				pendingGlobal = true
				mu.Unlock()
				if sidecarTimer != nil {
					sidecarTimer.Stop()
				}
				sidecarTimer = time.AfterFunc(debounceDuration, flushSidecars)
				continue
			}

			// Compose / app dir change: trigger debounced Reconcile.
			if reconcileTimer != nil {
				reconcileTimer.Stop()
			}
			reconcileTimer = time.AfterFunc(debounceDuration, func() {
				if err := r.Reconcile(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "reconciler: reconcile: %v\n", err)
				}
			})

		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "reconciler: watcher error: %v\n", err)
		}
	}
}

// dataDirForWatcher returns the data directory the syncer is configured with,
// or "" if no syncer is wired (configsync disabled).
func (r *Reconciler) dataDirForWatcher() string {
	if r.syncer == nil {
		return ""
	}
	return r.syncer.DataDir()
}
