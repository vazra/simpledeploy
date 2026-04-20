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

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

// captureCtxReconciler embeds mockReconciler and captures the context passed
// to Reconcile, blocking until released so tests can inspect cancellation.
type captureCtxReconciler struct {
	mockReconciler
	gotCtx  chan context.Context
	release chan struct{}
}

func (c *captureCtxReconciler) Reconcile(ctx context.Context) error {
	c.gotCtx <- ctx
	<-c.release
	return nil
}

func TestHandleUpdateEndpoints_ReconcileCtxNotCancelled(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	s.UpsertApp(&store.App{Name: "ctxapp", Slug: "ctxapp", ComposePath: composePath, Status: "running"}, nil)

	rec := &captureCtxReconciler{
		gotCtx:  make(chan context.Context, 1),
		release: make(chan struct{}),
	}
	srv.SetReconciler(rec)

	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	endpoints := []compose.EndpointConfig{
		{Domain: "x.example.com", Port: "80", TLS: "letsencrypt", Service: "web"},
	}
	body, _ := json.Marshal(endpoints)
	req, _ := http.NewRequest(http.MethodPut, httpSrv.URL+"/api/apps/ctxapp/endpoints", bytes.NewReader(body))
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

	// Give any cancellation propagation time to land before checking.
	time.Sleep(50 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		t.Errorf("Reconcile context cancelled after request returned: %v", err)
	}
	close(rec.release)
}

func TestHandleUpdateEndpoints(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.endpoints.0.domain: old.example.com
      simpledeploy.endpoints.0.port: "80"
      simpledeploy.endpoints.0.tls: letsencrypt
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: composePath, Status: "running"}, nil)

	endpoints := []compose.EndpointConfig{
		{Domain: "new.example.com", Port: "80", TLS: "letsencrypt", Service: "web"},
	}
	body, _ := json.Marshal(endpoints)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/myapp/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(updated), "new.example.com") {
		t.Errorf("expected new.example.com in compose, got:\n%s", string(updated))
	}
	if strings.Contains(string(updated), "old.example.com") {
		t.Errorf("old.example.com still present in compose")
	}
}

func TestHandleUpdateEndpoints_NoExistingLabel(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "noapp", Slug: "noapp", ComposePath: composePath, Status: "stopped"}, nil)

	endpoints := []compose.EndpointConfig{
		{Domain: "brand.new.com", Port: "3000", TLS: "letsencrypt", Service: "web"},
	}
	body, _ := json.Marshal(endpoints)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/noapp/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(updated), "brand.new.com") {
		t.Errorf("expected brand.new.com in compose, got:\n%s", string(updated))
	}
	if !strings.Contains(string(updated), "simpledeploy.endpoints.0.domain") {
		t.Errorf("expected endpoint label in compose, got:\n%s", string(updated))
	}
}

func TestHandleUpdateEndpoints_MultiService(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
  api:
    image: node
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "multi", Slug: "multi", ComposePath: composePath, Status: "running"}, nil)

	endpoints := []compose.EndpointConfig{
		{Domain: "web.example.com", Port: "80", TLS: "letsencrypt", Service: "web"},
		{Domain: "api.example.com", Port: "8080", TLS: "custom", Service: "api"},
	}
	body, _ := json.Marshal(endpoints)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/multi/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	result := string(updated)
	if !strings.Contains(result, "web.example.com") {
		t.Errorf("expected web.example.com, got:\n%s", result)
	}
	if !strings.Contains(result, "api.example.com") {
		t.Errorf("expected api.example.com, got:\n%s", result)
	}
}
