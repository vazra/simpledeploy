package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vazra/simpledeploy/internal/docker"
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
