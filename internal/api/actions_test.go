package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

type mockReconcilerFull struct {
	calls []string
}

func (m *mockReconcilerFull) DeployOne(_ context.Context, _, name string) error {
	m.calls = append(m.calls, "DeployOne:"+name)
	return nil
}
func (m *mockReconcilerFull) RemoveOne(_ context.Context, name string) error {
	m.calls = append(m.calls, "RemoveOne:"+name)
	return nil
}
func (m *mockReconcilerFull) RestartOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "RestartOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) StopOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "StopOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) StartOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "StartOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) PullOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "PullOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) ScaleOne(_ context.Context, slug string, _ map[string]int) error {
	m.calls = append(m.calls, "ScaleOne:"+slug)
	return nil
}

func (m *mockReconcilerFull) AppServices(_ context.Context, slug string) ([]deployer.ServiceStatus, error) {
	m.calls = append(m.calls, "AppServices:"+slug)
	return []deployer.ServiceStatus{{Service: "web", State: "running", Health: "healthy"}}, nil
}
func (m *mockReconcilerFull) RollbackOne(_ context.Context, slug string, versionID int64) error {
	m.calls = append(m.calls, fmt.Sprintf("RollbackOne:%s:%d", slug, versionID))
	return nil
}
func (m *mockReconcilerFull) ListVersions(_ context.Context, slug string) ([]store.ComposeVersion, error) {
	m.calls = append(m.calls, "ListVersions:"+slug)
	return []store.ComposeVersion{}, nil
}
func (m *mockReconcilerFull) ListDeployEvents(_ context.Context, slug string) ([]store.DeployEvent, error) {
	m.calls = append(m.calls, "ListDeployEvents:"+slug)
	return []store.DeployEvent{}, nil
}
func (m *mockReconcilerFull) Reconcile(_ context.Context) error { return nil }

func newActionTestServer(t *testing.T) (*Server, *mockReconcilerFull) {
	t.Helper()
	srv, _ := newTestServer(t)
	mock := &mockReconcilerFull{}
	srv.SetReconciler(mock)
	srv.SetAppsDir(t.TempDir())
	return srv, mock
}

func TestRestartEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/restart", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "RestartOne:myapp" {
		t.Errorf("expected RestartOne:myapp, got: %v", mock.calls)
	}
}

func TestStopEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/stop", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "StopOne:myapp" {
		t.Errorf("expected StopOne:myapp, got: %v", mock.calls)
	}
}

func TestStartEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/start", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "StartOne:myapp" {
		t.Errorf("expected StartOne:myapp, got: %v", mock.calls)
	}
}

func TestPullEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/pull", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "PullOne:myapp" {
		t.Errorf("expected PullOne:myapp, got: %v", mock.calls)
	}
}

func TestScaleEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	body, _ := json.Marshal(map[string]any{"scales": map[string]int{"web": 3}})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "ScaleOne:myapp" {
		t.Errorf("expected ScaleOne:myapp, got: %v", mock.calls)
	}
}

func TestRollbackEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	body, _ := json.Marshal(map[string]any{"version_id": 5})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "RollbackOne:myapp:5" {
		t.Errorf("expected RollbackOne:myapp:5, got: %v", mock.calls)
	}
}

func TestScaleEndpointMissingBody(t *testing.T) {
	srv, _ := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	body, _ := json.Marshal(map[string]any{"scales": map[string]int{}})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
