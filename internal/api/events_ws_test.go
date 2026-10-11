package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/vazra/simpledeploy/internal/events"
	"github.com/vazra/simpledeploy/internal/store"
)

func newEventsTestServer(t *testing.T) (*httptest.Server, *Server, *http.Cookie) {
	t.Helper()
	srv, _, cookie := newAuditTestServer(t)
	srv.SetBus(events.New())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv, cookie
}

func dialEvents(t *testing.T, ts *httptest.Server, cookie *http.Cookie) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(ts.URL)
	wsURL := "ws://" + u.Host + "/api/events"
	hdr := http.Header{}
	hdr.Set("Origin", "http://"+u.Host)
	if cookie != nil {
		hdr.Set("Cookie", cookie.Name+"="+cookie.Value)
	}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial events: %v (status=%d)", err, resp.StatusCode)
		}
		t.Fatalf("dial events: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func readFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return f
}

func TestEventsWSAuthRequired(t *testing.T) {
	srv, _, _ := newAuditTestServer(t)
	srv.SetBus(events.New())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	_, resp, err := websocket.DefaultDialer.Dial("ws://"+u.Host+"/api/events", nil)
	if err == nil {
		t.Fatal("expected dial error for unauthed conn")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
}

func TestEventsWSSubAndReceive(t *testing.T) {
	ts, srv, cookie := newEventsTestServer(t)
	conn := dialEvents(t, ts, cookie)

	// Subscribe to a global topic the user can see.
	if err := conn.WriteJSON(map[string]string{"op": "sub", "topic": "global:apps"}); err != nil {
		t.Fatalf("write sub: %v", err)
	}
	// Give server a moment to process the sub.
	time.Sleep(50 * time.Millisecond)

	srv.bus.Publish(t.Context(), events.Event{Type: "app.status", Topic: "global:apps"})

	f := readFrame(t, conn, 2*time.Second)
	if f["topic"] != "global:apps" || f["type"] != "app.status" {
		t.Fatalf("unexpected frame: %v", f)
	}
}

func TestEventsWSForbiddenTopic(t *testing.T) {
	ts, _, cookie := newEventsTestServer(t)
	conn := dialEvents(t, ts, cookie)

	// super_admin can see global:users; subscribing to it succeeds (no err frame).
	// Use a clearly disallowed topic shape: a non-app: non-global: topic name.
	if err := conn.WriteJSON(map[string]string{"op": "sub", "topic": "secret:internal"}); err != nil {
		t.Fatalf("write sub: %v", err)
	}
	f := readFrame(t, conn, 2*time.Second)
	if f["op"] != "err" || !strings.Contains(f["reason"].(string), "forbidden") {
		t.Fatalf("expected err frame, got %v", f)
	}
}

func TestEventsWSPong(t *testing.T) {
	ts, _, cookie := newEventsTestServer(t)
	conn := dialEvents(t, ts, cookie)
	if err := conn.WriteJSON(map[string]string{"op": "ping"}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	f := readFrame(t, conn, 2*time.Second)
	if f["op"] != "pong" {
		t.Fatalf("expected pong, got %v", f)
	}
}

func TestEventsWSUnsub(t *testing.T) {
	ts, srv, cookie := newEventsTestServer(t)
	conn := dialEvents(t, ts, cookie)

	conn.WriteJSON(map[string]string{"op": "sub", "topic": "global:apps"})
	time.Sleep(50 * time.Millisecond)
	conn.WriteJSON(map[string]string{"op": "unsub", "topic": "global:apps"})
	time.Sleep(50 * time.Millisecond)

	srv.bus.Publish(t.Context(), events.Event{Type: "app.status", Topic: "global:apps"})

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected no frame after unsub")
	}
}

// Events tagged with an app reach only users who can view that app, even on
// a global topic every user may subscribe to.
func TestEventsWSAppTaggedEventsFiltered(t *testing.T) {
	srv, st, _ := newAuditTestServer(t)
	srv.SetBus(events.New())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	for _, slug := range []string{"mine", "theirs"} {
		if err := st.UpsertApp(&store.App{Name: slug, Slug: slug, ComposePath: "/x/" + slug + ".yml", Status: "running"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	mine, err := st.GetAppBySlug("mine")
	if err != nil {
		t.Fatal(err)
	}
	conn := dialEvents(t, ts, makeUserCookie(t, srv, st, "mgr", mine.ID))
	if err := conn.WriteJSON(map[string]string{"op": "sub", "topic": events.TopicGlobalBackups}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	for _, e := range events.BackupRunEvents("theirs") {
		srv.bus.Publish(t.Context(), e)
	}
	for _, e := range events.BackupRunEvents("mine") {
		srv.bus.Publish(t.Context(), e)
	}
	f := readFrame(t, conn, 2*time.Second)
	if f["topic"] != events.TopicGlobalBackups || f["type"] != events.TypeBackupRun {
		t.Fatalf("first frame = %v, want the event for the user's own app", f)
	}
	if _, leaked := f["app"]; leaked {
		t.Fatalf("frame exposes the app slug: %v", f)
	}
	// Nothing else: the other app's event was dropped.
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, data, err := conn.ReadMessage(); err == nil {
		t.Fatalf("unexpected extra frame: %s", data)
	}
}
