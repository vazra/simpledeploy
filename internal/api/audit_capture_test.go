package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

// newAuditTestServer creates a Server+Store with an audit.Recorder wired in.
func newAuditTestServer(t *testing.T) (*Server, *store.Store, *http.Cookie) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := s.CreateUser("admin", hash, "super_admin", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	jwtMgr := auth.NewJWTManager("test-secret", 24*time.Hour)
	rl := auth.NewRateLimiter(10000, time.Minute)
	srv := NewServer(0, s, jwtMgr, rl)
	srv.SetAudit(audit.NewRecorder(s))

	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": "admin",
		"password": "password123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d", w.Code)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	return srv, s, cookie
}

// findAuditEntry searches audit_log for a matching category+action.
func findAuditEntry(t *testing.T, s *store.Store, category, action string) *store.AuditEntry {
	t.Helper()
	rows, _, err := s.ListActivity(context.Background(), store.ActivityFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	for i := range rows {
		if rows[i].Category == category && rows[i].Action == action {
			return &rows[i]
		}
	}
	return nil
}

// --- 8.1 Compose ---

func TestAuditComposeChanged(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx:1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app := &store.App{Name: "cmpapp", Slug: "cmpapp", ComposePath: composePath, Status: "running"}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}
	// Get app ID after upsert.
	storedApp, err := s.GetAppBySlug("cmpapp")
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	// Create a compose version.
	if err := s.CreateComposeVersion(storedApp.ID, "services:\n  web:\n    image: nginx:2\n", "sha256:abc"); err != nil {
		t.Fatalf("create compose version: %v", err)
	}
	// Get the version ID.
	versions, err := s.ListComposeVersions(storedApp.ID)
	if err != nil || len(versions) == 0 {
		t.Fatalf("list versions: %v (count=%d)", err, len(versions))
	}
	versionID := versions[0].ID

	req := authedRequest(t, http.MethodPost,
		fmt.Sprintf("/api/apps/cmpapp/versions/%d/restore", versionID),
		nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("restore status = %d, want 202; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "compose", "changed")
	if e == nil {
		t.Fatal("no compose/changed audit row found")
	}
	if e.AppSlug != "cmpapp" {
		t.Errorf("app_slug = %q, want cmpapp", e.AppSlug)
	}
}

// --- 8.2 Endpoints ---

func TestAuditEndpointAdded(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertApp(&store.App{Name: "epapp", Slug: "epapp", ComposePath: composePath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	req := authedRequest(t, http.MethodPut, "/api/apps/epapp/endpoints",
		[]map[string]string{{"domain": "ep.example.com", "service": "web"}},
		cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("endpoint status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "endpoint", "added")
	if e == nil {
		t.Fatal("no endpoint/added audit row found")
	}
	if e.AppSlug != "epapp" {
		t.Errorf("app_slug = %q, want epapp", e.AppSlug)
	}
}

// --- 8.3 Backups ---

func TestAuditBackupAdded(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	if err := s.UpsertApp(&store.App{Name: "bkapp", Slug: "bkapp", ComposePath: "/dev/null", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	req := authedRequest(t, http.MethodPost, "/api/apps/bkapp/backups/configs",
		map[string]any{
			"strategy":        "volume",
			"target":          "local",
			"schedule_cron":   "0 2 * * *",
			"retention_mode":  "count",
			"retention_count": 5,
		}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("backup status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "backup", "added")
	if e == nil {
		t.Fatal("no backup/added audit row found")
	}
	if e.AppSlug != "bkapp" {
		t.Errorf("app_slug = %q, want bkapp", e.AppSlug)
	}
}

// --- 8.4 Alert rules ---

func TestAuditAlertAdded(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	wh := &store.Webhook{Name: "aud-wh", Type: "slack", URL: "https://hooks.example.com/test"}
	if err := s.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	req := authedRequest(t, http.MethodPost, "/api/alerts/rules", map[string]any{
		"metric":       "cpu_pct",
		"operator":     ">",
		"threshold":    80.0,
		"duration_sec": 300,
		"webhook_id":   wh.ID,
		"enabled":      true,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("alert status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "alert", "added")
	if e == nil {
		t.Fatal("no alert/added audit row found")
	}
}

// --- 8.5 Webhooks ---

func TestAuditWebhookAdded(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	req := authedRequest(t, http.MethodPost, "/api/webhooks", map[string]string{
		"name": "my-webhook",
		"type": "slack",
		"url":  "https://hooks.slack.com/services/test",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("webhook status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "webhook", "added")
	if e == nil {
		t.Fatal("no webhook/added audit row found")
	}
}

// --- 8.6 Registries ---

func TestAuditRegistryAdded(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)
	srv.SetMasterSecret("test-secret-32-bytes-padded-here!")

	req := authedRequest(t, http.MethodPost, "/api/registries", map[string]string{
		"name":     "my-registry",
		"url":      "https://registry.example.com",
		"username": "user",
		"password": "pass",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("registry status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "registry", "added")
	if e == nil {
		t.Fatal("no registry/added audit row found")
	}
}

// --- 8.7 Access ---

func TestAuditAccessAdded(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	// Create another user and an app.
	hash, _ := auth.HashPassword("pw")
	u, err := s.CreateUser("viewer1", hash, "viewer", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.UpsertApp(&store.App{Name: "accapp", Slug: "accapp", ComposePath: "/dev/null", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	req := authedRequest(t, http.MethodPost, fmt.Sprintf("/api/users/%d/access", u.ID),
		map[string]string{"app_slug": "accapp"}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("grant access status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "access", "added")
	if e == nil {
		t.Fatal("no access/added audit row found")
	}
	if e.AppSlug != "accapp" {
		t.Errorf("app_slug = %q, want accapp", e.AppSlug)
	}
}

// --- 8.8 Lifecycle ---

func TestAuditLifecycleRemoved(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	srv.SetAppsDir(dir)
	appDir := filepath.Join(dir, "rmapp")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertApp(&store.App{Name: "rmapp", Slug: "rmapp", ComposePath: composePath, Status: "stopped"}, nil); err != nil {
		t.Fatal(err)
	}

	req := authedRequest(t, http.MethodDelete, "/api/apps/rmapp", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("remove status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "lifecycle", "removed")
	if e == nil {
		t.Fatal("no lifecycle/removed audit row found")
	}
	if e.AppSlug != "rmapp" {
		t.Errorf("app_slug = %q, want rmapp", e.AppSlug)
	}
}
