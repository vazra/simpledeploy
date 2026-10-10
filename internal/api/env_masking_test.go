package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

// newEnvApp creates appsDir/<slug>/ with a .env and registers the app.
func newEnvApp(t *testing.T, srv *Server, st *store.Store, slug, env string) (appsDir, appDir string, appID int64) {
	t.Helper()
	appsDir = t.TempDir()
	srv.SetAppsDir(appsDir)
	appDir = filepath.Join(appsDir, slug)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if env != "" {
		if err := os.WriteFile(filepath.Join(appDir, ".env"), []byte(env), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := &store.App{Name: slug, Slug: slug, ComposePath: filepath.Join(appDir, "docker-compose.yml"), Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAppBySlug(slug)
	if err != nil {
		t.Fatal(err)
	}
	return appsDir, appDir, got.ID
}

func getEnvRaw(t *testing.T, srv *Server, slug string, cookie *http.Cookie) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/apps/"+slug+"/env", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func putEnv(t *testing.T, srv *Server, slug string, cookie *http.Cookie, vars any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(vars)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/"+slug+"/env", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestGetEnvViewerMasked(t *testing.T) {
	srv, st := newTestServer(t)
	_, _, appID := newEnvApp(t, srv, st, "maskapp", "DB_PASSWORD=hunter2\nAPI_KEY=abc123\n")
	viewer, _ := makeRoleUserCookie(t, srv, st, "viewer1", "viewer", appID)

	code, body := getEnvRaw(t, srv, "maskapp", viewer)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %s", code, body)
	}
	if strings.Contains(body, "hunter2") || strings.Contains(body, "abc123") {
		t.Fatalf("viewer received env values: %s", body)
	}
	var vars []map[string]any
	if err := json.Unmarshal([]byte(body), &vars); err != nil {
		t.Fatal(err)
	}
	if len(vars) != 2 {
		t.Fatalf("got %d vars, want 2", len(vars))
	}
	for i, k := range []string{"DB_PASSWORD", "API_KEY"} {
		v := vars[i]
		if v["key"] != k || v["value"] != "" || v["masked"] != true || len(v) != 3 {
			t.Errorf("var %d = %v, want {key:%s value:\"\" masked:true}", i, v, k)
		}
	}
}

func TestGetEnvManageAndSuperAdminUnmasked(t *testing.T) {
	srv, st := newTestServer(t)
	_, _, appID := newEnvApp(t, srv, st, "plainapp", "DB_PASSWORD=hunter2\n")
	manage, _ := makeRoleUserCookie(t, srv, st, "manager1", "manage", appID)

	for name, cookie := range map[string]*http.Cookie{"manage": manage, "super_admin": superAdminCookie(t, srv.jwt)} {
		code, body := getEnvRaw(t, srv, "plainapp", cookie)
		if code != http.StatusOK {
			t.Fatalf("%s: status = %d", name, code)
		}
		if body != `[{"key":"DB_PASSWORD","value":"hunter2"}]`+"\n" {
			t.Errorf("%s: body = %s", name, body)
		}
	}
}

func TestGetEnvSymlinkNotFollowed(t *testing.T) {
	srv, st := newTestServer(t)
	_, appDir, _ := newEnvApp(t, srv, st, "linkapp", "")
	outside := filepath.Join(t.TempDir(), "host-secrets")
	if err := os.WriteFile(outside, []byte("ROOT_TOKEN=leaked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(appDir, ".env")); err != nil {
		t.Fatal(err)
	}

	code, body := getEnvRaw(t, srv, "linkapp", superAdminCookie(t, srv.jwt))
	if code != http.StatusInternalServerError || strings.Contains(body, "leaked") {
		t.Fatalf("status = %d body = %q, want 500 without target content", code, body)
	}
	if !strings.Contains(body, "failed to read .env") {
		t.Errorf("body = %q", body)
	}
}

func TestGetEnvSymlinkedAppDirRefused(t *testing.T) {
	srv, st := newTestServer(t)
	appsDir := t.TempDir()
	srv.SetAppsDir(appsDir)
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, ".env"), []byte("ROOT_TOKEN=leaked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(appsDir, "dirlink")); err != nil {
		t.Fatal(err)
	}
	app := &store.App{Name: "dirlink", Slug: "dirlink", ComposePath: filepath.Join(appsDir, "dirlink", "docker-compose.yml"), Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}

	code, body := getEnvRaw(t, srv, "dirlink", superAdminCookie(t, srv.jwt))
	if code != http.StatusInternalServerError || strings.Contains(body, "leaked") {
		t.Fatalf("status = %d body = %q, want 500", code, body)
	}
}

func TestGetEnvNonRegularFile(t *testing.T) {
	srv, st := newTestServer(t)
	_, appDir, _ := newEnvApp(t, srv, st, "dirapp", "")
	if err := os.Mkdir(filepath.Join(appDir, ".env"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code, _ := getEnvRaw(t, srv, "dirapp", superAdminCookie(t, srv.jwt)); code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a directory .env", code)
	}
}

func TestPutEnvSymlinkNotFollowed(t *testing.T) {
	srv, st := newTestServer(t)
	_, appDir, _ := newEnvApp(t, srv, st, "linkput", "")
	outside := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(outside, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(appDir, ".env")); err != nil {
		t.Fatal(err)
	}

	w := putEnv(t, srv, "linkput", superAdminCookie(t, srv.jwt), []map[string]string{{"key": "X", "value": "pwned"}})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if got, _ := os.ReadFile(outside); string(got) != "original\n" {
		t.Fatalf("symlink target modified: %q", got)
	}
}

func TestPutEnvAtomicMode0600(t *testing.T) {
	srv, st := newTestServer(t)
	_, appDir, _ := newEnvApp(t, srv, st, "modeapp", "OLD=1\n")
	envPath := filepath.Join(appDir, ".env")
	if err := os.Chmod(envPath, 0o644); err != nil {
		t.Fatal(err)
	}

	w := putEnv(t, srv, "modeapp", superAdminCookie(t, srv.jwt), []map[string]string{{"key": "NEW", "value": "2"}})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", w.Code, w.Body.String())
	}
	fi, err := os.Lstat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want regular 0600", fi.Mode())
	}
	if got, _ := os.ReadFile(envPath); string(got) != "NEW=2\n" {
		t.Fatalf("content = %q", got)
	}
	entries, _ := os.ReadDir(appDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestPutEnvRejectsMaskedEntries(t *testing.T) {
	srv, st := newTestServer(t)
	_, appDir, _ := newEnvApp(t, srv, st, "maskput", "SECRET=keep\n")

	w := putEnv(t, srv, "maskput", superAdminCookie(t, srv.jwt), []map[string]any{{"key": "SECRET", "value": "", "masked": true}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got, _ := os.ReadFile(filepath.Join(appDir, ".env")); string(got) != "SECRET=keep\n" {
		t.Fatalf(".env changed: %q", got)
	}
}

// envComposeApp is newEnvApp plus a docker-compose.yml.
func envComposeApp(t *testing.T, srv *Server, st *store.Store, slug, composeYAML, env string) (appDir string, appID int64) {
	t.Helper()
	_, appDir, appID = newEnvApp(t, srv, st, slug, env)
	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte(composeYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	return appDir, appID
}

func TestPutEnvRefusesValuesThatBreakComposeRules(t *testing.T) {
	srv, st := newTestServer(t)
	appDir, _ := envComposeApp(t, srv, st, "privenv", privilegedViaEnv, "PRIV=false\n")

	w := putEnv(t, srv, "privenv", superAdminCookie(t, srv.jwt), []map[string]string{{"key": "PRIV", "value": "true"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error      string   `json:"error"`
		Violations []string `json:"violations"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" || len(resp.Violations) == 0 || !strings.Contains(resp.Violations[0], "privileged") {
		t.Fatalf("resp = %+v, want privileged violation", resp)
	}
	if got, _ := os.ReadFile(filepath.Join(appDir, ".env")); string(got) != "PRIV=false\n" {
		t.Fatalf(".env changed: %q", got)
	}

	// An unreadable value is refused the same way.
	w = putEnv(t, srv, "privenv", superAdminCookie(t, srv.jwt), []map[string]string{{"key": "A", "value": `"unclosed`}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "violations") {
		t.Fatalf("unclosed quote: status = %d body = %s, want 400 with violations", w.Code, w.Body.String())
	}

	if w := putEnv(t, srv, "privenv", superAdminCookie(t, srv.jwt), []map[string]string{{"key": "PRIV", "value": "false"}}); w.Code != http.StatusOK {
		t.Fatalf("safe value: status = %d; body: %s", w.Code, w.Body.String())
	}
}

// A compose file that already fails the checks is not blamed on the new
// values: the edit gets the same 409 as endpoint and access edits.
func TestPutEnvOnAlreadyUnsafeCompose(t *testing.T) {
	srv, st := newTestServer(t)
	appDir, _ := envComposeApp(t, srv, st, "unsafeenv", "services:\n  web:\n    image: nginx\n    privileged: true\n", "A=1\n")

	w := putEnv(t, srv, "unsafeenv", superAdminCookie(t, srv.jwt), []map[string]string{{"key": "A", "value": "2"}})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error      string   `json:"error"`
		Violations []string `json:"violations"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != unsafeComposeEdit || len(resp.Violations) == 0 || !strings.Contains(resp.Violations[0], "privileged") {
		t.Fatalf("resp = %+v, want unsafe compose 409", resp)
	}
	if got, _ := os.ReadFile(filepath.Join(appDir, ".env")); string(got) != "A=1\n" {
		t.Fatalf(".env changed: %q", got)
	}

	// An unusable current .env counts as empty for this check.
	os.WriteFile(filepath.Join(appDir, ".env"), []byte("A=\"unclosed\n"), 0o600)
	if w := putEnv(t, srv, "unsafeenv", superAdminCookie(t, srv.jwt), []map[string]string{{"key": "A", "value": "2"}}); w.Code != http.StatusConflict {
		t.Fatalf("broken .env: status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

// Values that make the compose file unsafe can be fixed through the same
// endpoint, and a broken .env can be replaced.
func TestPutEnvFixesUnsafeOrBrokenValues(t *testing.T) {
	srv, st := newTestServer(t)
	appDir, _ := envComposeApp(t, srv, st, "fixenv", privilegedViaEnv, "PRIV=true\n")
	cookie := superAdminCookie(t, srv.jwt)

	if w := putEnv(t, srv, "fixenv", cookie, []map[string]string{{"key": "PRIV", "value": "true"}, {"key": "B", "value": "1"}}); w.Code != http.StatusConflict {
		t.Fatalf("still unsafe: status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	if w := putEnv(t, srv, "fixenv", cookie, []map[string]string{{"key": "PRIV", "value": "false"}}); w.Code != http.StatusOK {
		t.Fatalf("fix: status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	os.WriteFile(filepath.Join(appDir, ".env"), []byte("A=\"unclosed\n"), 0o600)
	if w := putEnv(t, srv, "fixenv", cookie, []map[string]string{{"key": "PRIV", "value": "true"}}); w.Code != http.StatusBadRequest {
		t.Fatalf("unsafe value over broken .env: status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if w := putEnv(t, srv, "fixenv", cookie, []map[string]string{{"key": "A", "value": "ok"}}); w.Code != http.StatusOK {
		t.Fatalf("replace broken .env: status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

const domainViaEnv = "services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: ${DOMAIN:-mine.example.com}\n      simpledeploy.endpoints.0.port: \"80\"\n"

func TestPutEnvRefusesDomainOfOtherApp(t *testing.T) {
	srv, st := newTestServer(t)
	writeEndpointApp(t, st, "other", "taken.example.com")
	appDir, appID := envComposeApp(t, srv, st, "domenv", domainViaEnv, "")

	for _, cookie := range []*http.Cookie{superAdminCookie(t, srv.jwt), makeUserCookie(t, srv, st, "mgr", appID)} {
		w := putEnv(t, srv, "domenv", cookie, []map[string]string{{"key": "DOMAIN", "value": "Taken.example.com"}})
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already used") {
			t.Fatalf("status = %d body = %s, want 409 already used", w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(appDir, ".env")); !os.IsNotExist(err) {
		t.Fatalf(".env written despite conflict: %v", err)
	}
}

func TestPutEnvWildcardDomainSuperAdminOnly(t *testing.T) {
	srv, st := newTestServer(t)
	_, appID := envComposeApp(t, srv, st, "wildenv", domainViaEnv, "")
	manage := makeUserCookie(t, srv, st, "mgr", appID)
	vars := []map[string]string{{"key": "DOMAIN", "value": "*.example.com"}}

	w := putEnv(t, srv, "wildenv", manage, vars)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "wildcard") {
		t.Fatalf("manage: status = %d body = %s, want 409 wildcard", w.Code, w.Body.String())
	}
	if w := putEnv(t, srv, "wildenv", superAdminCookie(t, srv.jwt), vars); w.Code != http.StatusOK {
		t.Fatalf("super_admin: status = %d; body: %s", w.Code, w.Body.String())
	}
	// The wildcard is now the app's own domain: other edits still work.
	vars = append(vars, map[string]string{"key": "OTHER", "value": "1"})
	if w := putEnv(t, srv, "wildenv", manage, vars); w.Code != http.StatusOK {
		t.Fatalf("manage keeping wildcard: status = %d; body: %s", w.Code, w.Body.String())
	}
}
