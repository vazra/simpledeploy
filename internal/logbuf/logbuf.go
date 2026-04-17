package logbuf

import (
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	maxSize int
	pos     int
	full    bool
	subs    map[chan Entry]struct{}
}

func New(size int) *Buffer {
	if size <= 0 {
		size = 1000
	}
	return &Buffer{
		entries: make([]Entry, size),
		maxSize: size,
		subs:    make(map[chan Entry]struct{}),
	}
}

func (b *Buffer) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	e := Entry{
		Timestamp: time.Now(),
		Message:   msg,
	}

	b.mu.Lock()
	b.entries[b.pos] = e
	b.pos++
	if b.pos >= b.maxSize {
		b.pos = 0
		b.full = true
	}
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
	b.mu.Unlock()

	return len(p), nil
}

func (b *Buffer) Recent(limit int) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var count int
	if b.full {
		count = b.maxSize
	} else {
		count = b.pos
	}
	if limit > count {
		limit = count
	}
	if limit <= 0 {
		return nil
	}

	result := make([]Entry, limit)
	for i := 0; i < limit; i++ {
		idx := b.pos - limit + i
		if idx < 0 {
			idx += b.maxSize
		}
		result[i] = b.entries[idx]
	}
	return result
}

func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = make([]Entry, b.maxSize)
	b.pos = 0
	b.full = false
}

func (b *Buffer) Subscribe() chan Entry {
	ch := make(chan Entry, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Buffer) Unsubscribe(ch chan Entry) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

func (b *Buffer) MaxSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.maxSize
}
