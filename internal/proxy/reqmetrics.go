package proxy

import (
	"context"
	"errors"
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
	rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	err := next.ServeHTTP(rw, r)
	latency := float64(time.Since(start).Milliseconds())

	status := rw.status
	if err != nil && !rw.wroteHeader {
		// Nothing reached the client yet: Caddy's error handling writes the
		// response (e.g. 502 when the upstream cannot be dialed) outside this
		// recorder, so take the status from the error.
		status = errorStatus(r, err)
	}

	if RequestStatsCh != nil {
		select {
		case RequestStatsCh <- RequestStatEvent{
			Domain:     r.Host,
			StatusCode: status,
			LatencyMs:  latency,
			Method:     r.Method,
			Path:       NormalizePath(r.URL.Path),
		}:
		default: // drop if channel full
		}
	}
	return err
}

// statusClientClosedRequest is the nginx-style status Caddy uses when the
// client went away before a response.
const statusClientClosedRequest = 499

// errorStatus returns the status for a request whose handler chain returned
// err without writing a response: the HandlerError's status (as Caddy's
// error handling writes it), 499 when the client went away, else 500.
func errorStatus(r *http.Request, err error) int {
	if he, ok := errors.AsType[caddyhttp.HandlerError](err); ok && he.StatusCode != 0 {
		return he.StatusCode
	}
	if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
		return statusClientClosedRequest
	}
	return http.StatusInternalServerError
}

// statusRecorder captures the final response status code (200 until one is
// written). Informational 1xx responses (100 Continue, 103 Early Hints) are
// skipped; 101 Switching Protocols is final.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader && (code < 100 || code > 199 || code == http.StatusSwitchingProtocols) {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

// Write marks the response as sent; without a prior status it is the
// implicit 200.
func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
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
