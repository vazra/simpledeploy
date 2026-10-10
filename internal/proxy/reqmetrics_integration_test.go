package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Real Caddy: a request whose upstream cannot be reached is recorded with the
// 502 the client got, not as a 200.
func TestCaddyProxyMetricsRecordUpstreamFailure(t *testing.T) {
	ch := make(chan RequestStatEvent, 16)
	RequestStatsCh = ch

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(up.Close)
	dead := freeAddr(t) // nothing listens here

	addr := freeAddr(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: addr, TLSMode: "off", DataDir: caddyDataDir(t)})
	if err := p.SetRoutes([]Route{
		{AppSlug: "down", Domain: "down.test", Upstream: dead, TLS: "off"},
		{AppSlug: "busy", Domain: "busy.test", Upstream: strings.TrimPrefix(up.URL, "http://"), TLS: "off"},
	}); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}
	// Stop Caddy before clearing the channel so no handler still reads it.
	t.Cleanup(func() {
		_ = p.Stop()
		RequestStatsCh = nil
	})

	client := &http.Client{Timeout: 5 * time.Second}
	for _, tc := range []struct {
		host string
		want int
	}{
		{"down.test", http.StatusBadGateway},
		{"busy.test", http.StatusServiceUnavailable},
	} {
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/x", nil)
		req.Host = tc.host
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.host, err)
		}
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Fatalf("GET %s: client got %d, want %d", tc.host, res.StatusCode, tc.want)
		}
		select {
		case ev := <-ch:
			if ev.Domain != tc.host || ev.StatusCode != tc.want {
				t.Errorf("recorded %s %d, want %s %d", ev.Domain, ev.StatusCode, tc.host, tc.want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("GET %s: no request stat recorded", tc.host)
		}
	}
}
