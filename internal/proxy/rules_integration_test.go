package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCaddyProxyRulesHostVariants runs real Caddy and checks that the IP
// allowlist applies regardless of Host case/port, on wildcard domains, and
// when another app claims the same domain.
func TestCaddyProxyRulesHostVariants(t *testing.T) {
	freshRegistries(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "upstream")
	}))
	t.Cleanup(up.Close)
	upAddr := strings.TrimPrefix(up.URL, "http://")

	addr := freeAddr(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: addr, TLSMode: "off", DataDir: caddyDataDir(t)})
	deny := []string{"10.9.9.9"} // excludes the 127.0.0.1 test client
	routes := []Route{
		{AppSlug: "a", Domain: "Secure.Test", Upstream: upAddr, TLS: "off", AllowedIPs: deny},
		{AppSlug: "z", Domain: "secure.test", Upstream: upAddr, TLS: "off"}, // other app, no rules
		{AppSlug: "w", Domain: "w.*.test", Upstream: upAddr, TLS: "off", AllowedIPs: deny},
		{AppSlug: "o", Domain: "open.test", Upstream: upAddr, TLS: "off"},
	}
	if err := p.SetRoutes(routes); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	client := &http.Client{Timeout: 5 * time.Second}
	get := func(host string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
		req.Host = host
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", host, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	_, port, _ := strings.Cut(addr, ":")
	for _, host := range []string{"secure.test", "SECURE.TEST", "Secure.Test:" + port, "w.foo.test", "W.FOO.test:" + port} {
		if code, body := get(host); code != http.StatusNotFound || body == "upstream" {
			t.Errorf("%s: status %d body %q, want 404 without upstream", host, code, body)
		}
	}
	for _, host := range []string{"open.test", "OPEN.test:" + port} {
		if code, body := get(host); code != http.StatusOK || body != "upstream" {
			t.Errorf("%s: status %d body %q, want 200 upstream", host, code, body)
		}
	}
}
