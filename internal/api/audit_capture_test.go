package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
// NOTE: ListActivity omits before_json/after_json. Use findFullAuditEntry
// when you need to assert BeforeJSON/AfterJSON content.
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

// findFullAuditEntry returns the full audit row (including BeforeJSON/AfterJSON)
// for the first entry matching category+action, using GetActivity.
func findFullAuditEntry(t *testing.T, s *store.Store, category, action string) *store.AuditEntry {
	t.Helper()
	slim := findAuditEntry(t, s, category, action)
	if slim == nil {
		return nil
	}
	full, err := s.GetActivity(context.Background(), slim.ID)
	if err != nil {
		t.Fatalf("GetActivity(%d): %v", slim.ID, err)
	}
	return &full
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

func TestAuditLifecycleCreated(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	srv.SetAppsDir(dir)
	srv.SetReconciler(&mockReconciler{})

	composeYAML := "services:\n  web:\n    image: nginx:latest\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(composeYAML))

	req := authedRequest(t, http.MethodPost, "/api/apps/deploy",
		map[string]any{
			"name":    "newapp",
			"compose": encoded,
		}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("deploy status = %d, want 202; body: %s", w.Code, w.Body.String())
	}

	e := findFullAuditEntry(t, s, "lifecycle", "created")
	if e == nil {
		t.Fatal("no lifecycle/created audit row found")
	}
	if e.AppSlug != "newapp" {
		t.Errorf("app_slug = %q, want newapp", e.AppSlug)
	}

	// Verify AfterJSON snapshot contains the app name.
	if e.AfterJSON == nil {
		t.Fatal("lifecycle/created AfterJSON is nil")
	}
	var after map[string]any
	if err := json.Unmarshal(e.AfterJSON, &after); err != nil {
		t.Fatalf("unmarshal AfterJSON: %v", err)
	}
	if after["name"] != "newapp" {
		t.Errorf("AfterJSON[name] = %v, want newapp", after["name"])
	}
}

func TestAuditComposeDiffOnDeploy(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	srv.SetAppsDir(dir)
	srv.SetReconciler(&mockReconciler{})

	composeYAML := "services:\n  web:\n    image: nginx:1.25\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(composeYAML))

	req := authedRequest(t, http.MethodPost, "/api/apps/deploy",
		map[string]any{
			"name":    "diffapp",
			"compose": encoded,
		}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("deploy status = %d, want 202; body: %s", w.Code, w.Body.String())
	}

	e := findFullAuditEntry(t, s, "compose", "changed")
	if e == nil {
		t.Fatal("no compose/changed audit row found")
	}
	if e.AfterJSON == nil {
		t.Fatal("compose/changed AfterJSON is nil; diff not captured")
	}
	var after map[string]any
	if err := json.Unmarshal(e.AfterJSON, &after); err != nil {
		t.Fatalf("unmarshal AfterJSON: %v", err)
	}
	services, ok := after["services"].(map[string]any)
	if !ok || services["web"] == nil {
		t.Errorf("compose/changed AfterJSON does not contain 'web' service; got %v", after)
	}
	web, _ := services["web"].(map[string]any)
	if web["image"] != "nginx:1.25" {
		t.Errorf("image = %v, want nginx:1.25", web["image"])
	}
}

func TestAuditComposeDiffOnRedeployRename(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	srv.SetAppsDir(dir)
	srv.SetReconciler(&mockReconciler{})

	// Initial deploy with service "web".
	first := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx:1.25\n"))
	req := authedRequest(t, http.MethodPost, "/api/apps/deploy",
		map[string]any{"name": "renameapp", "compose": first}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("first deploy status = %d, body: %s", w.Code, w.Body.String())
	}

	// Redeploy renaming "web" to "api".
	second := base64.StdEncoding.EncodeToString([]byte("services:\n  api:\n    image: nginx:1.25\n"))
	req = authedRequest(t, http.MethodPost, "/api/apps/deploy",
		map[string]any{"name": "renameapp", "compose": second, "force": true}, cookie)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("redeploy status = %d, body: %s", w.Code, w.Body.String())
	}

	// Find the most recent compose/changed; expect Before with web and After with api.
	entries, _, err := s.ListActivity(context.Background(), store.ActivityFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var latest *store.AuditEntry
	for i := range entries {
		if entries[i].Category == "compose" && entries[i].Action == "changed" {
			full, err := s.GetActivity(context.Background(), entries[i].ID)
			if err != nil {
				t.Fatal(err)
			}
			if latest == nil || full.ID > latest.ID {
				e := full
				latest = &e
			}
		}
	}
	if latest == nil {
		t.Fatal("no compose/changed entries found")
	}
	if latest.BeforeJSON == nil {
		t.Fatal("redeploy compose/changed BeforeJSON is nil; rename diff not captured")
	}
	if !strings.Contains(latest.Summary, "web removed") || !strings.Contains(latest.Summary, "api added") {
		t.Errorf("summary should mention web removed and api added; got %q", latest.Summary)
	}
}

func TestAuditComposeDiffOnRestore(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx:1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app := &store.App{Name: "rstapp", Slug: "rstapp", ComposePath: composePath, Status: "running"}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}
	storedApp, err := s.GetAppBySlug("rstapp")
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	newContent := "services:\n  web:\n    image: nginx:2\n"
	if err := s.CreateComposeVersion(storedApp.ID, newContent, "sha256:def"); err != nil {
		t.Fatalf("create compose version: %v", err)
	}
	versions, err := s.ListComposeVersions(storedApp.ID)
	if err != nil || len(versions) == 0 {
		t.Fatalf("list versions: %v (count=%d)", err, len(versions))
	}
	versionID := versions[0].ID

	req := authedRequest(t, http.MethodPost,
		fmt.Sprintf("/api/apps/rstapp/versions/%d/restore", versionID),
		nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("restore status = %d, want 202; body: %s", w.Code, w.Body.String())
	}

	e := findFullAuditEntry(t, s, "compose", "changed")
	if e == nil {
		t.Fatal("no compose/changed audit row found")
	}
	if e.BeforeJSON == nil {
		t.Fatal("compose/changed BeforeJSON is nil; old content not captured")
	}
	if e.AfterJSON == nil {
		t.Fatal("compose/changed AfterJSON is nil; new content not captured")
	}
	var before, after map[string]any
	if err := json.Unmarshal(e.BeforeJSON, &before); err != nil {
		t.Fatalf("unmarshal BeforeJSON: %v", err)
	}
	if err := json.Unmarshal(e.AfterJSON, &after); err != nil {
		t.Fatalf("unmarshal AfterJSON: %v", err)
	}
	beforeSvcs := before["services"].(map[string]any)
	afterSvcs := after["services"].(map[string]any)
	beforeImage := beforeSvcs["web"].(map[string]any)["image"]
	afterImage := afterSvcs["web"].(map[string]any)["image"]
	if beforeImage != "nginx:1" {
		t.Errorf("Before image = %v, want nginx:1", beforeImage)
	}
	if afterImage != "nginx:2" {
		t.Errorf("After image = %v, want nginx:2", afterImage)
	}
}

func TestAuditEnvChanged(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing .env so before-keys are non-empty.
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("OLD_KEY=oldval\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertApp(&store.App{Name: "envapp", Slug: "envapp", ComposePath: composePath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	req := authedRequest(t, http.MethodPut, "/api/apps/envapp/env",
		[]map[string]string{{"key": "NEW_KEY", "value": "supersecret"}},
		cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("env status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	e := findFullAuditEntry(t, s, "env", "changed")
	if e == nil {
		t.Fatal("no env/changed audit row found")
	}
	if e.AppSlug != "envapp" {
		t.Errorf("app_slug = %q, want envapp", e.AppSlug)
	}
	// Ensure secret value is NOT stored in audit JSON.
	if e.AfterJSON != nil && strings.Contains(string(e.AfterJSON), "supersecret") {
		t.Errorf("env audit leaked value: %s", e.AfterJSON)
	}
	// AfterJSON should contain the new key name.
	if e.AfterJSON == nil || !strings.Contains(string(e.AfterJSON), "NEW_KEY") {
		t.Errorf("AfterJSON missing key: %s", e.AfterJSON)
	}
}

func TestAuditAccessIPListChanged(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertApp(&store.App{Name: "ipapp", Slug: "ipapp", ComposePath: composePath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	req := authedRequest(t, http.MethodPut, "/api/apps/ipapp/access",
		map[string]string{"allow": "10.0.0.0/8,192.168.1.5"}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("access status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	e := findAuditEntry(t, s, "access", "iplist_changed")
	if e == nil {
		t.Fatal("no access/iplist_changed audit row found")
	}
	if e.AppSlug != "ipapp" {
		t.Errorf("app_slug = %q, want ipapp", e.AppSlug)
	}
}

func TestAuditCertUploaded(t *testing.T) {
	srv, s, cookie := newAuditTestServer(t)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertApp(&store.App{Name: "certapp", Slug: "certapp", ComposePath: composePath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	// Generate a self-signed cert + key for the test.
	certPEM, keyPEM := genTestCertPEM(t)

	req := authedRequest(t, http.MethodPut, "/api/apps/certapp/certs/foo.example.com",
		map[string]string{"cert": certPEM, "key": keyPEM}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cert status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	e := findFullAuditEntry(t, s, "endpoint", "cert_uploaded")
	if e == nil {
		t.Fatal("no endpoint/cert_uploaded audit row found")
	}
	if e.AppSlug != "certapp" {
		t.Errorf("app_slug = %q, want certapp", e.AppSlug)
	}
	// Verify cert/key bodies are NOT logged.
	if e.AfterJSON != nil {
		body := string(e.AfterJSON)
		if strings.Contains(body, "BEGIN CERTIFICATE") || strings.Contains(body, "BEGIN PRIVATE KEY") || strings.Contains(body, "BEGIN RSA") {
			t.Errorf("audit leaked cert/key payload: %s", body)
		}
	}
}

func genTestCertPEM(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "foo.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certBuf := &bytes.Buffer{}
	pem.Encode(certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBuf := &bytes.Buffer{}
	pem.Encode(keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return certBuf.String(), keyBuf.String()
}

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

	e := findAuditEntry(t, s, "lifecycle", "purged")
	if e == nil {
		t.Fatal("no lifecycle/purged audit row found")
	}
	if e.AppSlug != "rmapp" {
		t.Errorf("app_slug = %q, want rmapp", e.AppSlug)
	}
}
