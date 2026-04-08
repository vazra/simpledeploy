package reconciler

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceDuration = time.Second

// Watch runs an fsnotify watcher on appsDir and reconciles on changes.
// An initial Reconcile is run immediately. File events are debounced by 1 second.
// Returns nil when ctx is cancelled.
func (r *Reconciler) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer w.Close()

	if err := w.Add(r.appsDir); err != nil {
		return fmt.Errorf("watch dir: %w", err)
	}

	// initial reconcile
	if err := r.Reconcile(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "reconciler: initial reconcile: %v\n", err)
	}

	var debounce *time.Timer

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			_ = event
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(debounceDuration, func() {
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
