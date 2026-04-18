package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/config"
	"gopkg.in/yaml.v3"
)

func writeTempConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.DefaultConfig()
	cfg.MasterSecret = "test-secret"
	if err := cfg.SaveAtomic(path); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return cfg, path
}

func TestGetPublicHost(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)
	cfg, path := writeTempConfig(t)
	cfg.PublicHost = "example.com"
	srv.SetConfig(cfg, path)

	req := authedRequest(t, http.MethodGet, "/api/system/public-host", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["public_host"] != "example.com" {
		t.Errorf("public_host = %q, want example.com", resp["public_host"])
	}
}

func TestPutPublicHostAsAdmin(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)
	cfg, path := writeTempConfig(t)
	srv.SetConfig(cfg, path)

	req := authedRequest(t, http.MethodPut, "/api/system/public-host",
		map[string]string{"public_host": "deploy.example.org"}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if cfg.PublicHost != "deploy.example.org" {
		t.Errorf("in-memory cfg.PublicHost = %q", cfg.PublicHost)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	reloaded := config.DefaultConfig()
	if err := yaml.Unmarshal(data, reloaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if reloaded.PublicHost != "deploy.example.org" {
		t.Errorf("persisted public_host = %q", reloaded.PublicHost)
	}
}

func TestPutPublicHostAsNonAdmin(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	cfg, path := writeTempConfig(t)
	srv.SetConfig(cfg, path)

	userCookie := loginAs(t, srv, st, "bob", "password123", "viewer")
	req := authedRequest(t, http.MethodPut, "/api/system/public-host",
		map[string]string{"public_host": "evil.example.com"}, userCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", w.Code, w.Body.String())
	}
}

func TestPutPublicHostInvalid(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)
	cfg, path := writeTempConfig(t)
	srv.SetConfig(cfg, path)

	req := authedRequest(t, http.MethodPut, "/api/system/public-host",
		map[string]string{"public_host": "not a host!!!"}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}
