package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/appbundle"
	"github.com/vazra/simpledeploy/internal/store"
)

func TestBundleExportRoundTrip(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	composeContent := "services:\n  web:\n    image: nginx\n"
	appDir := filepath.Join(appsDir, "exp-app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte(composeContent), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	if err := srv.store.UpsertApp(&store.App{
		Name: "Exp App", Slug: "exp-app",
		ComposePath: filepath.Join(appDir, "docker-compose.yml"),
		Status:      "running",
	}, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/apps/exp-app/export", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("content-type = %q, want application/zip", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd == "" {
		t.Errorf("missing Content-Disposition header")
	}

	bundle, err := appbundle.Parse(w.Body.Bytes())
	if err != nil {
		t.Fatalf("parse bundle: %v", err)
	}
	if string(bundle.Compose) != composeContent {
		t.Errorf("compose mismatch: got %q want %q", bundle.Compose, composeContent)
	}
	if bundle.Manifest.App.Slug != "exp-app" {
		t.Errorf("manifest slug = %q, want exp-app", bundle.Manifest.App.Slug)
	}
	if bundle.Manifest.SchemaVersion != appbundle.SchemaVersion {
		t.Errorf("schema_version = %d", bundle.Manifest.SchemaVersion)
	}
}

func buildTestBundle(t *testing.T, slug, displayName string) []byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	b, err := appbundle.Build(dir, slug, displayName, "test")
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	return b
}

func multipartImport(t *testing.T, zipBytes []byte, mode, slug string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if mode != "" {
		_ = mw.WriteField("mode", mode)
	}
	if slug != "" {
		_ = mw.WriteField("slug", slug)
	}
	if zipBytes != nil {
		fw, err := mw.CreateFormFile("file", "bundle.zip")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := io.Copy(fw, bytes.NewReader(zipBytes)); err != nil {
			t.Fatalf("copy zip: %v", err)
		}
	}
	mw.Close()
	return &body, mw.FormDataContentType()
}

func TestBundleImportNew(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	zipBytes := buildTestBundle(t, "imp-app", "Imp App")
	body, ct := multipartImport(t, zipBytes, "new", "imp-app")
	req := httptest.NewRequest(http.MethodPost, "/api/apps/import", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	if _, err := srv.store.GetAppBySlug("imp-app"); err != nil {
		t.Fatalf("expected app in store: %v", err)
	}
}

func TestBundleImportNewExistingSlugConflict(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	if err := srv.store.UpsertApp(&store.App{Name: "x", Slug: "imp-app", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	zipBytes := buildTestBundle(t, "imp-app", "")
	body, ct := multipartImport(t, zipBytes, "new", "imp-app")
	req := httptest.NewRequest(http.MethodPost, "/api/apps/import", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

func TestBundleImportBadZip(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	body, ct := multipartImport(t, []byte("not a zip file"), "new", "bad-app")
	req := httptest.NewRequest(http.MethodPost, "/api/apps/import", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestBundleImportPreviewOverwrite(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	// Existing app on disk + in store with different compose content.
	slug := "prv-app"
	appDir := filepath.Join(appsDir, slug)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte("services:\n  web:\n    image: caddy\n"), 0o644); err != nil {
		t.Fatalf("write existing compose: %v", err)
	}
	if err := srv.store.UpsertApp(&store.App{
		Name: "Prv", Slug: slug,
		ComposePath: filepath.Join(appDir, "docker-compose.yml"),
		Status:      "running",
	}, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	zipBytes := buildTestBundle(t, slug, "Prv")
	body, ct := multipartImport(t, zipBytes, "overwrite", slug)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/import/preview", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["mode"] != "overwrite" {
		t.Errorf("mode = %v", resp["mode"])
	}
	if resp["current"] == nil {
		t.Errorf("current should be present for overwrite")
	}
	changes, ok := resp["changes"].(map[string]any)
	if !ok {
		t.Fatalf("changes missing or wrong type: %v", resp["changes"])
	}
	if changes["compose_changed"] != true {
		t.Errorf("compose_changed = %v, want true", changes["compose_changed"])
	}

	// Verify import did not actually run: store row still exists with original compose on disk.
	cur, err := os.ReadFile(filepath.Join(appDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if !bytes.Contains(cur, []byte("caddy")) {
		t.Errorf("preview must not modify on-disk compose; got %s", cur)
	}
}

func TestBundleImportPreviewOverwriteMissing(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	zipBytes := buildTestBundle(t, "nope", "")
	body, ct := multipartImport(t, zipBytes, "overwrite", "nope")
	req := httptest.NewRequest(http.MethodPost, "/api/apps/import/preview", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestBundleImportInvalidSlug(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	zipBytes := buildTestBundle(t, "ok", "")
	body, ct := multipartImport(t, zipBytes, "new", "Bad Slug!")
	req := httptest.NewRequest(http.MethodPost, "/api/apps/import", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}
