package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	// Create a test user so auth middleware can validate JWT user existence
	if _, err := s.CreateUser("admin", "hashed", "super_admin", "", ""); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv := NewServer(0, s, jwtMgr, nil)
	return srv, s
}

// superAdminCookie generates a session cookie for a super_admin user.
func superAdminCookie(t *testing.T, jwtMgr *auth.JWTManager) *http.Cookie {
	t.Helper()
	token, err := jwtMgr.Generate(1, "manage", "super_admin", 1)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return &http.Cookie{Name: "session", Value: token}
}

func TestListAppsEndpoint(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "app1", Slug: "app1", ComposePath: "/tmp/1.yml", Status: "running", Domain: "app1.example.com"}, nil)
	s.UpsertApp(&store.App{Name: "app2", Slug: "app2", ComposePath: "/tmp/2.yml", Status: "stopped"}, nil)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var apps []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 2 {
		t.Errorf("got %d apps, want 2", len(apps))
	}
}

func TestGetAppEndpoint(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running", Domain: "myapp.example.com"}, nil)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var app map[string]interface{}
	json.NewDecoder(w.Body).Decode(&app)
	if app["Name"] != "myapp" {
		t.Errorf("Name = %v, want myapp", app["Name"])
	}
}

func TestGetAppNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/nonexistent", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	// super_admin bypasses app access check, so store returns 404 from handleGetApp
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestGetAppIncludesAccessAllow(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	s.UpsertApp(&store.App{Name: "ipapp", Slug: "ipapp", ComposePath: "/tmp/test.yml", Status: "running"}, map[string]string{
		"simpledeploy.access.allow": "10.0.0.0/8,192.168.1.5",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/apps/ipapp", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	labels, ok := resp["Labels"].(map[string]interface{})
	if !ok {
		t.Fatal("Labels not in response or not a map")
	}
	if labels["simpledeploy.access.allow"] != "10.0.0.0/8,192.168.1.5" {
		t.Errorf("access.allow = %v, want %q", labels["simpledeploy.access.allow"], "10.0.0.0/8,192.168.1.5")
	}
}

func TestListAppsEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var apps []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 0 {
		t.Errorf("got %d apps, want 0", len(apps))
	}
}
