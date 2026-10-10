package backup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestS3TargetImplementsInterface(t *testing.T) {
	var _ Target = (*S3Target)(nil)
}

func TestS3Target_Type(t *testing.T) {
	target := &S3Target{}
	if target.Type() != "s3" {
		t.Errorf("Type() = %q, want %q", target.Type(), "s3")
	}
}

func TestS3TargetNewClient(t *testing.T) {
	cfg := S3Config{
		Endpoint:  "http://localhost:9000",
		Bucket:    "backups",
		Prefix:    "simpledeploy",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Region:    "us-east-1",
	}

	target, err := NewS3Target(cfg)
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	if target.client == nil {
		t.Error("expected non-nil s3 client")
	}
	if target.cfg.Bucket != "backups" {
		t.Errorf("bucket mismatch: %s", target.cfg.Bucket)
	}
}

func TestS3TargetDefaultRegion(t *testing.T) {
	cfg := S3Config{
		Bucket:    "backups",
		AccessKey: "key",
		SecretKey: "secret",
	}

	target, err := NewS3Target(cfg)
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	if target.cfg.Region != "us-east-1" {
		t.Errorf("expected default region us-east-1, got %s", target.cfg.Region)
	}
}

func TestS3TargetKey(t *testing.T) {
	tests := []struct {
		prefix   string
		filename string
		want     string
	}{
		{"", "backup.gz", "backup.gz"},
		{"myapp", "backup.gz", "myapp/backup.gz"},
		{"a/b", "f.tar.gz", "a/b/f.tar.gz"},
	}
	for _, tc := range tests {
		t.Run(tc.prefix+"/"+tc.filename, func(t *testing.T) {
			target := &S3Target{cfg: S3Config{Prefix: tc.prefix}}
			got := target.key(tc.filename)
			if got != tc.want {
				t.Errorf("key(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

// stubS3DNS replaces the resolver used for S3 endpoint checks.
func stubS3DNS(t *testing.T, answers map[string][]string) {
	t.Helper()
	old := s3LookupIPAddr
	s3LookupIPAddr = func(_ context.Context, host string) ([]net.IPAddr, error) {
		ips, ok := answers[host]
		if !ok {
			return nil, fmt.Errorf("no such host %q", host)
		}
		var out []net.IPAddr
		for _, ip := range ips {
			out = append(out, net.IPAddr{IP: net.ParseIP(ip)})
		}
		return out, nil
	}
	t.Cleanup(func() { s3LookupIPAddr = old })
}

func TestValidateS3Endpoint_Blocked(t *testing.T) {
	t.Setenv(AllowPrivateS3Env, "")
	stubS3DNS(t, map[string][]string{
		"internal.example": {"10.1.2.3"},
		"mixed.example":    {"93.184.216.34", "192.168.1.5"},
		"localhost":        {"127.0.0.1", "::1"},
	})
	blocked := []string{
		"http://127.0.0.1:9000",
		"http://localhost:9000",
		"https://10.0.0.5",
		"https://172.16.3.4",
		"https://192.168.0.10",
		"http://169.254.169.254",
		"http://100.64.1.1",
		"http://0.0.0.0:9000",
		"http://224.0.0.1",
		"http://[::1]:9000",
		"http://[fd00::1]",
		"http://[fe80::1]",
		"http://[::ffff:127.0.0.1]",
		"http://[64:ff9b::a00:1]",     // NAT64 of 10.0.0.1
		"http://[64:ff9b:1::808:808]", // NAT64 local-use
		"https://internal.example",
		"https://mixed.example",
	}
	for _, ep := range blocked {
		err := ValidateS3Endpoint(context.Background(), ep)
		if !errors.Is(err, ErrS3EndpointBlocked) {
			t.Errorf("ValidateS3Endpoint(%q) = %v, want ErrS3EndpointBlocked", ep, err)
			continue
		}
		if !strings.Contains(err.Error(), AllowPrivateS3Env) {
			t.Errorf("error for %q should mention %s: %v", ep, AllowPrivateS3Env, err)
		}
	}
}

func TestValidateS3Endpoint_Allowed(t *testing.T) {
	t.Setenv(AllowPrivateS3Env, "")
	stubS3DNS(t, map[string][]string{"s3.example": {"93.184.216.34", "2606:2800:220:1::1"}})
	for _, ep := range []string{"", "https://s3.example", "https://s3.example:9000/", "https://8.8.8.8", "https://[2606:4700:4700::1111]", "https://[64:ff9b::808:808]"} {
		if err := ValidateS3Endpoint(context.Background(), ep); err != nil {
			t.Errorf("ValidateS3Endpoint(%q) = %v, want nil", ep, err)
		}
	}
}

func TestValidateS3Endpoint_InvalidURL(t *testing.T) {
	for _, ep := range []string{"minio:9000", "ftp://s3.example", "https://", "not a url"} {
		err := ValidateS3Endpoint(context.Background(), ep)
		if err == nil || errors.Is(err, ErrS3EndpointBlocked) {
			t.Errorf("ValidateS3Endpoint(%q) = %v, want invalid URL error", ep, err)
		}
	}
}

func TestValidateS3Endpoint_OptIn(t *testing.T) {
	t.Setenv(AllowPrivateS3Env, "1")
	for _, ep := range []string{"http://127.0.0.1:9000", "http://10.0.0.5", "http://[::1]:9000"} {
		if err := ValidateS3Endpoint(context.Background(), ep); err != nil {
			t.Errorf("with opt-in, ValidateS3Endpoint(%q) = %v, want nil", ep, err)
		}
	}
}

func TestS3DialControl(t *testing.T) {
	blocked := []string{
		"127.0.0.1:443",
		"10.0.0.5:9000",
		"169.254.169.254:80",
		"100.64.1.1:443",
		"[::1]:443",
		"[::ffff:127.0.0.1]:443",
		"[fd00::1]:443",
		"[fe80::1%en0]:443",
		"[64:ff9b::7f00:1]:443", // NAT64 of 127.0.0.1
		"[64:ff9b:1::1]:443",
		"not-an-ip:443",
	}
	for _, addr := range blocked {
		err := s3DialControl("tcp", addr, nil)
		if !errors.Is(err, ErrS3EndpointBlocked) {
			t.Errorf("s3DialControl(%q) = %v, want ErrS3EndpointBlocked", addr, err)
			continue
		}
		if !strings.Contains(err.Error(), AllowPrivateS3Env) {
			t.Errorf("error for %q should mention %s: %v", addr, AllowPrivateS3Env, err)
		}
	}
	allowed := []string{
		"93.184.216.34:443",
		"8.8.8.8:80",
		"[2606:4700:4700::1111]:443",
		"[64:ff9b::808:808]:443", // NAT64 of 8.8.8.8
	}
	for _, addr := range allowed {
		if err := s3DialControl("tcp", addr, nil); err != nil {
			t.Errorf("s3DialControl(%q) = %v, want nil", addr, err)
		}
	}
}

// loopbackListener accepts connections on 127.0.0.1 and counts them.
func loopbackListener(t *testing.T) (port string, accepted *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted = &atomic.Int32{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			c.Close()
		}
	}()
	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, accepted
}

func TestS3SafeDial_BlocksRebinding(t *testing.T) {
	// A name that passed validation but now resolves to loopback is refused
	// at connect time: Control sees the resolved IP, not the name.
	port, accepted := loopbackListener(t)
	dial := s3SafeDialContext(nil)
	for _, addr := range []string{"localhost:" + port, "127.0.0.1:" + port} {
		conn, err := dial(context.Background(), "tcp", addr)
		if err == nil {
			conn.Close()
		}
		if !errors.Is(err, ErrS3EndpointBlocked) {
			t.Errorf("dial %s = %v, want ErrS3EndpointBlocked", addr, err)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if n := accepted.Load(); n != 0 {
		t.Fatalf("blocked dial reached the listener %d times", n)
	}
}

func TestS3SafeDial_AllowedAddressConnects(t *testing.T) {
	// Treat 127.0.0.1 as public and ::1 as blocked: the guarded dialer must
	// connect normally, falling back past the blocked address.
	old := s3DialBlocked
	s3DialBlocked = func(ip net.IP) bool { return ip == nil || ip.Equal(net.IPv6loopback) }
	t.Cleanup(func() { s3DialBlocked = old })

	port, _ := loopbackListener(t)
	dial := s3SafeDialContext(nil)
	for _, addr := range []string{"127.0.0.1:" + port, "localhost:" + port} {
		conn, err := dial(context.Background(), "tcp", addr)
		if err != nil {
			t.Errorf("dial %s = %v, want success", addr, err)
			continue
		}
		conn.Close()
	}
}

func TestRestrictS3Transport_Proxy(t *testing.T) {
	// The proxy listens on loopback: the connection to it must not be
	// IP-checked, while a direct (unproxied) connection still is.
	var proxied atomic.Int32
	var proxiedHost atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		proxiedHost.Store(r.Host)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, _ := url.Parse(proxy.URL)
	direct, directHits := fakeS3(t)

	tr := &http.Transport{}
	restrictS3Transport(tr, func(r *http.Request) (*url.URL, error) {
		if r.URL.Hostname() == "s3.example.test" {
			return proxyURL, nil
		}
		return nil, nil
	})
	t.Cleanup(tr.CloseIdleConnections)
	c := &http.Client{Transport: tr}

	resp, err := c.Get("http://s3.example.test:9000/bucket")
	if err != nil {
		t.Fatalf("proxied GET: %v", err)
	}
	resp.Body.Close()
	if proxied.Load() != 1 || proxiedHost.Load() != "s3.example.test:9000" {
		t.Fatalf("proxy hits=%d host=%v, want 1 request for s3.example.test:9000", proxied.Load(), proxiedHost.Load())
	}

	if _, err := c.Get(direct.URL); !errors.Is(err, ErrS3EndpointBlocked) {
		t.Fatalf("direct GET = %v, want ErrS3EndpointBlocked", err)
	}
	if directHits.Load() != 0 {
		t.Fatalf("blocked direct endpoint received %d requests", directHits.Load())
	}
}

func TestProxyDialAddr(t *testing.T) {
	cases := map[string]string{
		"http://proxy.local":        "proxy.local:80",
		"https://proxy.local":       "proxy.local:443",
		"socks5://proxy.local":      "proxy.local:1080",
		"http://proxy.local:3128":   "proxy.local:3128",
		"http://user:pw@[::1]:8080": "[::1]:8080",
	}
	for raw, want := range cases {
		u, _ := url.Parse(raw)
		if got := proxyDialAddr(u); got != want {
			t.Errorf("proxyDialAddr(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestS3Target_ProxyEnvHonoured re-runs itself in a child process because
// http.ProxyFromEnvironment reads the proxy variables once per process.
func TestS3Target_ProxyEnvHonoured(t *testing.T) {
	if os.Getenv("SD_S3_PROXY_CHILD") == "1" {
		s3ProxyEnvChild(t)
		return
	}
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "s3.example.test:9000" {
			proxied.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	direct, directHits := fakeS3(t)

	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "REQUEST_METHOD", AllowPrivateS3Env:
			continue
		}
		env = append(env, kv)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestS3Target_ProxyEnvHonoured$", "-test.count=1")
	cmd.Env = append(env,
		"SD_S3_PROXY_CHILD=1",
		"HTTP_PROXY="+proxy.URL,
		"SD_S3_DIRECT_URL="+strings.Replace(direct.URL, "127.0.0.1", "localhost", 1),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if proxied.Load() == 0 {
		t.Fatal("request did not go through HTTP_PROXY")
	}
	if directHits.Load() != 0 {
		t.Fatalf("unproxied private endpoint received %d requests", directHits.Load())
	}
}

func s3ProxyEnvChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stubS3DNS(t, map[string][]string{
		"s3.example.test":       {"93.184.216.34"},
		"internal.example.test": {"10.1.2.3"},
	})

	// Proxied endpoints are checked when the target is built.
	if _, err := NewS3Target(S3Config{Endpoint: "http://internal.example.test:9000", Bucket: "b"}); !errors.Is(err, ErrS3EndpointBlocked) {
		t.Fatalf("NewS3Target(private endpoint via proxy) = %v, want ErrS3EndpointBlocked", err)
	}

	// Proxied: the loopback proxy is reachable although it is private.
	target, err := NewS3Target(S3Config{Endpoint: "http://s3.example.test:9000", Bucket: "b", AccessKey: "k", SecretKey: "s"})
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	if err := target.Test(ctx); err != nil {
		t.Fatalf("Test() via HTTP_PROXY = %v, want nil", err)
	}

	// localhost is never proxied, so the IP check still applies.
	target, err = NewS3Target(S3Config{Endpoint: os.Getenv("SD_S3_DIRECT_URL"), Bucket: "b", AccessKey: "k", SecretKey: "s"})
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	if err := target.Test(ctx); !errors.Is(err, ErrS3EndpointBlocked) {
		t.Fatalf("Test() direct = %v, want ErrS3EndpointBlocked", err)
	}
}

// fakeS3 answers every request with 200 and counts hits.
func fakeS3(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestS3Target_PrivateEndpointBlockedAtConnect(t *testing.T) {
	t.Setenv(AllowPrivateS3Env, "")
	srv, hits := fakeS3(t)
	target, err := NewS3Target(S3Config{Endpoint: srv.URL, Bucket: "b", AccessKey: "k", SecretKey: "s"})
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	start := time.Now()
	err = target.Test(context.Background())
	if !errors.Is(err, ErrS3EndpointBlocked) {
		t.Fatalf("Test() = %v, want ErrS3EndpointBlocked", err)
	}
	if !strings.Contains(err.Error(), AllowPrivateS3Env) {
		t.Errorf("error should mention %s: %v", AllowPrivateS3Env, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("blocked endpoint received %d requests", hits.Load())
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("blocked dial was retried (took %s)", time.Since(start))
	}
}

func TestS3Target_PrivateEndpointOptIn(t *testing.T) {
	t.Setenv(AllowPrivateS3Env, "1")
	srv, hits := fakeS3(t)
	target, err := NewS3Target(S3Config{Endpoint: srv.URL, Bucket: "b", AccessKey: "k", SecretKey: "s"})
	if err != nil {
		t.Fatalf("NewS3Target: %v", err)
	}
	if err := target.Test(context.Background()); err != nil {
		t.Fatalf("Test() with opt-in = %v, want nil", err)
	}
	if hits.Load() == 0 {
		t.Fatal("expected request to reach the local endpoint")
	}
}

// stubS3Proxy makes every S3 request use proxy ("" for none).
func stubS3Proxy(t *testing.T, proxy string) {
	t.Helper()
	old := s3EnvProxy
	s3EnvProxy = func(*http.Request) (*url.URL, error) {
		if proxy == "" {
			return nil, nil
		}
		return url.Parse(proxy)
	}
	t.Cleanup(func() { s3EnvProxy = old })
}

func TestNewS3Target_ProxiedEndpointValidated(t *testing.T) {
	t.Setenv(AllowPrivateS3Env, "")
	stubS3Proxy(t, "http://proxy.example:3128")
	stubS3DNS(t, map[string][]string{
		"internal.example": {"10.1.2.3"},
		"public.example":   {"93.184.216.34"},
	})

	if _, err := NewS3Target(S3Config{Endpoint: "https://internal.example", Bucket: "b"}); !errors.Is(err, ErrS3EndpointBlocked) {
		t.Fatalf("private endpoint via proxy: err = %v, want ErrS3EndpointBlocked", err)
	}
	if _, err := NewS3Target(S3Config{Endpoint: "https://unknown.example", Bucket: "b"}); err == nil {
		t.Fatal("unresolvable endpoint via proxy: want error")
	}
	if _, err := NewS3Target(S3Config{Endpoint: "https://public.example", Bucket: "b"}); err != nil {
		t.Fatalf("public endpoint via proxy: %v", err)
	}
	// AWS default endpoint needs no check.
	if _, err := NewS3Target(S3Config{Bucket: "b"}); err != nil {
		t.Fatalf("default endpoint: %v", err)
	}

	t.Setenv(AllowPrivateS3Env, "1")
	if _, err := NewS3Target(S3Config{Endpoint: "https://internal.example", Bucket: "b"}); err != nil {
		t.Fatalf("private endpoint with opt-in: %v", err)
	}
}

func TestNewS3Target_DirectEndpointCheckedAtDial(t *testing.T) {
	t.Setenv(AllowPrivateS3Env, "")
	stubS3Proxy(t, "")
	stubS3DNS(t, map[string][]string{})
	// Without a proxy the dialer checks each connection, so building the
	// target does no DNS lookup and cannot fail on it.
	if _, err := NewS3Target(S3Config{Endpoint: "https://unknown.example", Bucket: "b"}); err != nil {
		t.Fatalf("direct endpoint: %v", err)
	}
}

func TestS3EndpointProxied(t *testing.T) {
	stubS3Proxy(t, "http://proxy.example:3128")
	if !s3EndpointProxied("https://s3.example.com") {
		t.Error("want proxied")
	}
	if s3EndpointProxied("://bad") {
		t.Error("unparsable endpoint: want not proxied")
	}
	stubS3Proxy(t, "")
	if s3EndpointProxied("https://s3.example.com") {
		t.Error("no proxy: want not proxied")
	}
	old := s3EnvProxy
	s3EnvProxy = func(*http.Request) (*url.URL, error) { return nil, errors.New("bad proxy") }
	defer func() { s3EnvProxy = old }()
	if !s3EndpointProxied("https://s3.example.com") {
		t.Error("proxy error: want treated as proxied")
	}
}
