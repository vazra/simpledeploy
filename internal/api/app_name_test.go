package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
)

var appNameCases = []string{
	"myapp", "my-app", "my_app", "app1", "1app", "a",
	"MyApp", "my.app", "MY-APP", "my app", "-app", "_app", ".app", "app!",
	strings.Repeat("a", 63), strings.Repeat("a", 64),
}

// validNewAppName must accept exactly the names docker compose accepts as
// the project name `simpledeploy-<name>` (compose-go's -p rule), capped at
// 63 characters.
func TestValidNewAppNameMatchesComposeProjectRule(t *testing.T) {
	for _, name := range appNameCases {
		project := "simpledeploy-" + name
		composeOK := name != "" && loader.NormalizeProjectName(project) == project &&
			loader.NormalizeProjectName(name) == name && len(name) <= 63
		if got := validNewAppName.MatchString(name); got != composeOK {
			t.Errorf("validNewAppName(%q) = %v, compose accepts = %v", name, got, composeOK)
		}
	}
}

func postDeploy(t *testing.T, srv *Server, name string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"name":    name,
		"compose": base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n")),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestDeployRejectsNamesComposeCannotUse(t *testing.T) {
	for _, name := range []string{"MyApp", "my.app", "MY-APP"} {
		srv, appsDir := newDeployTestServer(t)
		w := postDeploy(t, srv, name)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%q: status = %d, want 400; body: %s", name, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "lowercase letters, numbers, hyphens") {
			t.Errorf("%q: body = %q, want human-readable rule", name, w.Body.String())
		}
		if _, err := os.Stat(filepath.Join(appsDir, name)); !os.IsNotExist(err) {
			t.Errorf("%q: app dir created for rejected name", name)
		}
	}
	for _, name := range []string{"my-app", "my_app", "1app"} {
		srv, _ := newDeployTestServer(t)
		if w := postDeploy(t, srv, name); w.Code != http.StatusAccepted {
			t.Fatalf("%q: status = %d, want 202; body: %s", name, w.Code, w.Body.String())
		}
	}
}

// Apps whose folder name predates the stricter rule must stay removable.
func TestRemoveAppAcceptsLegacyName(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	appDir := filepath.Join(appsDir, "My.App")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/apps/My.App", nil)
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(appDir); !os.IsNotExist(err) {
		t.Error("legacy app dir should have been removed")
	}
}

func TestBundleImportRejectsNamesComposeCannotUse(t *testing.T) {
	for _, path := range []string{"/api/apps/import", "/api/apps/import/preview"} {
		srv, _ := newDeployTestServer(t)
		body, ct := multipartImport(t, buildTestBundle(t, "ok", ""), "new", "My.App")
		req := httptest.NewRequest(http.MethodPost, path, body)
		req.Header.Set("Content-Type", ct)
		req.AddCookie(superAdminCookie(t, srv.jwt))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400; body: %s", path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "lowercase letters, numbers, hyphens") {
			t.Errorf("%s: body = %q, want human-readable rule", path, w.Body.String())
		}
	}
}
