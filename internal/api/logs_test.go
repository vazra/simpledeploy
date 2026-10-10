package api

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/logbuf"
	"github.com/vazra/simpledeploy/internal/store"
)

func newTestServerWithDocker(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	srv, s := newTestServer(t)
	srv.SetDocker(docker.NewMockClient())
	return srv, s
}

func TestHandleLogsAppNotFound(t *testing.T) {
	srv, _ := newTestServerWithDocker(t)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/nonexistent/logs", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Without WebSocket upgrade headers, upgrader returns 400 Bad Request if app exists.
	// But app doesn't exist so store.GetAppBySlug fails -> 404.
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleLogsRequiresAuth(t *testing.T) {
	srv, s := newTestServerWithDocker(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/logs", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestCheckWebSocketOrigin(t *testing.T) {
	cases := []struct {
		name      string
		host      string
		origin    string
		bearer    bool
		fetchSite string
		want      bool
	}{
		{"same host and port", "dash.example.com:8443", "https://dash.example.com:8443", false, "", true},
		{"case-insensitive", "Dash.Example.com:8443", "https://dash.EXAMPLE.com:8443", false, "", true},
		{"different port same host", "dash.example.com:8443", "http://dash.example.com:8080", false, "", false},
		{"origin port missing, host has port", "dash.example.com:8443", "https://dash.example.com", false, "", false},
		{"default https port implied", "dash.example.com", "https://dash.example.com", false, "", true},
		{"default https port explicit on host", "dash.example.com:443", "https://dash.example.com", false, "", true},
		{"default http port implied", "localhost", "http://localhost", false, "", true},
		{"different host", "dash.example.com", "https://evil.example.com", false, "", false},
		{"sibling subdomain", "dash.example.com", "https://app.example.com", false, "", false},
		{"ipv6 same port", "[::1]:8443", "http://[::1]:8443", false, "", true},
		{"ipv6 different port", "[::1]:8443", "http://[::1]:9000", false, "", false},
		{"null origin", "dash.example.com", "null", false, "", false},
		{"garbage origin", "dash.example.com", "://bad", false, "", false},
		{"no origin, cookie caller", "dash.example.com", "", false, "", false},
		{"no origin, bearer caller", "dash.example.com", "", true, "", true},
		{"proxy dropped port, browser says same-origin", "dash.example.com", "https://dash.example.com:8443", false, "same-origin", true},
		{"same-site sibling port", "dash.example.com:8443", "https://dash.example.com:8080", false, "same-site", false},
		{"cross-site", "dash.example.com", "https://evil.test", false, "cross-site", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/events", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.bearer {
				r.Header.Set("Authorization", "Bearer sd_x")
			}
			if tc.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			if got := checkWebSocketOrigin(r); got != tc.want {
				t.Fatalf("checkWebSocketOrigin(host=%q, origin=%q) = %v, want %v", tc.host, tc.origin, got, tc.want)
			}
		})
	}
}

func TestStreamContainerLogsDemux(t *testing.T) {
	var in bytes.Buffer
	in.Write(wsTestFrame(1, "2026-04-08T12:00:00.000000000Z out\n"))
	in.Write(wsTestFrame(2, "2026-04-08T12:00:01.000000000Z err\n"))
	var got []map[string]string
	err := streamContainerLogs(&in, false, func(stream, line string) error {
		got = append(got, logLineMessage(stream, line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["stream"] != "stdout" || got[0]["line"] != "out" ||
		got[1]["stream"] != "stderr" || got[1]["line"] != "err" || got[1]["ts"] != "2026-04-08T12:00:01.000000000Z" {
		t.Fatalf("got %v", got)
	}
}

func TestStreamContainerLogsOversizedFrame(t *testing.T) {
	hdr := make([]byte, 8)
	hdr[0] = 1
	binary.BigEndian.PutUint32(hdr[4:], 0xFFFFFFFF)
	sent := 0
	err := streamContainerLogs(bytes.NewReader(hdr), false, func(string, string) error { sent++; return nil })
	if !errors.Is(err, logbuf.ErrDockerFrameTooLarge) {
		t.Fatalf("err = %v, want ErrDockerFrameTooLarge", err)
	}
	if sent != 0 {
		t.Fatalf("sent %d lines", sent)
	}
}

func TestStreamContainerLogsTTY(t *testing.T) {
	long := strings.Repeat("x", maxTTYLogLine+10)
	raw := "first\r\n" + long + "\nlast"
	var lines []string
	err := streamContainerLogs(strings.NewReader(raw), true, func(stream, line string) error {
		if stream != "stdout" {
			t.Errorf("stream = %q", stream)
		}
		lines = append(lines, logLineMessage(stream, line)["line"])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || lines[0] != "first" || lines[2] != "last" {
		t.Fatalf("lines = %q", lines)
	}
	if lines[1] != strings.Repeat("x", maxTTYLogLine)+logbuf.TruncatedMarker {
		t.Fatalf("long line len = %d, want truncated", len(lines[1]))
	}
}

func TestStreamContainerLogsClientGone(t *testing.T) {
	var in bytes.Buffer
	in.Write(wsTestFrame(1, "a\n"))
	in.Write(wsTestFrame(1, "b\n"))
	calls := 0
	err := streamContainerLogs(&in, false, func(string, string) error { calls++; return errors.New("closed") })
	if !errors.Is(err, errLogClientGone) || calls != 1 {
		t.Fatalf("err = %v calls = %d", err, calls)
	}
	calls = 0
	err = streamContainerLogs(strings.NewReader("a\nb\n"), true, func(string, string) error { calls++; return errors.New("closed") })
	if !errors.Is(err, errLogClientGone) || calls != 1 {
		t.Fatalf("tty: err = %v calls = %d", err, calls)
	}
}
