package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

// Large request headers reach the app, a stalled upload to an HTTP route is
// cut after the body idle timeout, and quiet gRPC bidi streams and
// WebSockets are not.
func TestCaddyProxyRequestLimits(t *testing.T) {
	old := requestBodyIdleTimeout
	requestBodyIdleTimeout = 300 * time.Millisecond
	t.Cleanup(func() { requestBodyIdleTimeout = old })

	gl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grpc listen: %v", err)
	}
	gs := grpc.NewServer()
	reflection.Register(gs) // ServerReflectionInfo is a bidi stream
	go func() { _ = gs.Serve(gl) }()
	t.Cleanup(gs.Stop)

	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, "cookie=%d body=%d", len(r.Header.Get("Cookie")), len(body))
	}))
	t.Cleanup(web.Close)

	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if err := p.SetRoutes([]Route{
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(web.URL, "http://"), TLS: "off", Protocol: "http"},
		{AppSlug: "demo", Domain: "demo.test", Upstream: strings.TrimPrefix(wsSrv.URL, "http://"), TLS: "off", Protocol: "http", Path: "/ws*"},
		{AppSlug: "demo", Domain: "demo.test", Upstream: gl.Addr().String(), TLS: "off", Protocol: "grpc"},
	}); err != nil {
		t.Fatalf("SetRoutes: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Run("large header", func(t *testing.T) {
		cookie := "a=" + strings.Repeat("x", 64<<10)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/", nil)
		req.Host = "demo.test"
		req.Header.Set("Cookie", cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK || string(b) != fmt.Sprintf("cookie=%d body=0", len(cookie)) {
			t.Fatalf("status %d body %q, want 200 with the full cookie", res.StatusCode, b)
		}
	})

	t.Run("stalled upload is cut", func(t *testing.T) {
		c, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		fmt.Fprint(c, "POST /upload HTTP/1.1\r\nHost: demo.test\r\nContent-Length: 10\r\n\r\nhello")
		time.Sleep(4 * requestBodyIdleTimeout)
		_, _ = c.Write([]byte("world"))
		res, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err == nil {
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				t.Fatalf("stalled upload succeeded: %q", b)
			}
		}
	})

	t.Run("quiet grpc stream survives", func(t *testing.T) {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithAuthority("demo.test"),
		)
		if err != nil {
			t.Fatalf("grpc client: %v", err)
		}
		defer conn.Close()
		stream, err := rpb.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
		if err != nil {
			t.Fatalf("ServerReflectionInfo: %v", err)
		}
		list := &rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_ListServices{}}
		for i := 0; i < 2; i++ {
			if i > 0 {
				time.Sleep(4 * requestBodyIdleTimeout)
			}
			if err := stream.Send(list); err != nil {
				t.Fatalf("send %d: %v", i, err)
			}
			if _, err := stream.Recv(); err != nil {
				t.Fatalf("recv %d: %v", i, err)
			}
		}
		_ = stream.CloseSend()
	})

	t.Run("quiet websocket survives", func(t *testing.T) {
		c, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		fmt.Fprint(c, "GET /ws HTTP/1.1\r\nHost: demo.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
		br := bufio.NewReader(c)
		up, err := http.ReadResponse(br, nil)
		if err != nil || up.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("upgrade = %v, %v; want 101", up, err)
		}
		buf := make([]byte, 4)
		for i := 0; i < 2; i++ {
			if i > 0 {
				time.Sleep(4 * requestBodyIdleTimeout)
			}
			if _, err := c.Write([]byte("ping")); err != nil {
				t.Fatalf("write %d: %v", i, err)
			}
			if _, err := io.ReadFull(br, buf); err != nil || string(buf) != "ping" {
				t.Fatalf("echo %d = %q, %v; want ping", i, buf, err)
			}
		}
	})
}
