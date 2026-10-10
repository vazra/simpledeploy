package proxy

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// securityHeadersUpstream serves /own (the app sets every security header
// itself), /partial (only X-Frame-Options) and anything else (no headers).
func securityHeadersUpstream(t *testing.T) string {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		switch r.URL.Path {
		case "/own":
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Strict-Transport-Security", "max-age=60")
		case "/partial":
			h.Set("X-Frame-Options", "DENY")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(up.Close)
	return strings.TrimPrefix(up.URL, "http://")
}

// wantHeader fails unless res carries exactly one value for name equal to
// want ("" means the header must be absent).
func wantHeader(t *testing.T, res *http.Response, name, want string) {
	t.Helper()
	got := res.Header.Values(name)
	switch {
	case want == "" && len(got) != 0:
		t.Errorf("%s = %q, want absent", name, got)
	case want != "" && (len(got) != 1 || got[0] != want):
		t.Errorf("%s = %q, want [%q]", name, got, want)
	}
}

// Real Caddy: SimpleDeploy adds each default security header only when the
// app's response does not already set it.
func TestCaddyProxySecurityHeadersKeepAppValues(t *testing.T) {
	addr := freeAddr(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: addr, TLSMode: "off", DataDir: caddyDataDir(t)})
	if err := p.SetRoutes([]Route{
		{AppSlug: "demo", Domain: "demo.test", Upstream: securityHeadersUpstream(t), TLS: "off"},
	}); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	client := &http.Client{Timeout: 5 * time.Second}
	get := func(path string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+path, nil)
		req.Host = "demo.test"
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, res.StatusCode)
		}
		return res
	}

	t.Run("app values kept", func(t *testing.T) {
		res := get("/own")
		wantHeader(t, res, "X-Content-Type-Options", "nosniff")
		wantHeader(t, res, "X-Frame-Options", "DENY")
		wantHeader(t, res, "Referrer-Policy", "no-referrer")
		// Plain-HTTP route: SimpleDeploy adds no HSTS, but the app's own is
		// passed through untouched.
		wantHeader(t, res, "Strict-Transport-Security", "max-age=60")
	})
	t.Run("missing ones added", func(t *testing.T) {
		res := get("/partial")
		wantHeader(t, res, "X-Frame-Options", "DENY")
		wantHeader(t, res, "X-Content-Type-Options", "nosniff")
		wantHeader(t, res, "Referrer-Policy", "strict-origin-when-cross-origin")
	})
	t.Run("defaults without HSTS on plain HTTP", func(t *testing.T) {
		res := get("/none")
		wantHeader(t, res, "X-Content-Type-Options", "nosniff")
		wantHeader(t, res, "X-Frame-Options", "SAMEORIGIN")
		wantHeader(t, res, "Referrer-Policy", "strict-origin-when-cross-origin")
		wantHeader(t, res, "Strict-Transport-Security", "")
	})
}

// Real Caddy over TLS: HSTS is added on TLS routes unless the app sends its
// own, which is kept.
func TestCaddyProxyHSTSKeepsAppValue(t *testing.T) {
	addr := freeAddr(t)
	dataDir := caddyDataDir(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: addr, TLSMode: "local", DataDir: dataDir})
	loadWithoutTrustInstall(t, p, []Route{
		{AppSlug: "demo", Domain: "demo.test", Upstream: securityHeadersUpstream(t), TLS: "local"},
	})
	pool := waitForLocalRoot(t, dataDir)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "demo.test", MinVersion: tls.VersionTLS12},
		},
	}
	get := func(path string) *http.Response {
		t.Helper()
		// Retry while Caddy issues the leaf cert for demo.test.
		deadline := time.Now().Add(15 * time.Second)
		for {
			req, _ := http.NewRequest(http.MethodGet, "https://"+addr+path, nil)
			req.Host = "demo.test"
			res, err := client.Do(req)
			if err == nil {
				res.Body.Close()
				if res.StatusCode != http.StatusOK {
					t.Fatalf("GET %s: status %d", path, res.StatusCode)
				}
				return res
			}
			if time.Now().After(deadline) {
				t.Fatalf("GET %s: %v", path, err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	wantHeader(t, get("/own"), "Strict-Transport-Security", "max-age=60")
	wantHeader(t, get("/none"), "Strict-Transport-Security", "max-age=31536000; includeSubDomains")
}
