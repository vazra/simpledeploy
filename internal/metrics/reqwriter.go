package metrics

import (
	"context"
	"time"

	"github.com/vazra/simpledeploy/internal/proxy"
)

// RequestStatsInserter is implemented by store.Store.
type RequestStatsInserter interface {
	InsertRequestStats(stats []RequestStat) error
}

type RequestStatsWriter struct {
	store     RequestStatsInserter
	in        <-chan proxy.RequestStatEvent
	appLookup func(domain string) (int64, error)
	bufSize   int
}

func NewRequestStatsWriter(st RequestStatsInserter, in <-chan proxy.RequestStatEvent, appLookup func(string) (int64, error), bufSize int) *RequestStatsWriter {
	return &RequestStatsWriter{store: st, in: in, appLookup: appLookup, bufSize: bufSize}
}

func (w *RequestStatsWriter) Run(ctx context.Context, flushInterval time.Duration) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]RequestStat, 0, w.bufSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		_ = w.store.InsertRequestStats(buf)
		buf = buf[:0]
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
			buf = append(buf, RequestStat{
				AppID:       appID,
				Timestamp:   time.Now().UTC(),
				StatusCode:  ev.StatusCode,
				LatencyMs:   ev.LatencyMs,
				Method:      ev.Method,
				PathPattern: proxy.NormalizePath(ev.Path),
				Tier:        TierRaw,
			})
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
