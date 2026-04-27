package recipes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheHitMissExpiry(t *testing.T) {
	var hits int32
	body := `{"schema_version":1,"generated_at":"2026-01-01T00:00:00Z","recipes":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, 0)
	cache := NewCache(c, 50*time.Millisecond)
	if _, err := cache.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("want 1 origin hit (cached), got %d", got)
	}
	time.Sleep(80 * time.Millisecond)
	if _, err := cache.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("want 2 origin hits after expiry, got %d", got)
	}
}

func TestCacheStaleOnError(t *testing.T) {
	var fail int32
	body := `{"schema_version":1,"generated_at":"x","recipes":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.LoadInt32(&fail) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, 0)
	cache := NewCache(c, 1*time.Millisecond)
	if _, err := cache.Index(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	atomic.StoreInt32(&fail, 1)
	idx, err := cache.Index(context.Background())
	if err != nil {
		t.Fatal("want stale fallback, got error:", err)
	}
	if idx == nil {
		t.Fatal("want stale index, got nil")
	}
}
