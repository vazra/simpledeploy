package proxy

import (
	"net/http"
	"strings"
	"time"

	caddy "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// RequestStatsCh is set before Caddy starts; metrics are sent here.
var RequestStatsCh chan<- RequestStatEvent

// RequestStatEvent carries per-request stats.
type RequestStatEvent struct {
	Domain     string
	StatusCode int
	LatencyMs  float64
	Method     string
	Path       string
}

func init() {
	caddy.RegisterModule(RequestMetrics{})
}

// RequestMetrics is a Caddy middleware that records request stats.
type RequestMetrics struct{}

func (RequestMetrics) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.simpledeploy_metrics",
		New: func() caddy.Module { return new(RequestMetrics) },
	}
}

func (m *RequestMetrics) Provision(_ caddy.Context) error { return nil }
func (m *RequestMetrics) Validate() error                 { return nil }

func (m *RequestMetrics) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	start := time.Now()
	rw := &statusRecorder{ResponseWriter: w, status: 200}
	err := next.ServeHTTP(rw, r)
	latency := float64(time.Since(start).Milliseconds())

	if RequestStatsCh != nil {
		select {
		case RequestStatsCh <- RequestStatEvent{
			Domain:     r.Host,
			StatusCode: rw.status,
			LatencyMs:  latency,
			Method:     r.Method,
			Path:       NormalizePath(r.URL.Path),
		}:
		default: // drop if channel full
		}
	}
	return err
}

// statusRecorder captures the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap returns the underlying ResponseWriter for http.ResponseController.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// NormalizePath replaces dynamic path segments (numeric IDs, UUIDs) with {id}.
func NormalizePath(path string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if isIDSegment(seg) {
			segments[i] = "{id}"
		}
	}
	return strings.Join(segments, "/")
}

func isIDSegment(s string) bool {
	if s == "" {
		return false
	}
	// all digits
	allDigits := true
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}
	// UUID: 36 chars with exactly 4 hyphens
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	return false
}
