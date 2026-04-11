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
	stats []RequestMetricPoint
}

func (m *mockReqStore) InsertRequestMetrics(points []RequestMetricPoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = append(m.stats, points...)
	return nil
}

func (m *mockReqStore) all() []RequestMetricPoint {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RequestMetricPoint, len(m.stats))
	copy(out, m.stats)
	return out
}

func TestRequestMetricsWriterFlush(t *testing.T) {
	st := &mockReqStore{}
	ch := make(chan proxy.RequestStatEvent, 10)

	appLookup := func(domain string) (int64, error) {
		if domain == "example.com" {
			return 42, nil
		}
		return 0, errUnknownDomain
	}

	w := NewRequestMetricsWriter(st, ch, appLookup, 100)

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
	// events for example.com are bucketed into one point per app
	if len(got) != 1 {
		t.Fatalf("got %d points, want 1 (bucketed per app, unknown domain skipped)", len(got))
	}

	// verify the aggregated point
	if got[0].AppID != 42 {
		t.Errorf("AppID = %d, want 42", got[0].AppID)
	}
	if got[0].Count != 2 {
		t.Errorf("Count = %d, want 2", got[0].Count)
	}
	if got[0].ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1 (one 500)", got[0].ErrorCount)
	}
	if got[0].Tier != TierRaw {
		t.Errorf("Tier = %q, want raw", got[0].Tier)
	}
	// max latency should be 99.9
	if got[0].MaxLatency != 99.9 {
		t.Errorf("MaxLatency = %v, want 99.9", got[0].MaxLatency)
	}
}

func TestRequestMetricsWriterMultiApp(t *testing.T) {
	st := &mockReqStore{}
	ch := make(chan proxy.RequestStatEvent, 20)

	appLookup := func(domain string) (int64, error) {
		switch domain {
		case "a.com":
			return 1, nil
		case "b.com":
			return 2, nil
		}
		return 0, errUnknownDomain
	}

	w := NewRequestMetricsWriter(st, ch, appLookup, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx, 10*time.Second)
	}()

	ch <- proxy.RequestStatEvent{Domain: "a.com", StatusCode: 200, LatencyMs: 5.0, Method: "GET", Path: "/"}
	ch <- proxy.RequestStatEvent{Domain: "b.com", StatusCode: 200, LatencyMs: 10.0, Method: "GET", Path: "/"}
	ch <- proxy.RequestStatEvent{Domain: "a.com", StatusCode: 200, LatencyMs: 15.0, Method: "POST", Path: "/x"}

	time.Sleep(50 * time.Millisecond)

	cancel()
	<-done

	got := st.all()
	if len(got) != 2 {
		t.Fatalf("got %d points, want 2 (one per app)", len(got))
	}
}
