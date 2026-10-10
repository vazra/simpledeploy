package api

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/gorilla/websocket"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/events"
	"github.com/vazra/simpledeploy/internal/logbuf"
	"github.com/vazra/simpledeploy/internal/store"
)

// setWSRecheck shortens the WebSocket auth recheck interval for a test.
func setWSRecheck(t *testing.T, d time.Duration) {
	t.Helper()
	prev := wsAuthRecheckNanos.Swap(int64(d))
	t.Cleanup(func() { wsAuthRecheckNanos.Store(prev) })
}

// makeRoleUserCookie creates a user with role, grants app access, logs in
// and returns the session cookie and user ID.
func makeRoleUserCookie(t *testing.T, srv *Server, st *store.Store, username, role string, appIDs ...int64) (*http.Cookie, int64) {
	t.Helper()
	hash, err := auth.HashPassword("pass")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(username, hash, role, "", "")
	if err != nil {
		t.Fatalf("create user %q: %v", username, err)
	}
	for _, aid := range appIDs {
		if err := st.GrantAppAccess(u.ID, aid); err != nil {
			t.Fatalf("grant access: %v", err)
		}
	}
	w := postJSON(t, srv, "/api/auth/login", map[string]string{"username": username, "password": "pass"})
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			return c, u.ID
		}
	}
	t.Fatalf("no session cookie for %q (status %d)", username, w.Code)
	return nil, 0
}

// dialWSPath opens a WebSocket on path with a same-origin Origin header and
// the given cookie and extra headers.
func dialWSPath(t *testing.T, ts *httptest.Server, path string, cookie *http.Cookie, extra http.Header) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(ts.URL)
	hdr := http.Header{}
	for k, v := range extra {
		hdr[k] = v
	}
	if cookie != nil {
		hdr.Set("Origin", "http://"+u.Host)
		hdr.Set("Cookie", cookie.Name+"="+cookie.Value)
	}
	conn, resp, err := websocket.DefaultDialer.Dial("ws://"+u.Host+path, hdr)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial %s: %v (status=%d)", path, err, resp.StatusCode)
		}
		t.Fatalf("dial %s: %v", path, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// wsFrames reads a WebSocket in the background. gorilla connections are
// unusable after a read deadline expires, so "nothing arrived" checks must
// not use read deadlines on the conn itself. Frames and the final read
// error share one channel so ordering is preserved.
type wsFrames struct {
	items chan wsItem
}

type wsItem struct {
	data []byte
	err  error
}

func pumpWS(conn *websocket.Conn) *wsFrames {
	f := &wsFrames{items: make(chan wsItem, 64)}
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			f.items <- wsItem{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()
	return f
}

// next returns the next frame decoded as JSON.
func (f *wsFrames) next(t *testing.T) map[string]any {
	t.Helper()
	select {
	case it := <-f.items:
		if it.err != nil {
			t.Fatalf("connection closed: %v", it.err)
		}
		var m map[string]any
		if err := json.Unmarshal(it.data, &m); err != nil {
			t.Fatalf("unmarshal %s: %v", it.data, err)
		}
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for frame")
	}
	return nil
}

// expectNone asserts the socket stays open and silent for d.
func (f *wsFrames) expectNone(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case it := <-f.items:
		if it.err != nil {
			t.Fatalf("connection closed early: %v", it.err)
		}
		t.Fatalf("unexpected frame: %s", it.data)
	case <-time.After(d):
	}
}

// expectPolicyClose waits for the server's policy-violation close frame,
// skipping any data frames before it.
func (f *wsFrames) expectPolicyClose(t *testing.T) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case it := <-f.items:
			if it.err == nil {
				continue
			}
			if !websocket.IsCloseError(it.err, websocket.ClosePolicyViolation) {
				t.Fatalf("expected policy-violation close, got %v", it.err)
			}
			return
		case <-deadline:
			t.Fatal("socket not closed after authorization change")
		}
	}
}

func wsTestFrame(stream byte, payload string) []byte {
	hdr := make([]byte, 8)
	hdr[0] = stream
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
	return append(hdr, payload...)
}

// stubLogsDocker overrides ContainerLogs on the shared mock.
type stubLogsDocker struct {
	*docker.MockClient
	open func(ctx context.Context) (io.ReadCloser, error)
}

func (d *stubLogsDocker) ContainerLogs(ctx context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	return d.open(ctx)
}

func newLogsWSServer(t *testing.T, open func(ctx context.Context) (io.ReadCloser, error)) (*Server, *store.Store, *docker.MockClient, *httptest.Server) {
	t.Helper()
	srv, st := newTestServer(t)
	mock := docker.NewMockClient()
	mock.AddContainer("c1", nil)
	srv.SetDocker(&stubLogsDocker{MockClient: mock, open: open})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, st, mock, ts
}

func TestLogsWSClosesWhenAppAccessRevoked(t *testing.T) {
	setWSRecheck(t, 50*time.Millisecond)
	pr, pw := io.Pipe()
	srv, st, _, ts := newLogsWSServer(t, func(context.Context) (io.ReadCloser, error) { return pr, nil })
	appID := seedApp(t, st, "logsapp")
	cookie, uid := makeRoleUserCookie(t, srv, st, "member", "manage", appID)

	ws := pumpWS(dialWSPath(t, ts, "/api/apps/logsapp/logs", cookie, nil))
	go pw.Write(wsTestFrame(1, "2026-04-08T12:00:00.000000000Z hello\n"))
	m := ws.next(t)
	if m["line"] != "hello" || m["stream"] != "stdout" || m["ts"] != "2026-04-08T12:00:00.000000000Z" {
		t.Fatalf("frame = %v", m)
	}

	// Still authorized: the stream stays open across rechecks.
	ws.expectNone(t, 200*time.Millisecond)

	if err := st.RevokeAppAccess(uid, appID); err != nil {
		t.Fatal(err)
	}
	ws.expectPolicyClose(t)
}

func TestLogsWSClosesOnLogout(t *testing.T) {
	setWSRecheck(t, 50*time.Millisecond)
	pr, _ := io.Pipe()
	srv, st, _, ts := newLogsWSServer(t, func(context.Context) (io.ReadCloser, error) { return pr, nil })
	appID := seedApp(t, st, "logsapp")
	cookie, uid := makeRoleUserCookie(t, srv, st, "member", "manage", appID)

	ws := pumpWS(dialWSPath(t, ts, "/api/apps/logsapp/logs", cookie, nil))
	ws.expectNone(t, 150*time.Millisecond)
	if err := st.BumpTokenVersion(uid); err != nil {
		t.Fatal(err)
	}
	ws.expectPolicyClose(t)
}

func TestLogsWSAPIKeyRevocationCloses(t *testing.T) {
	setWSRecheck(t, 50*time.Millisecond)
	pr, _ := io.Pipe()
	srv, st, _, ts := newLogsWSServer(t, func(context.Context) (io.ReadCloser, error) { return pr, nil })
	srv.SetMasterSecret("test-master-secret-0123456789abcdef")
	appID := seedApp(t, st, "logsapp")
	_, uid := makeRoleUserCookie(t, srv, st, "member", "manage", appID)
	plain, hash, err := auth.GenerateAPIKey("test-master-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	key, err := st.CreateAPIKey(uid, hash, "cli", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Bearer callers may omit Origin.
	ws := pumpWS(dialWSPath(t, ts, "/api/apps/logsapp/logs", nil, http.Header{"Authorization": {"Bearer " + plain}}))
	ws.expectNone(t, 150*time.Millisecond)
	if err := st.DeleteAPIKey(key.ID, uid); err != nil {
		t.Fatal(err)
	}
	ws.expectPolicyClose(t)
}

func TestLogsWSTTYStreamIsRaw(t *testing.T) {
	raw := "2026-04-08T12:00:00.000000000Z tty line\r\n2026-04-08T12:00:01.000000000Z second\n"
	srv, st, mock, ts := newLogsWSServer(t, func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(raw)), nil
	})
	mock.ContainerInspectFn = func(context.Context, string) (container.InspectResponse, error) {
		return container.InspectResponse{Config: &container.Config{Tty: true}}, nil
	}
	seedApp(t, st, "ttyapp")

	ws := pumpWS(dialWSPath(t, ts, "/api/apps/ttyapp/logs", superAdminCookie(t, srv.jwt), nil))
	m := ws.next(t)
	if m["line"] != "tty line" || m["stream"] != "stdout" {
		t.Fatalf("frame 1 = %v", m)
	}
	m = ws.next(t)
	if m["line"] != "second" {
		t.Fatalf("frame 2 = %v", m)
	}
}

func TestLogsWSRejectsOversizedFrame(t *testing.T) {
	hdr := make([]byte, 8)
	hdr[0] = 1
	binary.BigEndian.PutUint32(hdr[4:], logbuf.MaxDockerFrameSize+1)
	data := append(wsTestFrame(1, "2026-04-08T12:00:00.000000000Z ok\n"), hdr...)
	srv, st, _, ts := newLogsWSServer(t, func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(data))), nil
	})
	seedApp(t, st, "bigapp")

	ws := pumpWS(dialWSPath(t, ts, "/api/apps/bigapp/logs", superAdminCookie(t, srv.jwt), nil))
	if m := ws.next(t); m["line"] != "ok" {
		t.Fatalf("frame 1 = %v", m)
	}
	m := ws.next(t)
	if msg, _ := m["error"].(string); !strings.Contains(msg, "1 MiB") {
		t.Fatalf("expected oversized-frame error, got %v", m)
	}
}

func TestLogsWSDoesNotEchoDockerErrors(t *testing.T) {
	srv, st, _, ts := newLogsWSServer(t, func(context.Context) (io.ReadCloser, error) {
		return nil, errors.New("Error response from daemon: open /var/lib/docker/containers/abc: permission denied")
	})
	seedApp(t, st, "errapp")

	ws := pumpWS(dialWSPath(t, ts, "/api/apps/errapp/logs", superAdminCookie(t, srv.jwt), nil))
	m := ws.next(t)
	if m["error"] != "could not read container logs" {
		t.Fatalf("error = %q, want generic message", m["error"])
	}
}

func TestEventsWSRecheckDropsLostAppTopics(t *testing.T) {
	setWSRecheck(t, 50*time.Millisecond)
	srv, st := newTestServer(t)
	srv.SetBus(events.New())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	appID := seedApp(t, st, "evapp")
	cookie, uid := makeRoleUserCookie(t, srv, st, "member", "manage", appID)

	conn := dialWSPath(t, ts, "/api/events", cookie, nil)
	ws := pumpWS(conn)
	if err := conn.WriteJSON(map[string]string{"op": "sub", "topic": "app:evapp"}); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(map[string]string{"op": "ping"}); err != nil {
		t.Fatal(err)
	}
	if f := ws.next(t); f["op"] != "pong" {
		t.Fatalf("frame = %v, want pong", f)
	}
	srv.bus.Publish(context.Background(), events.Event{Type: "app.status", Topic: "app:evapp"})
	if f := ws.next(t); f["topic"] != "app:evapp" {
		t.Fatalf("frame = %v, want app:evapp event", f)
	}

	if err := st.RevokeAppAccess(uid, appID); err != nil {
		t.Fatal(err)
	}
	f := ws.next(t)
	if f["op"] != "err" || f["topic"] != "app:evapp" || f["reason"] != "forbidden" {
		t.Fatalf("frame = %v, want forbidden err for app:evapp", f)
	}
	srv.bus.Publish(context.Background(), events.Event{Type: "app.status", Topic: "app:evapp"})
	ws.expectNone(t, 300*time.Millisecond)

	if err := st.DeleteUser(uid); err != nil {
		t.Fatal(err)
	}
	ws.expectPolicyClose(t)
}

func TestEventsWSClosesOnRoleChange(t *testing.T) {
	setWSRecheck(t, 50*time.Millisecond)
	srv, st := newTestServer(t)
	srv.SetBus(events.New())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	cookie, uid := makeRoleUserCookie(t, srv, st, "member", "manage")

	ws := pumpWS(dialWSPath(t, ts, "/api/events", cookie, nil))
	ws.expectNone(t, 150*time.Millisecond)
	if err := st.UpdateUserRole(uid, "viewer"); err != nil {
		t.Fatal(err)
	}
	ws.expectPolicyClose(t)
}

// deployLogsReconciler serves a test-controlled deploy log channel.
type deployLogsReconciler struct {
	mockReconciler
	ch chan deployer.OutputLine
}

func (m *deployLogsReconciler) SubscribeDeployLog(string) (<-chan deployer.OutputLine, func(), bool) {
	return m.ch, func() {}, true
}

func TestDeployLogsWSClosesWhenAccessRevoked(t *testing.T) {
	setWSRecheck(t, 50*time.Millisecond)
	srv, st := newTestServer(t)
	rec := &deployLogsReconciler{ch: make(chan deployer.OutputLine, 4)}
	srv.SetReconciler(rec)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	appID := seedApp(t, st, "depapp")
	cookie, uid := makeRoleUserCookie(t, srv, st, "member", "manage", appID)

	ws := pumpWS(dialWSPath(t, ts, "/api/apps/depapp/deploy-logs", cookie, nil))
	rec.ch <- deployer.OutputLine{Line: "pulling image"}
	if m := ws.next(t); m["line"] != "pulling image" {
		t.Fatalf("frame = %v", m)
	}
	ws.expectNone(t, 150*time.Millisecond)

	if err := st.RevokeAppAccess(uid, appID); err != nil {
		t.Fatal(err)
	}
	ws.expectPolicyClose(t)
}

func TestSystemLogsWSClosesWhenDemoted(t *testing.T) {
	setWSRecheck(t, 50*time.Millisecond)
	srv, st := newTestServer(t)
	srv.SetLogBuffer(logbuf.New(10))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	cookie, uid := makeRoleUserCookie(t, srv, st, "admin2", "super_admin")

	ws := pumpWS(dialWSPath(t, ts, "/api/system/process-logs/stream", cookie, nil))
	ws.expectNone(t, 150*time.Millisecond)
	if err := st.UpdateUserRole(uid, "manage"); err != nil {
		t.Fatal(err)
	}
	ws.expectPolicyClose(t)
}
