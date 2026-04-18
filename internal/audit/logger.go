package audit

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Username  string    `json:"username,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Success   bool      `json:"success"`
}

type Logger struct {
	mu      sync.RWMutex
	writer  io.Writer
	entries []Event
	maxSize int
	pos     int
	full    bool
}

func New(w io.Writer, bufferSize int) *Logger {
	if bufferSize <= 0 {
		bufferSize = 500
	}
	return &Logger{
		writer:  w,
		entries: make([]Event, bufferSize),
		maxSize: bufferSize,
	}
}

func (l *Logger) Log(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	data, err := json.Marshal(e)
	if err == nil {
		data = append(data, '\n')
		l.mu.Lock()
		_, _ = l.writer.Write(data)
		l.entries[l.pos] = e
		l.pos++
		if l.pos >= l.maxSize {
			l.pos = 0
			l.full = true
		}
		l.mu.Unlock()
	}
}

func (l *Logger) Recent(limit int) []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var count int
	if l.full {
		count = l.maxSize
	} else {
		count = l.pos
	}
	if limit > count {
		limit = count
	}
	if limit <= 0 {
		return nil
	}

	result := make([]Event, limit)
	for i := 0; i < limit; i++ {
		idx := l.pos - limit + i
		if idx < 0 {
			idx += l.maxSize
		}
		result[i] = l.entries[idx]
	}
	return result
}

// Clear resets the ring buffer, discarding all in-memory entries.
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make([]Event, l.maxSize)
	l.pos = 0
	l.full = false
}

// Resize changes the buffer capacity, discarding all existing entries.
func (l *Logger) Resize(newSize int) {
	if newSize < 10 {
		newSize = 10
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make([]Event, newSize)
	l.maxSize = newSize
	l.pos = 0
	l.full = false
}

// MaxSize returns the current buffer capacity.
func (l *Logger) MaxSize() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.maxSize
}
