package logbuf

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// maxLineBytes caps a single buffered message. Anything beyond is
// truncated with an explicit marker so a runaway log line cannot blow
// memory or carry a megabytes-long payload to UI subscribers.
const maxLineBytes = 8 * 1024

// ansiEscapeRe matches CSI / OSC / common terminal escape sequences. We
// strip these before storing because the buffer is rendered in many
// contexts (browser, curl, terminal); a crafted escape from a malicious
// container image's pull progress could otherwise hijack a viewer's
// terminal (cursor moves, OSC-52 clipboard, etc.).
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// sanitizeLogMessage strips ANSI escapes, removes control characters
// other than tab, and truncates to maxLineBytes.
func sanitizeLogMessage(msg string) string {
	msg = strings.TrimRight(msg, "\n")
	msg = ansiEscapeRe.ReplaceAllString(msg, "")
	// Strip ASCII control chars (keep tab, drop CR/LF/NUL/etc.).
	if strings.ContainsAny(msg, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0d\x0e\x0f") {
		var b strings.Builder
		b.Grow(len(msg))
		for _, r := range msg {
			if r == '\t' || r >= 0x20 {
				b.WriteRune(r)
			}
		}
		msg = b.String()
	}
	if len(msg) > maxLineBytes {
		msg = msg[:maxLineBytes] + "...[truncated]"
	}
	return msg
}

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
	e := Entry{
		Timestamp: time.Now(),
		Message:   sanitizeLogMessage(string(p)),
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
