package auth

import (
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(5, time.Second)
	for i := 0; i < 5; i++ {
		if !rl.Allow("user1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiterBlock(t *testing.T) {
	rl := NewRateLimiter(3, time.Second)
	for i := 0; i < 3; i++ {
		rl.Allow("user2")
	}
	if rl.Allow("user2") {
		t.Fatal("request over limit should be blocked")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)
	rl.Allow("user3")
	rl.Allow("user3")

	if rl.Allow("user3") {
		t.Fatal("should be blocked before window expires")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("user3") {
		t.Fatal("should be allowed after window refill")
	}
}
