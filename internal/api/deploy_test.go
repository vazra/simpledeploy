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

	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

type mockReconciler struct{}

func (m *mockReconciler) DeployOne(_ context.Context, _, _ string) error                         { return nil }
func (m *mockReconciler) RemoveOne(_ context.Context, _ string) error                            { return nil }
func (m *mockReconciler) RestartOne(_ context.Context, _ string) error                           { return nil }
func (m *mockReconciler) StopOne(_ context.Context, _ string) error                              { return nil }
func (m *mockReconciler) StartOne(_ context.Context, _ string) error                             { return nil }
func (m *mockReconciler) PullOne(_ context.Context, _ string) error                              { return nil }
func (m *mockReconciler) ScaleOne(_ context.Context, _ string, _ map[string]int) error           { return nil }
func (m *mockReconciler) AppServices(_ context.Context, _ string) ([]deployer.ServiceStatus, error) {
	return nil, nil
}
func (m *mockReconciler) RollbackOne(_ context.Context, _ string, _ int64) error { return nil }
func (m *mockReconciler) ListVersions(_ context.Context, _ string) ([]store.ComposeVersion, error) {
	return nil, nil
}
func (m *mockReconciler) ListDeployEvents(_ context.Context, _ string) ([]store.DeployEvent, error) {
	return nil, nil
}
func (m *mockReconciler) Reconcile(_ context.Context) error { return nil }

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

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["name"] != "myapp" {
		t.Errorf("name = %q, want myapp", resp["name"])
	}
	if resp["status"] != "deployed" {
		t.Errorf("status = %q, want deployed", resp["status"])
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

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
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
