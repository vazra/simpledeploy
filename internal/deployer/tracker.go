package deployer

import (
	"context"
	"fmt"
	"sync"
)

type Tracker struct {
	mu      sync.Mutex
	flights map[string]context.CancelFunc
}

func NewTracker() *Tracker {
	return &Tracker{flights: make(map[string]context.CancelFunc)}
}

func (t *Tracker) Track(slug string, cancel context.CancelFunc) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flights[slug] = cancel
}

func (t *Tracker) Done(slug string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.flights, slug)
}

func (t *Tracker) Cancel(slug string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	cancel, ok := t.flights[slug]
	if !ok {
		return fmt.Errorf("no in-flight deploy for %q", slug)
	}
	cancel()
	delete(t.flights, slug)
	return nil
}

func (t *Tracker) IsDeploying(slug string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.flights[slug]
	return ok
}
