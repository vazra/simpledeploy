package metrics

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockStore implements MetricInserter for tests.
type mockStore struct {
	mu     sync.Mutex
	points []MetricPoint
}

func (m *mockStore) InsertMetrics(pts []MetricPoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.points = append(m.points, pts...)
	return nil
}

func (m *mockStore) all() []MetricPoint {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MetricPoint, len(m.points))
	copy(out, m.points)
	return out
}

func makePoints(n int) []MetricPoint {
	pts := make([]MetricPoint, n)
	for i := range pts {
		pts[i] = MetricPoint{
			ContainerID: "c1",
			CPUPct:      float64(i),
			MemBytes:    int64(i * 100),
			Tier:        TierRaw,
			Ts:          time.Now().Add(-time.Duration(n-i) * time.Second).Unix(),
		}
	}
	return pts
}

func TestWriterFlushesOnBufferFull(t *testing.T) {
	st := &mockStore{}
	ch := make(chan MetricPoint, 20)
	bufSize := 5

	w := NewWriter(st, ch, bufSize)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx, 10*time.Second)
	}()

	pts := makePoints(bufSize)
	for _, p := range pts {
		ch <- p
	}

	deadline := time.After(2 * time.Second)
	for {
		got := st.all()
		if len(got) >= bufSize {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d points stored, want %d", len(got), bufSize)
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-done
}

func TestWriterFlushesOnShutdown(t *testing.T) {
	st := &mockStore{}
	ch := make(chan MetricPoint, 20)
	bufSize := 10

	w := NewWriter(st, ch, bufSize)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx, 10*time.Second)
	}()

	// send fewer than bufSize points (no automatic flush)
	pts := makePoints(3)
	for _, p := range pts {
		ch <- p
	}

	// give goroutine time to read from channel
	time.Sleep(50 * time.Millisecond)

	cancel()
	<-done

	got := st.all()
	if len(got) != 3 {
		t.Errorf("got %d points, want 3", len(got))
	}
}
