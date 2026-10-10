package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

const privilegedViaEnv = "services:\n  web:\n    image: nginx\n    privileged: ${PRIV:-false}\n"

func doJSON(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func writeAppFile(t *testing.T, appsDir, slug, name, content string) string {
	t.Helper()
	dir := filepath.Join(appsDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func protectApps(t *testing.T, appsDir string) {
	t.Helper()
	compose.SetProtectedPaths("", appsDir)
	t.Cleanup(func() { compose.SetProtectedPaths("", "") })
}

// --- rollback / version ownership ---

type rollbackBlockedReconciler struct{ mockReconcilerFull }

func (m *rollbackBlockedReconciler) RollbackOne(_ context.Context, _ string, _ int64) error {
	return &compose.ViolationError{Violations: []string{`service "web": privileged mode not allowed`}}
}

func TestRollbackRejectsVersionOfAnotherApp(t *testing.T) {
	srv, mock := newActionTestServer(t)
	seedVersion(t, srv, "mine")
	otherVer := seedVersion(t, srv, "theirs")

	w := doJSON(t, srv, http.MethodPost, "/api/apps/mine/rollback", map[string]any{"version_id": otherVer})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) != 0 {
		t.Errorf("reconciler called: %v", mock.calls)
	}
}

func TestRollbackReportsViolations(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetAppsDir(t.TempDir())
	srv.SetReconciler(&rollbackBlockedReconciler{})
	ver := seedVersion(t, srv, "app")

	w := doJSON(t, srv, http.MethodPost, "/api/apps/app/rollback", map[string]any{"version_id": ver})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Violations []string `json:"violations"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Violations) != 1 || !strings.Contains(resp.Violations[0], "privileged") {
		t.Errorf("violations = %v", resp.Violations)
	}
}

// seedVersionContent is seedVersion with the given compose content.
func seedVersionContent(t *testing.T, srv *Server, slug, content string) int64 {
	t.Helper()
	if err := srv.store.UpsertApp(&store.App{Name: slug, Slug: slug, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	app, err := srv.store.GetAppBySlug(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.store.CreateComposeVersion(app.ID, content, "h-"+slug); err != nil {
		t.Fatal(err)
	}
	versions, err := srv.store.ListComposeVersions(app.ID)
	if err != nil || len(versions) == 0 {
		t.Fatalf("list versions: %v", err)
	}
	return versions[0].ID
}

func TestRollbackRefusesDomainOfOtherApp(t *testing.T) {
	srv, mock := newActionTestServer(t)
	writeEndpointApp(t, srv.store, "other", "taken.example.com")
	ver := seedVersionContent(t, srv, "app", "services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: taken.example.com\n")

	w := doJSON(t, srv, http.MethodPost, "/api/apps/app/rollback", map[string]any{"version_id": ver})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already used by app") {
		t.Fatalf("status = %d body = %s, want 409 already used", w.Code, w.Body.String())
	}
	if len(mock.calls) != 0 {
		t.Errorf("reconciler called: %v", mock.calls)
	}
}

func TestRollbackChecksVersionWithCurrentDotEnv(t *testing.T) {
	srv, mock := newActionTestServer(t)
	writeAppFile(t, srv.appsDir, "app", ".env", "PRIV=true\n")
	ver := seedVersionContent(t, srv, "app", privilegedViaEnv)

	w := doJSON(t, srv, http.MethodPost, "/api/apps/app/rollback", map[string]any{"version_id": ver})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error      string   `json:"error"`
		Violations []string `json:"violations"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != versionRefused || len(resp.Violations) == 0 || !strings.Contains(resp.Violations[0], "privileged") {
		t.Errorf("body = %+v, want privileged violation", resp)
	}
	if len(mock.calls) != 0 {
		t.Errorf("reconciler called: %v", mock.calls)
	}
}

func TestRollbackReportsInvalidVersion(t *testing.T) {
	srv, mock := newActionTestServer(t)
	ver := seedVersionContent(t, srv, "app", "services:\n  web: [\n")

	w := doJSON(t, srv, http.MethodPost, "/api/apps/app/rollback", map[string]any{"version_id": ver})
	var resp struct {
		Error      string   `json:"error"`
		Violations []string `json:"violations"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if w.Code != http.StatusBadRequest || resp.Error == "" || len(resp.Violations) == 0 {
		t.Fatalf("status = %d body = %+v, want 400 {error, violations}", w.Code, resp)
	}
	if len(mock.calls) != 0 {
		t.Errorf("reconciler called: %v", mock.calls)
	}
}

func TestDeleteVersionRejectsVersionOfAnotherApp(t *testing.T) {
	srv, _ := newActionTestServer(t)
	seedVersion(t, srv, "mine")
	otherVer := seedVersion(t, srv, "theirs")

	w := doJSON(t, srv, http.MethodDelete, fmt.Sprintf("/api/apps/mine/versions/%d", otherVer), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
	if _, err := srv.store.GetComposeVersion(otherVer); err != nil {
		t.Fatalf("other app's version was deleted: %v", err)
	}

	own := seedVersion(t, srv, "own")
	w = doJSON(t, srv, http.MethodDelete, fmt.Sprintf("/api/apps/own/versions/%d", own), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("own delete status = %d; body: %s", w.Code, w.Body.String())
	}
	if _, err := srv.store.GetComposeVersion(own); err == nil {
		t.Fatal("own version still exists")
	}
}

// --- deploy ---

func TestDeployValidatesWithAppDotEnv(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	writeAppFile(t, appsDir, "envapp", ".env", "PRIV=true\n")

	w := doJSON(t, srv, http.MethodPost, "/api/apps/deploy", map[string]any{
		"name": "envapp", "compose": base64.StdEncoding.EncodeToString([]byte(privilegedViaEnv)),
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "privileged") {
		t.Fatalf("status = %d body = %s, want 400 privileged violation", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(appsDir, "envapp", "docker-compose.yml")); !os.IsNotExist(err) {
		t.Error("refused compose was written to disk")
	}
}

func TestDeployRefusedRedeployKeepsCurrentCompose(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	current := "services:\n  web:\n    image: nginx:1\n"
	path := writeAppFile(t, appsDir, "live", "docker-compose.yml", current)
	if err := srv.store.UpsertApp(&store.App{Name: "live", Slug: "live", ComposePath: path, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	bad := "services:\n  web:\n    image: nginx\n    privileged: true\n"
	w := doJSON(t, srv, http.MethodPost, "/api/apps/deploy", map[string]any{
		"name": "live", "force": true, "compose": base64.StdEncoding.EncodeToString([]byte(bad)),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != current {
		t.Fatalf("current compose changed or removed: %q, %v", got, err)
	}
}

func TestDeployRejectsBindIntoAnotherApp(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	protectApps(t, appsDir)
	writeAppFile(t, appsDir, "victim", "docker-compose.yml", "services:\n  db:\n    image: postgres\n")

	c := "services:\n  web:\n    image: nginx\n    volumes:\n      - ../victim:/data\n"
	w := doJSON(t, srv, http.MethodPost, "/api/apps/deploy", map[string]any{
		"name": "thief", "compose": base64.StdEncoding.EncodeToString([]byte(c)),
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "another app") {
		t.Fatalf("status = %d body = %s, want 400 another app violation", w.Code, w.Body.String())
	}
}

func TestDeployRejectsSymlinkedDotEnv(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	dir := filepath.Join(appsDir, "linkenv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secrets.env")
	os.WriteFile(outside, []byte("A=1\n"), 0o600)
	if err := os.Symlink(outside, filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	w := doJSON(t, srv, http.MethodPost, "/api/apps/deploy", map[string]any{
		"name": "linkenv", "compose": base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n")),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestGetComposeRefusesSymlink(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	dir := filepath.Join(appsDir, "linked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "host-secret")
	os.WriteFile(outside, []byte("TOP-SECRET"), 0o600)
	if err := os.Symlink(outside, filepath.Join(dir, "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/apps/linked/compose", nil)
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "TOP-SECRET") {
		t.Fatalf("status = %d body = %q, want 409 without target content", w.Code, w.Body.String())
	}
}

// --- bundle export / import ---

func TestExportRefusesSymlinkedFiles(t *testing.T) {
	for _, name := range []string{"docker-compose.yml", ".env", "simpledeploy.yml"} {
		t.Run(name, func(t *testing.T) {
			srv, appsDir := newDeployTestServer(t)
			path := writeAppFile(t, appsDir, "exp", "docker-compose.yml", "services:\n  web:\n    image: nginx\n")
			if err := srv.store.UpsertApp(&store.App{Name: "exp", Slug: "exp", ComposePath: path, Status: "running"}, nil); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "host-secret")
			os.WriteFile(outside, []byte("TOP-SECRET=1\n"), 0o600)
			link := filepath.Join(appsDir, "exp", name)
			os.Remove(link)
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/apps/exp/export", nil)
			req.AddCookie(superAdminCookie(t, srv.jwt))
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "TOP-SECRET") {
				t.Fatalf("status = %d, want 409 without target content", w.Code)
			}
		})
	}
}

// rawBundle builds a bundle ZIP by hand so env.example can carry values
// (appbundle.Build redacts them).
func rawBundle(t *testing.T, composeYAML, envExample string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	add("manifest.json", `{"schema_version":1,"app":{"slug":"x"}}`)
	add("docker-compose.yml", composeYAML)
	if envExample != "" {
		add("env.example", envExample)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func postImport(t *testing.T, srv *Server, zipBytes []byte, mode, slug string) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartImport(t, zipBytes, mode, slug)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/import", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestImportNewValidatesWithBundleEnv(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	w := postImport(t, srv, rawBundle(t, privilegedViaEnv, "PRIV=true\n"), "new", "envimp")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "privileged") {
		t.Fatalf("status = %d body = %s, want 400 privileged violation", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(appsDir, "envimp")); !os.IsNotExist(err) {
		t.Error("refused import created the app folder")
	}
}

func TestImportOverwriteValidatesWithCurrentEnv(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	path := writeAppFile(t, appsDir, "ow", "docker-compose.yml", "services:\n  web:\n    image: nginx\n")
	writeAppFile(t, appsDir, "ow", ".env", "PRIV=true\n")
	if err := srv.store.UpsertApp(&store.App{Name: "ow", Slug: "ow", ComposePath: path, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	w := postImport(t, srv, rawBundle(t, privilegedViaEnv, ""), "overwrite", "ow")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "privileged") {
		t.Fatalf("status = %d body = %s, want 400 privileged violation", w.Code, w.Body.String())
	}
}

func TestImportResolvesPathsFromRealAppFolder(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	protectApps(t, appsDir)
	c := "services:\n  web:\n    image: nginx\n    volumes:\n      - ../victim/data:/data\n"
	w := postImport(t, srv, rawBundle(t, c, ""), "new", "thief")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "another app") {
		t.Fatalf("status = %d body = %s, want 400 another app violation", w.Code, w.Body.String())
	}
}

func TestImportPinsPublishedPortsToLoopback(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "")
	srv, appsDir := newDeployTestServer(t)
	c := "services:\n  web:\n    image: nginx\n    ports:\n      - \"8080:80\"\n"
	w := postImport(t, srv, rawBundle(t, c, ""), "new", "ports")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(appsDir, "ports", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "127.0.0.1:8080:80") {
		t.Errorf("ports not pinned to loopback:\n%s", got)
	}
}

func TestRestoreVersionRefusesUnsafeVersion(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	current := "services:\n  web:\n    image: nginx:1\n"
	path := writeAppFile(t, appsDir, "vapp", "docker-compose.yml", current)
	if err := srv.store.UpsertApp(&store.App{Name: "vapp", Slug: "vapp", ComposePath: path, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	app, _ := srv.store.GetAppBySlug("vapp")
	if err := srv.store.CreateComposeVersion(app.ID, "services:\n  web:\n    image: nginx\n    privileged: true\n", "h1"); err != nil {
		t.Fatal(err)
	}
	vers, _ := srv.store.ListComposeVersions(app.ID)
	w := doJSON(t, srv, http.MethodPost, fmt.Sprintf("/api/apps/vapp/versions/%d/restore", vers[0].ID), nil)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "privileged") {
		t.Fatalf("status = %d body = %s, want 400 privileged", w.Code, w.Body.String())
	}
	if got, _ := os.ReadFile(path); string(got) != current {
		t.Fatalf("compose changed: %q", got)
	}
}

// Label edits on an app whose compose fails validation would change its
// hash and drop the routes it keeps while unchanged, so they are refused.
func TestEndpointAndAccessEditsRefusedOnUnsafeCompose(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	legacy := "services:\n  web:\n    image: nginx\n    privileged: true\n"
	path := writeAppFile(t, appsDir, "legacy", "docker-compose.yml", legacy)
	if err := srv.store.UpsertApp(&store.App{Name: "legacy", Slug: "legacy", ComposePath: path, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	w := putEndpoints(t, srv, "legacy", superAdminCookie(t, srv.jwt),
		[]compose.EndpointConfig{{Domain: "legacy.example.com", Port: "80", Service: "web"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "privileged") {
		t.Fatalf("endpoints: status = %d body = %s, want 409 with violations", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPut, "/api/apps/legacy/access", map[string]string{"allow": "10.0.0.1"})
	if w.Code != http.StatusConflict {
		t.Fatalf("access: status = %d body = %s, want 409", w.Code, w.Body.String())
	}
	if got, _ := os.ReadFile(path); string(got) != legacy {
		t.Fatalf("compose changed: %q", got)
	}
}

func TestRestoreVersionRefusesDomainOfOtherApp(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	writeEndpointApp(t, srv.store, "other", "taken.example.com")
	current := "services:\n  web:\n    image: nginx:1\n"
	path := writeAppFile(t, appsDir, "vapp", "docker-compose.yml", current)
	if err := srv.store.UpsertApp(&store.App{Name: "vapp", Slug: "vapp", ComposePath: path, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	app, _ := srv.store.GetAppBySlug("vapp")
	old := "services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: taken.example.com\n"
	if err := srv.store.CreateComposeVersion(app.ID, old, "h1"); err != nil {
		t.Fatal(err)
	}
	vers, _ := srv.store.ListComposeVersions(app.ID)
	w := doJSON(t, srv, http.MethodPost, fmt.Sprintf("/api/apps/vapp/versions/%d/restore", vers[0].ID), nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already used by app") {
		t.Fatalf("status = %d body = %s, want 409 already used", w.Code, w.Body.String())
	}
	if got, _ := os.ReadFile(path); string(got) != current {
		t.Fatalf("compose changed: %q", got)
	}
}

func TestDeployReportsRefusedFileReferences(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	c := "services:\n  web:\n    image: nginx\n    label_file: /etc/passwd\n"
	w := doJSON(t, srv, http.MethodPost, "/api/apps/deploy", map[string]any{
		"name": "lf", "compose": base64.StdEncoding.EncodeToString([]byte(c)),
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "label_file") {
		t.Fatalf("status = %d body = %s, want 400 label_file violation", w.Code, w.Body.String())
	}
}

type rollbackDotEnvReconciler struct{ mockReconcilerFull }

func (m *rollbackDotEnvReconciler) RollbackOne(_ context.Context, _ string, _ int64) error {
	return fmt.Errorf("rollback: %w", fmt.Errorf("%w: unterminated quote", compose.ErrDotEnv))
}

func TestRollbackReportsDotEnvProblem(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetAppsDir(t.TempDir())
	srv.SetReconciler(&rollbackDotEnvReconciler{})
	ver := seedVersion(t, srv, "app")

	w := doJSON(t, srv, http.MethodPost, "/api/apps/app/rollback", map[string]any{"version_id": ver})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), dotEnvProblem) {
		t.Fatalf("status = %d body = %s, want 400 .env problem", w.Code, w.Body.String())
	}
}

// An app whose compose file is refused over a file reference still owns
// its domains (the display parse drops the references).
func TestDomainOwnershipSeesFileRefRefusedApp(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	outside := filepath.Join(t.TempDir(), "labels")
	os.WriteFile(outside, []byte("team=core\n"), 0o600)
	refs := "services:\n  web:\n    image: nginx\n    label_file: " + outside + "\n    labels:\n      simpledeploy.endpoints.0.domain: refs.example.com\n      simpledeploy.endpoints.0.port: \"80\"\n"
	path := writeAppFile(t, appsDir, "other", "docker-compose.yml", refs)
	if err := srv.store.UpsertApp(&store.App{Name: "other", Slug: "other", ComposePath: path, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	cfg, err := parseComposeForDisplay(path, "other")
	if err != nil || len(cfg.Endpoints) != 1 || cfg.Endpoints[0].Domain != "refs.example.com" {
		t.Fatalf("parseComposeForDisplay = %+v, %v; want refs.example.com", cfg, err)
	}
	minePath := writeAppFile(t, appsDir, "mine", "docker-compose.yml", "services:\n  web:\n    image: nginx\n")
	if err := srv.store.UpsertApp(&store.App{Name: "mine", Slug: "mine", ComposePath: minePath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	w := putEndpoints(t, srv, "mine", superAdminCookie(t, srv.jwt),
		[]compose.EndpointConfig{{Domain: "refs.example.com", Port: "80", Service: "web"}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `already used by app "other"`) {
		t.Fatalf("status = %d body = %s, want 409 already used by other", w.Code, w.Body.String())
	}
}
