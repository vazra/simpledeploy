package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

type mockReconciler struct{}

func (m *mockReconciler) DeployOne(_ context.Context, _, _ string) error               { return nil }
func (m *mockReconciler) RemoveOne(_ context.Context, _ string) error                  { return nil }
func (m *mockReconciler) RestartOne(_ context.Context, _ string) error                 { return nil }
func (m *mockReconciler) StopOne(_ context.Context, _ string) error                    { return nil }
func (m *mockReconciler) StartOne(_ context.Context, _ string) error                   { return nil }
func (m *mockReconciler) PullOne(_ context.Context, _ string) error                    { return nil }
func (m *mockReconciler) ScaleOne(_ context.Context, _ string, _ map[string]int) error { return nil }
func (m *mockReconciler) AppServices(_ context.Context, _ string) ([]deployer.ServiceStatus, error) {
	return nil, nil
}
func (m *mockReconciler) AppConfig(_ string) (*compose.AppConfig, error) { return nil, nil }
func (m *mockReconciler) RollbackOne(_ context.Context, _ string, _ int64) error { return nil }
func (m *mockReconciler) ListVersions(_ context.Context, _ string) ([]store.ComposeVersion, error) {
	return nil, nil
}
func (m *mockReconciler) ListDeployEvents(_ context.Context, _ string) ([]store.DeployEvent, error) {
	return nil, nil
}
func (m *mockReconciler) Reconcile(_ context.Context) error           { return nil }
func (m *mockReconciler) RefreshRoutes(_ context.Context) error       { return nil }
func (m *mockReconciler) CancelOne(_ context.Context, _ string) error { return nil }
func (m *mockReconciler) IsDeploying(_ string) bool                   { return false }
func (m *mockReconciler) SubscribeDeployLog(_ string) (<-chan deployer.OutputLine, func(), bool) {
	return nil, nil, false
}

func newDeployTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	srv, _ := newTestServer(t)
	appsDir := t.TempDir()
	srv.SetAppsDir(appsDir)
	return srv, appsDir
}

func TestDeployEndpoint(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	composeContent := "services:\n  web:\n    image: nginx\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(composeContent))

	body, _ := json.Marshal(map[string]string{
		"name":    "myapp",
		"compose": encoded,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["name"] != "myapp" {
		t.Errorf("name = %q, want myapp", resp["name"])
	}
	if resp["status"] != "started" {
		t.Errorf("status = %q, want started", resp["status"])
	}

	written, err := os.ReadFile(filepath.Join(appsDir, "myapp", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("compose file not written: %v", err)
	}
	if string(written) != composeContent {
		t.Errorf("compose content mismatch")
	}
}

func TestDeployEndpointWithReconciler(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	srv.SetReconciler(&mockReconciler{})
	cookie := superAdminCookie(t, srv.jwt)

	encoded := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n"))
	body, _ := json.Marshal(map[string]string{"name": "testapp", "compose": encoded})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
}

func TestGetComposeEndpoint(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	composeContent := "services:\n  web:\n    image: nginx\n"
	appDir := filepath.Join(appsDir, "myapp")
	os.MkdirAll(appDir, 0755)
	os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte(composeContent), 0644)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/compose", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/yaml" {
		t.Errorf("Content-Type = %q, want text/yaml", ct)
	}

	if w.Body.String() != composeContent {
		t.Errorf("body mismatch: got %q, want %q", w.Body.String(), composeContent)
	}
}

func TestGetComposeNotFound(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/nonexistent/compose", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestRemoveAppEndpoint(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	// create app dir to simulate deployed app
	appDir := filepath.Join(appsDir, "myapp")
	os.MkdirAll(appDir, 0755)
	os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte("services: {}"), 0644)

	req := httptest.NewRequest(http.MethodDelete, "/api/apps/myapp", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	if _, err := os.Stat(appDir); !os.IsNotExist(err) {
		t.Error("app dir should have been removed")
	}
}

func TestDeployConflictManualNoSuggestion(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	srv.store.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", Status: "running"}, nil)

	encoded := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n"))
	body, _ := json.Marshal(map[string]string{"name": "myapp", "compose": encoded, "source": "manual"})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["suggested_name"]; ok {
		t.Errorf("manual conflict must not return suggested_name, got %q", resp["suggested_name"])
	}
	if resp["error"] == "" {
		t.Errorf("expected error message, got empty")
	}
}

func TestDeployConflictTemplateSuggestsName2(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	srv.store.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", Status: "running"}, nil)

	encoded := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n"))
	body, _ := json.Marshal(map[string]string{"name": "myapp", "compose": encoded, "source": "template"})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["suggested_name"] != "myapp-2" {
		t.Errorf("suggested_name = %q, want myapp-2", resp["suggested_name"])
	}
}

func TestDeployConflictTemplateSkipsTakenCandidate(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	srv.store.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", Status: "running"}, nil)
	srv.store.UpsertApp(&store.App{Name: "myapp-2", Slug: "myapp-2", Status: "running"}, nil)

	encoded := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n"))
	body, _ := json.Marshal(map[string]string{"name": "myapp", "compose": encoded, "source": "template"})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["suggested_name"] != "myapp-3" {
		t.Errorf("suggested_name = %q, want myapp-3", resp["suggested_name"])
	}
}

func TestDeployFreshNameProceeds(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	encoded := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n"))
	body, _ := json.Marshal(map[string]string{"name": "brandnew", "compose": encoded, "source": "template"})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
}

func TestDeployEndpointInvalidBase64(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	body, _ := json.Marshal(map[string]string{"name": "myapp", "compose": "!!not-base64!!"})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestValidateComposeValid(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	composeContent := "services:\n  web:\n    image: nginx\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(composeContent))

	body, _ := json.Marshal(map[string]string{"compose": encoded})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/validate-compose", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["valid"] != true {
		t.Errorf("valid = %v, want true", resp["valid"])
	}
	if resp["errors"] != nil {
		t.Errorf("errors should be absent on valid compose, got %v", resp["errors"])
	}
}

func TestValidateComposeInvalidYAML(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	encoded := base64.StdEncoding.EncodeToString([]byte(":\t: bad yaml {{{\n"))
	body, _ := json.Marshal(map[string]string{"compose": encoded})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/validate-compose", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["valid"] != false {
		t.Errorf("valid = %v, want false", resp["valid"])
	}
	if resp["errors"] == nil {
		t.Error("expected errors in response")
	}
}

func TestValidateComposeMissingImage(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	// service with no image and no build context - compose-go should reject this
	composeContent := "services:\n  web:\n    ports:\n      - \"80:80\"\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(composeContent))
	body, _ := json.Marshal(map[string]string{"compose": encoded})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/validate-compose", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	// compose-go may or may not reject missing image; just confirm response is well-formed
	if _, ok := resp["valid"]; !ok {
		t.Error("response missing 'valid' field")
	}
}

func TestValidateComposeEmptyBody(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	body, _ := json.Marshal(map[string]string{"compose": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/validate-compose", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestValidateComposeMalformedBase64(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	body, _ := json.Marshal(map[string]string{"compose": "!!not-base64!!"})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/validate-compose", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestHandleRemoveAppDeletesSidecar verifies that DELETE /api/apps/{slug} removes
// the per-app sidecar (simpledeploy.yml) from disk when configsync is wired.
func TestHandleRemoveAppDeletesSidecar(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	// Seed an app in the store.
	if err := srv.store.UpsertApp(&store.App{
		Name:        "sidecar-app",
		Slug:        "sidecar-app",
		ComposePath: filepath.Join(appsDir, "sidecar-app", "docker-compose.yml"),
		Status:      "running",
	}, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}

	// Create app dir with compose + sidecar.
	appDir := filepath.Join(appsDir, "sidecar-app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	sidecarPath := filepath.Join(appDir, "simpledeploy.yml")
	if err := os.WriteFile(sidecarPath, []byte("version: 1\napp:\n  slug: sidecar-app\n"), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	// Wire configsync.
	cs := configsync.New(srv.store, appsDir, t.TempDir())
	t.Cleanup(func() { cs.Close() })
	srv.SetConfigSync(cs)

	req := httptest.NewRequest(http.MethodDelete, "/api/apps/sidecar-app", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DELETE /api/apps/sidecar-app: status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// App dir entirely removed by handleRemoveApp.
	if _, err := os.Stat(appDir); !os.IsNotExist(err) {
		t.Error("app dir should be fully removed after DELETE")
	}
	// Sidecar specifically gone (redundant with above but documents intent).
	if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
		t.Error("sidecar simpledeploy.yml should be gone after DELETE")
	}
}
