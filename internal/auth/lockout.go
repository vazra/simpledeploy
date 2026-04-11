package auth

import (
	"math"
	"sync"
	"time"
)

// LoginLockout tracks failed login attempts and enforces progressive lockout.
// After threshold failures, locks out with progressive duration: 1m, 2m, 4m, 8m, 16m, 30m (cap).
// Tracks per key (caller passes both username and IP as separate keys).
type LoginLockout struct {
	mu        sync.Mutex
	entries   map[string]*lockoutEntry
	threshold int
}

type lockoutEntry struct {
	failures    int
	lockedUntil time.Time
}

func NewLoginLockout(threshold int) *LoginLockout {
	return &LoginLockout{
		entries:   make(map[string]*lockoutEntry),
		threshold: threshold,
	}
}

// IsLocked returns true if the key is currently locked out.
func (l *LoginLockout) IsLocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return false
	}
	return time.Now().Before(e.lockedUntil)
}

// RecordFailure increments the failure count and sets lockout duration if threshold reached.
func (l *LoginLockout) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		e = &lockoutEntry{}
		l.entries[key] = e
	}
	e.failures++
	if e.failures >= l.threshold {
		exp := e.failures - l.threshold
		mins := math.Pow(2, float64(exp))
		if mins > 30 {
			mins = 30
		}
		e.lockedUntil = time.Now().Add(time.Duration(mins) * time.Minute)
	}
}

// RecordSuccess resets the entry for the given key.
func (l *LoginLockout) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// Cleanup removes entries where lockout has expired and failures are below threshold.
func (l *LoginLockout) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, e := range l.entries {
		if now.After(e.lockedUntil) && e.failures < l.threshold {
			delete(l.entries, k)
		}
	}
}
