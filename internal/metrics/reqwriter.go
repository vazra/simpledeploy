package metrics

import (
	"context"
	"time"

	"github.com/vazra/simpledeploy/internal/proxy"
)

// RequestMetricsInserter is implemented by store.Store.
type RequestMetricsInserter interface {
	InsertRequestMetrics(points []RequestMetricPoint) error
}

type RequestMetricsWriter struct {
	store     RequestMetricsInserter
	in        <-chan proxy.RequestStatEvent
	appLookup func(domain string) (int64, error)
	bufSize   int
}

func NewRequestMetricsWriter(st RequestMetricsInserter, in <-chan proxy.RequestStatEvent, appLookup func(string) (int64, error), bufSize int) *RequestMetricsWriter {
	return &RequestMetricsWriter{store: st, in: in, appLookup: appLookup, bufSize: bufSize}
}

// pending holds in-flight request counts per app per second bucket.
type pending struct {
	appID      int64
	ts         int64
	count      int64
	errorCount int64
	totalLat   float64
	maxLat     float64
}

func (w *RequestMetricsWriter) Run(ctx context.Context, flushInterval time.Duration) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	// bucket key: appID
	buckets := make(map[int64]*pending)

	flush := func() {
		if len(buckets) == 0 {
			return
		}
		pts := make([]RequestMetricPoint, 0, len(buckets))
		for _, b := range buckets {
			var avgLat float64
			if b.count > 0 {
				avgLat = b.totalLat / float64(b.count)
			}
			pts = append(pts, RequestMetricPoint{
				AppID:      b.appID,
				Ts:         b.ts,
				Tier:       TierRaw,
				Count:      b.count,
				ErrorCount: b.errorCount,
				AvgLatency: avgLat,
				MaxLatency: b.maxLat,
			})
		}
		_ = w.store.InsertRequestMetrics(pts)
		buckets = make(map[int64]*pending)
	}

	for {
		select {
		case ev, ok := <-w.in:
			if !ok {
				flush()
				return
			}
			appID, err := w.appLookup(ev.Domain)
			if err != nil {
				continue
			}
			b, ok := buckets[appID]
			if !ok {
				b = &pending{appID: appID, ts: time.Now().Unix()}
				buckets[appID] = b
			}
			b.count++
			if ev.StatusCode >= 500 {
				b.errorCount++
			}
			b.totalLat += ev.LatencyMs
			if ev.LatencyMs > b.maxLat {
				b.maxLat = ev.LatencyMs
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}
