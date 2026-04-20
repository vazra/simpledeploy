package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestHandleUpdateAccess_ReconcileCtxNotCancelled(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	s.UpsertApp(&store.App{Name: "accctxapp", Slug: "accctxapp", ComposePath: composePath, Status: "running"}, nil)

	rec := &captureCtxReconciler{
		gotCtx:  make(chan context.Context, 1),
		release: make(chan struct{}),
	}
	srv.SetReconciler(rec)

	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	body, _ := json.Marshal(map[string]string{"allow": "192.168.1.0/24"})
	req, _ := http.NewRequest(http.MethodPut, httpSrv.URL+"/api/apps/accctxapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	var ctx context.Context
	select {
	case ctx = <-rec.gotCtx:
	case <-time.After(2 * time.Second):
		t.Fatal("Reconcile not invoked within 2s")
	}
	time.Sleep(50 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		t.Errorf("Reconcile context cancelled after request returned: %v", err)
	}
	close(rec.release)
}

func TestHandleUpdateAccess(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: app.example.com
      simpledeploy.access.allow: "10.0.0.0/8"
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": "192.168.1.0/24,203.0.113.5"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/myapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if !strings.Contains(string(updated), "192.168.1.0/24,203.0.113.5") {
		t.Errorf("expected new allowlist in compose, got:\n%s", string(updated))
	}
}

func TestHandleUpdateAccess_InvalidIP(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "badapp", Slug: "badapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": "not-an-ip"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/badapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateAccess_ClearAllowlist(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: app.example.com
      simpledeploy.access.allow: "10.0.0.0/8"
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "clrapp", Slug: "clrapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": ""})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/clrapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateAccess_NoExistingLabel(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: app.example.com
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "newapp", Slug: "newapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"allow": "10.0.0.1"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/newapp/access", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if !strings.Contains(string(updated), "simpledeploy.access.allow") {
		t.Errorf("expected access.allow label added, got:\n%s", string(updated))
	}
}
