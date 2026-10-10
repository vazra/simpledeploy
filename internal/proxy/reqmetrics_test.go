package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/users/123", "/users/{id}"},
		{"/posts/abc", "/posts/abc"},
		{"/a/550e8400-e29b-41d4-a716-446655440000/b", "/a/{id}/b"},
		{"/", "/"},
		{"/users/123/orders/456", "/users/{id}/orders/{id}"},
		{"/api/v2/items", "/api/v2/items"},
	}
	for _, c := range cases {
		got := NormalizePath(c.in)
		if got != c.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRequestMetricsModuleInfo(t *testing.T) {
	m := RequestMetrics{}
	info := m.CaddyModule()
	if info.ID != "http.handlers.simpledeploy_metrics" {
		t.Errorf("module ID: got %q, want %q", info.ID, "http.handlers.simpledeploy_metrics")
	}
	if info.New == nil {
		t.Error("New is nil")
	}
}

func TestStatusRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: 200}

	sr.WriteHeader(404)
	if sr.status != 404 {
		t.Errorf("status: got %d, want 404", sr.status)
	}
	// second write should be ignored
	sr.WriteHeader(500)
	if sr.status != 404 {
		t.Errorf("second WriteHeader should not change status: got %d", sr.status)
	}
	if sr.Unwrap() != rec {
		t.Error("Unwrap should return underlying ResponseWriter")
	}
}

// nopHandler is a caddyhttp.Handler that does nothing.
type nopHandler struct{}

func (nopHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) error { return nil }

func TestRequestMetricsServeHTTP(t *testing.T) {
	ch := make(chan RequestStatEvent, 1)
	RequestStatsCh = ch
	defer func() { RequestStatsCh = nil }()

	m := &RequestMetrics{}
	req := httptest.NewRequest("GET", "/users/42", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	if err := m.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Domain != "example.com" {
			t.Errorf("Domain: got %q, want %q", ev.Domain, "example.com")
		}
		if ev.StatusCode != 200 {
			t.Errorf("StatusCode: got %d, want 200", ev.StatusCode)
		}
		if ev.Path != "/users/{id}" {
			t.Errorf("Path: got %q, want %q", ev.Path, "/users/{id}")
		}
		if ev.Method != "GET" {
			t.Errorf("Method: got %q, want GET", ev.Method)
		}
	default:
		t.Fatal("no event sent to channel")
	}
}

func TestRequestMetricsNilChannel(t *testing.T) {
	RequestStatsCh = nil
	m := &RequestMetrics{}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	// should not panic
	if err := m.ServeHTTP(w, req, nopHandler{}); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
}

// The recorded status is the one the client gets: the status written by the
// chain, else the status of the error Caddy turns into a response.
func TestRequestMetricsRecordsErrorStatus(t *testing.T) {
	errDial := errors.New("dial tcp 10.0.0.1:80: connect: connection refused")
	cases := []struct {
		name    string
		next    caddyhttp.HandlerFunc
		ctxDone bool // client went away before the chain returned
		want    int
	}{
		{"handler error 502", func(http.ResponseWriter, *http.Request) error {
			return caddyhttp.Error(http.StatusBadGateway, errDial)
		}, false, http.StatusBadGateway},
		{"wrapped handler error", func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("proxy: %w", caddyhttp.Error(http.StatusGatewayTimeout, errDial))
		}, false, http.StatusGatewayTimeout},
		{"plain error", func(http.ResponseWriter, *http.Request) error {
			return errDial
		}, false, http.StatusInternalServerError},
		{"handler error without status", func(http.ResponseWriter, *http.Request) error {
			return caddyhttp.HandlerError{Err: errDial}
		}, false, http.StatusInternalServerError},
		{"handler error 499", func(http.ResponseWriter, *http.Request) error {
			return caddyhttp.Error(499, context.Canceled)
		}, false, 499},
		{"client cancel error", func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("read body: %w", context.Canceled)
		}, false, 499},
		{"error after client left", func(http.ResponseWriter, *http.Request) error {
			return errDial
		}, true, 499},
		{"client cancel written as 499", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(499) // what reverse_proxy does on context.Canceled
			return nil
		}, false, 499},
		{"status written before error", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return caddyhttp.Error(http.StatusBadGateway, errDial)
		}, false, http.StatusOK},
		{"body written before error", func(w http.ResponseWriter, _ *http.Request) error {
			_, _ = w.Write([]byte("partial"))
			return errDial
		}, false, http.StatusOK},
		{"early hints then error", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusEarlyHints)
			return caddyhttp.Error(http.StatusBadGateway, errDial)
		}, false, http.StatusBadGateway},
		{"100 continue then status", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusContinue)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return nil
		}, false, http.StatusRequestEntityTooLarge},
		{"early hints only", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusEarlyHints)
			return nil // net/http then sends an implicit 200
		}, false, http.StatusOK},
		{"websocket upgrade", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusSwitchingProtocols)
			return nil
		}, false, http.StatusSwitchingProtocols},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan RequestStatEvent, 1)
			RequestStatsCh = ch
			defer func() { RequestStatsCh = nil }()

			req := httptest.NewRequest("GET", "/", nil)
			if tc.ctxDone {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			var nextErr error
			next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
				nextErr = tc.next(w, r)
				return nextErr
			})
			if err := (&RequestMetrics{}).ServeHTTP(httptest.NewRecorder(), req, next); err != nextErr {
				t.Errorf("returned error %v, want the chain's error %v unchanged", err, nextErr)
			}
			ev := <-ch
			if ev.StatusCode != tc.want {
				t.Errorf("StatusCode = %d, want %d", ev.StatusCode, tc.want)
			}
		})
	}
}

// Ensure nopHandler satisfies caddyhttp.Handler at compile time.
var _ caddyhttp.Handler = nopHandler{}
