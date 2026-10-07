package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// loadWithoutTrustInstall loads the proxy's generated config into Caddy
// with the internal CA's install_trust disabled, so the test never touches
// the system trust store. Everything else is the config buildConfig emits.
func loadWithoutTrustInstall(t *testing.T, p *CaddyProxy, routes []Route) {
	t.Helper()
	p.mu.Lock()
	p.routes = routes
	p.mu.Unlock()
	raw, err := p.BuildConfigJSON()
	if err != nil {
		t.Fatalf("BuildConfigJSON: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg["apps"].(map[string]interface{})["pki"] = map[string]interface{}{
		"certificate_authorities": map[string]interface{}{
			"local": map[string]interface{}{"install_trust": false},
		},
	}
	data, _ := json.Marshal(cfg)
	if err := caddy.Load(data, true); err != nil {
		t.Fatalf("caddy.Load: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })
}

// waitForLocalRoot waits for Caddy's internal CA root and returns a pool
// containing it.
func waitForLocalRoot(t *testing.T, dataDir string) *x509.CertPool {
	t.Helper()
	path := filepath.Join(dataDir, "caddy", "pki", "authorities", "local", "root.crt")
	deadline := time.Now().Add(15 * time.Second)
	for {
		pemBytes, err := os.ReadFile(path)
		if err == nil {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pemBytes) {
				return pool
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("local CA root not found at %s: %v", path, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestCaddyProxyGRPCOverTLSLocalCA(t *testing.T) {
	gl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grpc listen: %v", err)
	}
	gs := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("svc", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(gs, hs)
	go func() { _ = gs.Serve(gl) }()
	t.Cleanup(gs.Stop)

	catchAll := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "catchall")
	}))
	t.Cleanup(catchAll.Close)

	addr := freeAddr(t)
	dataDir := caddyDataDir(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: addr, TLSMode: "local", DataDir: dataDir})
	loadWithoutTrustInstall(t, p, []Route{
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(catchAll.URL, "http://"), TLS: "local", Protocol: "http"},
		{AppSlug: "demo", Domain: "demo.test", Upstream: gl.Addr().String(), TLS: "local", Protocol: "grpc"},
	})
	pool := waitForLocalRoot(t, dataDir)
	tlsCfg := &tls.Config{RootCAs: pool, ServerName: "demo.test", MinVersion: tls.VersionTLS12}

	// The TLS listener must negotiate HTTP/2 via ALPN (gRPC requires it).
	// Retry while Caddy issues the leaf cert for demo.test.
	deadline := time.Now().Add(15 * time.Second)
	for {
		c := tlsCfg.Clone()
		c.NextProtos = []string{"h2", "http/1.1"}
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, c)
		if err == nil {
			proto := conn.ConnectionState().NegotiatedProtocol
			conn.Close()
			if proto != "h2" {
				t.Fatalf("ALPN = %q, want h2", proto)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("TLS handshake never succeeded: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithAuthority("demo.test"),
	)
	if err != nil {
		t.Fatalf("grpc client: %v", err)
	}
	defer conn.Close()
	hc := healthpb.NewHealthClient(conn)

	resp, err := hc.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("Check over TLS = %v, %v; want SERVING", resp, err)
	}
	stream, err := hc.Watch(ctx, &healthpb.HealthCheckRequest{Service: "svc"})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if first, err := stream.Recv(); err != nil || first.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("Watch first = %v, %v; want SERVING", first, err)
	}
	hs.SetServingStatus("svc", healthpb.HealthCheckResponse_NOT_SERVING)
	if second, err := stream.Recv(); err != nil || second.GetStatus() != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("Watch second = %v, %v; want NOT_SERVING", second, err)
	}

	// Plain HTTPS on the same host still reaches the catch-all.
	hc2 := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg, ForceAttemptHTTP2: true}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/", nil)
	req.Host = "demo.test"
	res, err := hc2.Do(req)
	if err != nil {
		t.Fatalf("HTTPS GET: %v", err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "catchall" {
		t.Errorf("HTTPS GET body = %q, want catchall", b)
	}
}

func TestCaddyProxyGRPCWebStreamingFlushesThroughCatchAll(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Chunked gRPC-Web style response: no Content-Length, first frame
		// flushed, then the handler blocks until the client saw it.
		w.Header().Set("Content-Type", "application/grpc-web+proto")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "frame1")
		http.NewResponseController(w).Flush()
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
		fmt.Fprint(w, "frame2")
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	grpcStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "wrong: native grpc route")
	}))
	t.Cleanup(grpcStub.Close)

	addr := freeAddr(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: addr, TLSMode: "off", DataDir: caddyDataDir(t)})
	if err := p.SetRoutes([]Route{
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(upstream.URL, "http://"), TLS: "off", Protocol: "http"},
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(grpcStub.URL, "http://"), TLS: "off", Protocol: "grpc"},
	}); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/pkg.Svc/Stream", strings.NewReader(""))
	req.Host = "demo.test"
	req.Header.Set("Content-Type", "application/grpc-web+proto")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()

	first := make(chan string, 1)
	go func() {
		buf := make([]byte, len("frame1"))
		_, _ = io.ReadFull(res.Body, buf)
		first <- string(buf)
	}()
	select {
	case got := <-first:
		if got != "frame1" {
			t.Fatalf("first frame = %q, want frame1", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first frame not received before the response ended (proxy buffered the stream)")
	}
	close(release)
	rest, _ := io.ReadAll(res.Body)
	if string(rest) != "frame2" {
		t.Fatalf("rest = %q, want frame2", rest)
	}
}
