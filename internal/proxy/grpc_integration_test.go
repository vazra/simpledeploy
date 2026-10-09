package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// caddyDataDir returns a temp dir for Caddy storage. Caddy's background
// storage cleaning can still write lock files right after Stop, which makes
// t.TempDir's strict RemoveAll fail, so cleanup here is best effort.
func caddyDataDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sd-caddy-test-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.RemoveAll(dir)
	})
	return dir
}

func TestCaddyProxyGRPCPathAndCatchAll(t *testing.T) {
	// Upstream 1: native gRPC server (h2c, prior knowledge) with health service.
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

	// Upstream 2: catch-all HTTP/1.1 server (stands in for gRPC-Web + REST).
	catchAll := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "catchall %s", r.Header.Get("Content-Type"))
	}))
	t.Cleanup(catchAll.Close)

	// Upstream 3: /ws* path route. Plain GET answers "ws-plain"; an Upgrade
	// request switches protocols and echoes raw bytes.
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			fmt.Fprint(w, "ws-plain")
			return
		}
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		_, _ = io.Copy(conn, brw.Reader)
	}))
	t.Cleanup(wsSrv.Close)

	addr := freeAddr(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: addr, TLSMode: "off", DataDir: caddyDataDir(t)})
	// Catch-all first on purpose: buildConfig must reorder grpc and path routes ahead of it.
	routes := []Route{
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(catchAll.URL, "http://"), TLS: "off", Protocol: "http"},
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(wsSrv.URL, "http://"), TLS: "off", Protocol: "http", Path: "/ws*"},
		{AppSlug: "demo", Domain: "demo.test", Upstream: gl.Addr().String(), TLS: "off", Protocol: "grpc"},
	}
	if err := p.SetRoutes(routes); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// gRPC unary + server streaming through the proxy (h2c listener -> h2c upstream).
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("demo.test"),
	)
	if err != nil {
		t.Fatalf("grpc client: %v", err)
	}
	defer conn.Close()
	hc := healthpb.NewHealthClient(conn)

	resp, err := hc.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check via proxy: %v", err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("Check status = %v, want SERVING", resp.GetStatus())
	}

	stream, err := hc.Watch(ctx, &healthpb.HealthCheckRequest{Service: "svc"})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("Watch first = %v, %v; want SERVING", first, err)
	}
	hs.SetServingStatus("svc", healthpb.HealthCheckResponse_NOT_SERVING)
	second, err := stream.Recv()
	if err != nil || second.GetStatus() != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("Watch second = %v, %v; want NOT_SERVING (stream not flushed?)", second, err)
	}

	doHTTP := func(method, path, contentType string) string {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, method, "http://"+addr+path, strings.NewReader(""))
		req.Host = "demo.test"
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}

	// gRPC-Web must NOT hit the native gRPC route.
	if got := doHTTP(http.MethodPost, "/grpc.health.v1.Health/Check", "application/grpc-web+proto"); got != "catchall application/grpc-web+proto" {
		t.Errorf("grpc-web body = %q, want catch-all", got)
	}
	if got := doHTTP(http.MethodGet, "/", ""); got != "catchall " {
		t.Errorf("GET / body = %q, want %q", got, "catchall ")
	}
	if got := doHTTP(http.MethodGet, "/ws/info", ""); got != "ws-plain" {
		t.Errorf("GET /ws/info body = %q, want ws-plain", got)
	}

	// Websocket-style upgrade through the path route, then raw echo.
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprint(c, "GET /ws HTTP/1.1\r\nHost: demo.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	br := bufio.NewReader(c)
	up, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read upgrade response: %v", err)
	}
	if up.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d, want 101", up.StatusCode)
	}
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(br, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v; want ping", buf, err)
	}
}

func TestCaddyProxyExtraListenAddrServesSameRoutes(t *testing.T) {
	gl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grpc listen: %v", err)
	}
	gs := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	go func() { _ = gs.Serve(gl) }()
	t.Cleanup(gs.Stop)

	catchAll := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "catchall")
	}))
	t.Cleanup(catchAll.Close)

	mainAddr, extraAddr := freeAddr(t), freeAddr(t)
	p := NewCaddyProxy(CaddyConfig{ListenAddr: mainAddr, ExtraListenAddrs: []string{extraAddr}, TLSMode: "off", DataDir: caddyDataDir(t)})
	routes := []Route{
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(catchAll.URL, "http://"), TLS: "off", Protocol: "http"},
		{AppSlug: "demo", Domain: "demo.test", Upstream: gl.Addr().String(), TLS: "off", Protocol: "grpc"},
	}
	if err := p.SetRoutes(routes); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, addr := range []string{mainAddr, extraAddr} {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithAuthority("demo.test"),
		)
		if err != nil {
			t.Fatalf("grpc client %s: %v", addr, err)
		}
		resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		conn.Close()
		if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			t.Fatalf("Check via %s = %v, %v; want SERVING", addr, resp, err)
		}

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/", nil)
		req.Host = "demo.test"
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET via %s: %v", addr, err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(b) != "catchall" {
			t.Errorf("GET via %s body = %q, want catchall", addr, b)
		}
	}
}

