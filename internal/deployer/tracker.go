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
	lines       chan OutputLine
	mu          sync.Mutex
	subscribers []chan OutputLine
	closed      bool
}

func newDeployLog() *DeployLog {
	dl := &DeployLog{
		lines: make(chan OutputLine, 100),
	}
	go dl.broadcast()
	return dl
}

func (dl *DeployLog) broadcast() {
	for line := range dl.lines {
		dl.mu.Lock()
		for _, sub := range dl.subscribers {
			select {
			case sub <- line:
			default: // drop if subscriber is slow
			}
		}
		dl.mu.Unlock()
	}
}

func (dl *DeployLog) Subscribe() (<-chan OutputLine, func()) {
	ch := make(chan OutputLine, 100)
	dl.mu.Lock()
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
	select {
	case dl.lines <- line:
	default:
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
	for _, sub := range dl.subscribers {
		select {
		case sub <- done:
		default:
		}
	}
	close(dl.lines)
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

func (t *Tracker) TrackWithLog(slug string, cancel context.CancelFunc) *DeployLog {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flights[slug] = cancel
	dl := newDeployLog()
	t.logs[slug] = dl
	return dl
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
