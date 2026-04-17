package auth

import (
	"sync"
	"time"
)

type bucket struct {
	tokens    int
	lastReset time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*bucket),
	}
}

// Allow returns true if the request is allowed, false if rate limited.
// Each key gets limit tokens per window; refills when window expires.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Periodic cleanup of stale buckets
	if len(rl.buckets) > 100 {
		cutoff := now.Add(-2 * rl.window)
		for k, v := range rl.buckets {
			if v.lastReset.Before(cutoff) {
				delete(rl.buckets, k)
			}
		}
	}

	b, exists := rl.buckets[key]
	if !exists || now.Sub(b.lastReset) >= rl.window {
		rl.buckets[key] = &bucket{tokens: rl.limit - 1, lastReset: now}
		return true
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}
