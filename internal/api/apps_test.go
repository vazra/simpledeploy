package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
	srv := NewServer(0, s)
	return srv, s
}

func TestListAppsEndpoint(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "app1", Slug: "app1", ComposePath: "/tmp/1.yml", Status: "running", Domain: "app1.example.com"}, nil)
	s.UpsertApp(&store.App{Name: "app2", Slug: "app2", ComposePath: "/tmp/2.yml", Status: "stopped"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/apps/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestListAppsEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
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
