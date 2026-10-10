package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/compose"
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

// GET /api/apps/{slug} still shows endpoints when the .env is broken or the
// compose file is a link.
func TestGetAppEndpointsParseFallbacks(t *testing.T) {
	const withEndpoint = "services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: shown.example.com\n"
	cases := map[string]func(t *testing.T, dir string){
		"broken .env": func(t *testing.T, dir string) {
			os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(withEndpoint), 0o600)
			os.WriteFile(filepath.Join(dir, ".env"), []byte("A=\"unclosed\n"), 0o600)
		},
		"symlinked compose": func(t *testing.T, dir string) {
			target := filepath.Join(t.TempDir(), "compose.yml")
			os.WriteFile(target, []byte(withEndpoint), 0o600)
			if err := os.Symlink(target, filepath.Join(dir, "docker-compose.yml")); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			srv, s := newTestServer(t)
			dir := t.TempDir()
			setup(t, dir)
			composePath := filepath.Join(dir, "docker-compose.yml")
			if _, err := compose.ParseFile(composePath, "fb"); err == nil {
				t.Fatal("ParseFile succeeded; the fallback is not exercised")
			}
			s.UpsertApp(&store.App{Name: "fb", Slug: "fb", ComposePath: composePath, Status: "running"}, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/apps/fb", nil)
			req.AddCookie(superAdminCookie(t, srv.jwt))
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
			}
			var resp struct {
				Endpoints []compose.EndpointConfig `json:"endpoints"`
			}
			json.NewDecoder(w.Body).Decode(&resp)
			if len(resp.Endpoints) != 1 || resp.Endpoints[0].Domain != "shown.example.com" {
				t.Fatalf("endpoints = %+v, want shown.example.com", resp.Endpoints)
			}
		})
	}
}

func TestParseComposeForDisplayRefusesFIFOTarget(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.Symlink(fifo, composePath); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := parseComposeForDisplay(composePath, "fifo")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error for a FIFO target")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseComposeForDisplay blocked on a FIFO")
	}
}
