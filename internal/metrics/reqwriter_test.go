package metrics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/proxy"
)

var errUnknownDomain = errors.New("unknown domain")

type mockReqStore struct {
	mu    sync.Mutex
	stats []RequestStat
}

func (m *mockReqStore) InsertRequestStats(stats []RequestStat) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = append(m.stats, stats...)
	return nil
}

func (m *mockReqStore) all() []RequestStat {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RequestStat, len(m.stats))
	copy(out, m.stats)
	return out
}

func TestRequestStatsWriterFlush(t *testing.T) {
	st := &mockReqStore{}
	ch := make(chan proxy.RequestStatEvent, 10)

	appLookup := func(domain string) (int64, error) {
		if domain == "example.com" {
			return 42, nil
		}
		return 0, errUnknownDomain
	}

	w := NewRequestStatsWriter(st, ch, appLookup, 100)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx, 10*time.Second)
	}()

	ch <- proxy.RequestStatEvent{
		Domain:     "example.com",
		StatusCode: 200,
		LatencyMs:  12.5,
		Method:     "GET",
		Path:       "/users/123",
	}
	ch <- proxy.RequestStatEvent{
		Domain:     "unknown.com",
		StatusCode: 404,
		LatencyMs:  1.0,
		Method:     "GET",
		Path:       "/foo",
	}
	ch <- proxy.RequestStatEvent{
		Domain:     "example.com",
		StatusCode: 500,
		LatencyMs:  99.9,
		Method:     "POST",
		Path:       "/orders",
	}

	// give goroutine time to read events
	time.Sleep(50 * time.Millisecond)

	cancel()
	<-done

	got := st.all()
	if len(got) != 2 {
		t.Fatalf("got %d stats, want 2 (unknown domain should be skipped)", len(got))
	}

	// verify first stat
	if got[0].AppID != 42 {
		t.Errorf("AppID = %d, want 42", got[0].AppID)
	}
	if got[0].StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", got[0].StatusCode)
	}
	if got[0].PathPattern != "/users/{id}" {
		t.Errorf("PathPattern = %q, want /users/{id}", got[0].PathPattern)
	}
	if got[0].Method != "GET" {
		t.Errorf("Method = %q, want GET", got[0].Method)
	}
	if got[0].Tier != "raw" {
		t.Errorf("Tier = %q, want raw", got[0].Tier)
	}

	// verify second stat
	if got[1].StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", got[1].StatusCode)
	}
	if got[1].PathPattern != "/orders" {
		t.Errorf("PathPattern = %q, want /orders", got[1].PathPattern)
	}
}

func TestRequestStatsWriterBufferFlush(t *testing.T) {
	st := &mockReqStore{}
	ch := make(chan proxy.RequestStatEvent, 20)

	appLookup := func(domain string) (int64, error) {
		return 1, nil
	}

	bufSize := 3
	w := NewRequestStatsWriter(st, ch, appLookup, bufSize)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx, 10*time.Second)
	}()

	for i := 0; i < bufSize; i++ {
		ch <- proxy.RequestStatEvent{Domain: "a.com", StatusCode: 200, Method: "GET", Path: "/"}
	}

	deadline := time.After(2 * time.Second)
	for {
		got := st.all()
		if len(got) >= bufSize {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d stats stored, want %d", len(got), bufSize)
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-done
}
