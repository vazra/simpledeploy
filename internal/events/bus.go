// Package events provides an in-process publish/subscribe bus used to push
// notify-only realtime updates to UI clients via WebSocket. Events carry only
// type/topic metadata; consumers refetch via REST to read current state.
package events

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Event is a single notify-only message broadcast on the bus.
// Type is a coarse classification (e.g. "app.changed", "app.status").
// Topic identifies the resource scope (e.g. "app:foo", "global:settings").
// No payload data ships in events; clients refetch the relevant REST resource.
type Event struct {
	Type    string    `json:"type"`
	Topic   string    `json:"topic"`
	ActorID *int64    `json:"actor_id,omitempty"`
	Ts      time.Time `json:"ts"`
}

// subscriber wraps a delivery channel with an optional server-side filter.
type subscriber struct {
	ch     chan Event
	filter func(Event) bool
	stale  atomic.Bool
}

// Stale reports whether the subscriber dropped at least one event due to a
// full buffer. Reading is non-destructive; consumers should call Reset after
// reacting (e.g. by sending a synthetic resync).
func (s *subscriber) Stale() bool { return s.stale.Load() }

// Reset clears the stale flag.
func (s *subscriber) Reset() { s.stale.Store(false) }

// Bus is a single-process fan-out bus. Safe for concurrent use.
type Bus struct {
	mu   sync.RWMutex
	subs []*subscriber
}

// New constructs an empty Bus.
func New() *Bus { return &Bus{} }

const subBuffer = 64

// Subscribe registers a new subscriber. filter is evaluated server-side at
// publish time so events that fail filter never reach the channel; pass nil
// to receive all events. The returned cancel function unsubscribes and closes
// the channel; calling it more than once is safe.
//
// The returned read-only channel has buffer 64. On overflow, the oldest event
// is dropped and Stale() returns true so the handler can emit a synthetic
// resync frame.
func (b *Bus) Subscribe(filter func(Event) bool) (<-chan Event, func(), *subscriber) {
	s := &subscriber{ch: make(chan Event, subBuffer), filter: filter}
	b.mu.Lock()
	b.subs = append(b.subs, s)
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			for i, x := range b.subs {
				if x == s {
					b.subs = append(b.subs[:i], b.subs[i+1:]...)
					break
				}
			}
			b.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel, s
}

// Publish fans an event out to every subscriber whose filter accepts it.
// Never blocks: if a subscriber's buffer is full, the oldest event is
// discarded and the subscriber's stale flag is set.
func (b *Bus) Publish(_ context.Context, e Event) {
	if e.Ts.IsZero() {
		e.Ts = time.Now().UTC()
	}
	b.mu.RLock()
	subs := make([]*subscriber, len(b.subs))
	copy(subs, b.subs)
	b.mu.RUnlock()

	for _, s := range subs {
		if s.filter != nil && !s.filter(e) {
			continue
		}
		select {
		case s.ch <- e:
		default:
			// Drop oldest, push newest, mark stale.
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- e:
			default:
			}
			s.stale.Store(true)
		}
	}
}
