package metrics

import (
	"context"
	"time"
)

// MetricInserter is implemented by store.Store.
type MetricInserter interface {
	InsertMetrics(points []MetricPoint) error
}

type Writer struct {
	store   MetricInserter
	in      <-chan MetricPoint
	bufSize int
}

func NewWriter(st MetricInserter, in <-chan MetricPoint, bufSize int) *Writer {
	return &Writer{store: st, in: in, bufSize: bufSize}
}

func (w *Writer) Run(ctx context.Context, flushInterval time.Duration) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]MetricPoint, 0, w.bufSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		_ = w.store.InsertMetrics(buf)
		buf = buf[:0]
	}

	for {
		select {
		case pt, ok := <-w.in:
			if !ok {
				// channel closed: flush and return
				flush()
				return
			}
			buf = append(buf, pt)
			if len(buf) >= w.bufSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}
