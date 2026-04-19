package deployer

import (
	"context"
	"fmt"
	"sync"
)

type OutputLine struct {
	Line   string `json:"line"`
	Stream string `json:"stream"` // "stdout" or "stderr"
	Done   bool   `json:"done"`
	Action string `json:"action,omitempty"` // set on done: "deploy", "pull_failed", etc.
}

type DeployLog struct {
	mu          sync.Mutex
	history     []OutputLine // buffered history for late subscribers
	subscribers []chan OutputLine
	closed      bool
}

func newDeployLog() *DeployLog {
	return &DeployLog{}
}

func (dl *DeployLog) Subscribe() (<-chan OutputLine, func()) {
	ch := make(chan OutputLine, 200)
	dl.mu.Lock()
	// replay history for late subscribers
	for _, line := range dl.history {
		select {
		case ch <- line:
		default:
		}
	}
	if dl.closed {
		close(ch)
		dl.mu.Unlock()
		return ch, func() {}
	}
	dl.subscribers = append(dl.subscribers, ch)
	dl.mu.Unlock()
	unsub := func() {
		dl.mu.Lock()
		for i, s := range dl.subscribers {
			if s == ch {
				dl.subscribers = append(dl.subscribers[:i], dl.subscribers[i+1:]...)
				break
			}
		}
		dl.mu.Unlock()
	}
	return ch, unsub
}

func (dl *DeployLog) Send(line OutputLine) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if dl.closed {
		return
	}
	dl.history = append(dl.history, line)
	if len(dl.history) > 10000 {
		dl.history = dl.history[len(dl.history)-10000:]
	}
	for _, sub := range dl.subscribers {
		select {
		case sub <- line:
		default:
		}
	}
}

func (dl *DeployLog) Close(action string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if dl.closed {
		return
	}
	dl.closed = true
	done := OutputLine{Done: true, Action: action}
	dl.history = append(dl.history, done)
	for _, sub := range dl.subscribers {
		select {
		case sub <- done:
		default:
		}
		close(sub)
	}
}

type Tracker struct {
	mu      sync.Mutex
	flights map[string]context.CancelFunc
	logs    map[string]*DeployLog
}

func NewTracker() *Tracker {
	return &Tracker{
		flights: make(map[string]context.CancelFunc),
		logs:    make(map[string]*DeployLog),
	}
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

// TrackWithLog registers an in-flight deploy and returns a fresh DeployLog.
// Returns (nil, false) if a deploy for the same slug is already in flight; the
// caller must skip its own docker compose invocation to avoid clobbering the
// existing subscribers and racing compose on the same project.
func (t *Tracker) TrackWithLog(slug string, cancel context.CancelFunc) (*DeployLog, bool) {
	if t == nil {
		return nil, true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, busy := t.logs[slug]; busy {
		return nil, false
	}
	t.flights[slug] = cancel
	dl := newDeployLog()
	t.logs[slug] = dl
	return dl, true
}

func (t *Tracker) DoneWithLog(slug string, action string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	dl := t.logs[slug]
	delete(t.flights, slug)
	delete(t.logs, slug)
	t.mu.Unlock()
	if dl != nil {
		dl.Close(action)
	}
}

func (t *Tracker) Subscribe(slug string) (<-chan OutputLine, func(), bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	dl, ok := t.logs[slug]
	if !ok {
		return nil, nil, false
	}
	ch, unsub := dl.Subscribe()
	return ch, unsub, true
}
