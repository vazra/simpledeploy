package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

// captureCtxReconciler embeds mockReconciler and captures the context passed
// to Reconcile, blocking until released so tests can inspect cancellation.
type captureCtxReconciler struct {
	mockReconciler
	gotCtx  chan context.Context
	release chan struct{}
}

func (c *captureCtxReconciler) Reconcile(ctx context.Context) error {
	c.gotCtx <- ctx
	<-c.release
	return nil
}

func (c *captureCtxReconciler) RefreshRoutes(ctx context.Context) error {
	c.gotCtx <- ctx
	<-c.release
	return nil
}

func TestHandleUpdateEndpoints_ReconcileCtxNotCancelled(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	s.UpsertApp(&store.App{Name: "ctxapp", Slug: "ctxapp", ComposePath: composePath, Status: "running"}, nil)

	rec := &captureCtxReconciler{
		gotCtx:  make(chan context.Context, 1),
		release: make(chan struct{}),
	}
	srv.SetReconciler(rec)

	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	endpoints := []compose.EndpointConfig{
		{Domain: "x.example.com", Port: "80", TLS: "letsencrypt", Service: "web"},
	}
	body, _ := json.Marshal(endpoints)
	req, _ := http.NewRequest(http.MethodPut, httpSrv.URL+"/api/apps/ctxapp/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	var ctx context.Context
	select {
	case ctx = <-rec.gotCtx:
	case <-time.After(2 * time.Second):
		t.Fatal("Reconcile not invoked within 2s")
	}

	// Give any cancellation propagation time to land before checking.
	time.Sleep(50 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		t.Errorf("Reconcile context cancelled after request returned: %v", err)
	}
	close(rec.release)
}

func TestHandleUpdateEndpoints(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.endpoints.0.domain: old.example.com
      simpledeploy.endpoints.0.port: "80"
      simpledeploy.endpoints.0.tls: letsencrypt
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: composePath, Status: "running"}, nil)

	endpoints := []compose.EndpointConfig{
		{Domain: "new.example.com", Port: "80", TLS: "letsencrypt", Service: "web"},
	}
	body, _ := json.Marshal(endpoints)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/myapp/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(updated), "new.example.com") {
		t.Errorf("expected new.example.com in compose, got:\n%s", string(updated))
	}
	if strings.Contains(string(updated), "old.example.com") {
		t.Errorf("old.example.com still present in compose")
	}
}

func TestHandleUpdateEndpoints_NoExistingLabel(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "noapp", Slug: "noapp", ComposePath: composePath, Status: "stopped"}, nil)

	endpoints := []compose.EndpointConfig{
		{Domain: "brand.new.com", Port: "3000", TLS: "letsencrypt", Service: "web"},
	}
	body, _ := json.Marshal(endpoints)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/noapp/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(updated), "brand.new.com") {
		t.Errorf("expected brand.new.com in compose, got:\n%s", string(updated))
	}
	if !strings.Contains(string(updated), "simpledeploy.endpoints.0.domain") {
		t.Errorf("expected endpoint label in compose, got:\n%s", string(updated))
	}
}

func TestHandleUpdateEndpoints_MultiService(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
  api:
    image: node
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "multi", Slug: "multi", ComposePath: composePath, Status: "running"}, nil)

	endpoints := []compose.EndpointConfig{
		{Domain: "web.example.com", Port: "80", TLS: "letsencrypt", Service: "web"},
		{Domain: "api.example.com", Port: "8080", TLS: "custom", Service: "api"},
	}
	body, _ := json.Marshal(endpoints)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/multi/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	result := string(updated)
	if !strings.Contains(result, "web.example.com") {
		t.Errorf("expected web.example.com, got:\n%s", result)
	}
	if !strings.Contains(result, "api.example.com") {
		t.Errorf("expected api.example.com, got:\n%s", result)
	}
}

func TestHandleUpdateEndpoints_ProtocolAndPath(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  co:\n    image: example/co\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	s.UpsertApp(&store.App{Name: "grpcapp", Slug: "grpcapp", ComposePath: composePath, Status: "running"}, nil)

	endpoints := []compose.EndpointConfig{
		{Domain: "co.example.com", Port: "50051", TLS: "letsencrypt", Service: "co", Protocol: "GRPC"},
		{Domain: "co.example.com", Port: "8000", TLS: "letsencrypt", Service: "co", Path: "/ws*"},
		{Domain: "co.example.com", Port: "8001", TLS: "letsencrypt", Service: "co"},
	}
	body, _ := json.Marshal(endpoints)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/grpcapp/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	parsed, err := compose.ParseFile(composePath, "grpcapp")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(parsed.Endpoints) != 3 {
		t.Fatalf("endpoints = %d, want 3", len(parsed.Endpoints))
	}
	byPort := map[string]compose.EndpointConfig{}
	for _, ep := range parsed.Endpoints {
		byPort[ep.Port] = ep
	}
	if byPort["50051"].Protocol != "grpc" {
		t.Errorf("50051 protocol = %q, want grpc", byPort["50051"].Protocol)
	}
	if byPort["8000"].Path != "/ws*" {
		t.Errorf("8000 path = %q, want /ws*", byPort["8000"].Path)
	}
	if byPort["8001"].Protocol != "" || byPort["8001"].Path != "" {
		t.Errorf("8001 = %+v, want catch-all", byPort["8001"])
	}
}

func TestHandleUpdateEndpoints_RejectsBadProtocolPathAndCollisions(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	s.UpsertApp(&store.App{Name: "badapp", Slug: "badapp", ComposePath: composePath, Status: "running"}, nil)

	cases := []struct {
		name string
		eps  []compose.EndpointConfig
		want string
	}{
		{"protocol", []compose.EndpointConfig{{Domain: "a.example.com", Port: "80", Service: "web", Protocol: "tcp"}}, "invalid protocol"},
		{"path", []compose.EndpointConfig{{Domain: "a.example.com", Port: "80", Service: "web", Path: "ws"}}, "invalid path"},
		{"collision", []compose.EndpointConfig{
			{Domain: "a.example.com", Port: "80", Service: "web"},
			{Domain: "a.example.com", Port: "81", Service: "web", Protocol: "h2c"},
		}, "duplicate domain"},
		{"tls conflict", []compose.EndpointConfig{
			{Domain: "a.example.com", Port: "80", Service: "web", TLS: "letsencrypt"},
			{Domain: "a.example.com", Port: "81", Service: "web", Path: "/ws*", TLS: "off"},
		}, "endpoint a.example.com/ws*: tls \"off\" conflicts with tls \"auto\" of endpoint a.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.eps)
			req := httptest.NewRequest(http.MethodPut, "/api/apps/badapp/endpoints", bytes.NewReader(body))
			req.AddCookie(cookie)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("body = %q, want substring %q", w.Body.String(), tc.want)
			}
		})
	}
}

// writeEndpointApp writes a compose file with the given endpoint domains on
// service "web" and registers the app.
func writeEndpointApp(t *testing.T, s *store.Store, slug string, domains ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("services:\n  web:\n    image: nginx\n")
	if len(domains) > 0 {
		b.WriteString("    labels:\n")
	}
	for i, d := range domains {
		fmt.Fprintf(&b, "      simpledeploy.endpoints.%d.domain: %q\n      simpledeploy.endpoints.%d.port: \"80\"\n", i, d, i)
		if i > 0 {
			fmt.Fprintf(&b, "      simpledeploy.endpoints.%d.path: \"/p%d*\"\n", i, i)
		}
	}
	composePath := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertApp(&store.App{Name: slug, Slug: slug, ComposePath: composePath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	return composePath
}

func putEndpoints(t *testing.T, srv *Server, slug string, cookie *http.Cookie, eps []compose.EndpointConfig) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(eps)
	req := httptest.NewRequest(http.MethodPut, "/api/apps/"+slug+"/endpoints", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestHandleUpdateEndpoints_RejectsDomainOfOtherApp(t *testing.T) {
	srv, s := newTestServer(t)
	writeEndpointApp(t, s, "other", "taken.example.com")
	minePath := writeEndpointApp(t, s, "mine")
	before, _ := os.ReadFile(minePath)

	// Case and trailing dot do not matter.
	w := putEndpoints(t, srv, "mine", superAdminCookie(t, srv.jwt), []compose.EndpointConfig{
		{Domain: "free.example.com", Port: "80", Service: "web"},
		{Domain: "TAKEN.example.com.", Port: "80", Service: "web"},
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `domain TAKEN.example.com. is already used by app "other"`) {
		t.Errorf("super_admin body = %q, want conflicting app slug", w.Body.String())
	}
	if after, _ := os.ReadFile(minePath); string(after) != string(before) {
		t.Error("compose file changed despite conflict")
	}

	// Non-super_admin: no other app details.
	app, _ := s.GetAppBySlug("mine")
	cookie := makeUserCookie(t, srv, s, "mgr", app.ID)
	w = putEndpoints(t, srv, "mine", cookie, []compose.EndpointConfig{{Domain: "taken.example.com", Port: "80", Service: "web"}})
	if w.Code != http.StatusConflict {
		t.Fatalf("manage status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "already used by another app") || strings.Contains(body, "other\"") {
		t.Errorf("manage body = %q, want generic message without slug", body)
	}
}

func TestHandleUpdateEndpoints_StoredDomainFallback(t *testing.T) {
	srv, s := newTestServer(t)
	// Compose file unreadable: the stored primary domain still counts.
	if err := s.UpsertApp(&store.App{Name: "legacy", Slug: "legacy", ComposePath: filepath.Join(t.TempDir(), "missing.yml"), Status: "running", Domain: "legacy.example.com"}, nil); err != nil {
		t.Fatal(err)
	}
	writeEndpointApp(t, s, "mine")
	w := putEndpoints(t, srv, "mine", superAdminCookie(t, srv.jwt), []compose.EndpointConfig{{Domain: "Legacy.example.com", Port: "80", Service: "web"}})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateEndpoints_SameAppSharedDomainStillWorks(t *testing.T) {
	srv, s := newTestServer(t)
	writeEndpointApp(t, s, "other", "other.example.com")
	writeEndpointApp(t, s, "mine", "mine.example.com", "mine.example.com")
	w := putEndpoints(t, srv, "mine", superAdminCookie(t, srv.jwt), []compose.EndpointConfig{
		{Domain: "mine.example.com", Port: "80", Service: "web"},
		{Domain: "Mine.example.com", Port: "81", Service: "web", Path: "/api*"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateEndpoints_RejectsReservedDomain(t *testing.T) {
	srv, s := newTestServer(t)
	writeEndpointApp(t, s, "mine")
	eps := []compose.EndpointConfig{{Domain: "manage.example.com.", Port: "80", Service: "web"}}
	srv.SetReservedDomains("", "Manage.Example.com")

	// manage users may not claim the dashboard domain.
	manage := loginAs(t, srv, s, "mgr", "password1", "manage")
	u, _ := s.GetUserByUsername("mgr")
	app, _ := s.GetAppBySlug("mine")
	if err := s.GrantAppAccess(u.ID, app.ID); err != nil {
		t.Fatal(err)
	}
	w := putEndpoints(t, srv, "mine", manage, eps)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "reserved for the SimpleDeploy dashboard") {
		t.Fatalf("manage: status = %d body = %q, want 409 reserved", w.Code, w.Body.String())
	}

	// super_admin may route it through an app to expose the dashboard.
	if w := putEndpoints(t, srv, "mine", superAdminCookie(t, srv.jwt), eps); w.Code != http.StatusOK {
		t.Fatalf("super_admin: status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

// Label edits on a compose file that cannot be parsed are refused instead
// of rewriting a file nobody checked.
func TestEndpointAndAccessEditsRefusedWhenComposeUnparsable(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"invalid yaml": func(t *testing.T, dir string) {
			os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: [\n"), 0o600)
		},
		"broken .env": func(t *testing.T, dir string) {
			os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600)
			os.WriteFile(filepath.Join(dir, ".env"), []byte("A=\"unclosed\n"), 0o600)
		},
		"symlinked compose": func(t *testing.T, dir string) {
			outside := filepath.Join(t.TempDir(), "compose.yml")
			os.WriteFile(outside, []byte("services:\n  web:\n    image: nginx\n"), 0o600)
			if err := os.Symlink(outside, filepath.Join(dir, "docker-compose.yml")); err != nil {
				t.Fatal(err)
			}
		},
		"missing compose": func(t *testing.T, dir string) {},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			srv, s := newTestServer(t)
			dir := t.TempDir()
			setup(t, dir)
			composePath := filepath.Join(dir, "docker-compose.yml")
			before, _ := os.ReadFile(composePath)
			if err := s.UpsertApp(&store.App{Name: "broken", Slug: "broken", ComposePath: composePath, Status: "running"}, nil); err != nil {
				t.Fatal(err)
			}
			w := putEndpoints(t, srv, "broken", superAdminCookie(t, srv.jwt),
				[]compose.EndpointConfig{{Domain: "b.example.com", Port: "80", Service: "web"}})
			if w.Code != http.StatusConflict {
				t.Fatalf("endpoints: status = %d, want 409; body: %s", w.Code, w.Body.String())
			}
			w = doJSON(t, srv, http.MethodPut, "/api/apps/broken/access", map[string]string{"allow": "10.0.0.1"})
			if w.Code != http.StatusConflict {
				t.Fatalf("access: status = %d, want 409; body: %s", w.Code, w.Body.String())
			}
			if after, _ := os.ReadFile(composePath); string(after) != string(before) {
				t.Fatalf("compose changed: %q", after)
			}
		})
	}
}

func TestEndpointAndAccessEditsWriteAtomically(t *testing.T) {
	srv, s := newTestServer(t)
	composePath := writeEndpointApp(t, s, "atom", "old.example.com")
	if err := os.Chmod(composePath, 0o644); err != nil {
		t.Fatal(err)
	}
	w := putEndpoints(t, srv, "atom", superAdminCookie(t, srv.jwt),
		[]compose.EndpointConfig{{Domain: "new.example.com", Port: "80", Service: "web"}})
	if w.Code != http.StatusOK {
		t.Fatalf("endpoints: status = %d; body: %s", w.Code, w.Body.String())
	}
	if fi, _ := os.Lstat(composePath); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode after endpoints = %v, want 0600", fi.Mode().Perm())
	}
	if err := os.Chmod(composePath, 0o644); err != nil {
		t.Fatal(err)
	}
	w = doJSON(t, srv, http.MethodPut, "/api/apps/atom/access", map[string]string{"allow": "10.0.0.1"})
	if w.Code != http.StatusOK {
		t.Fatalf("access: status = %d; body: %s", w.Code, w.Body.String())
	}
	if fi, _ := os.Lstat(composePath); !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode after access = %v, want regular 0600", fi.Mode())
	}
	entries, _ := os.ReadDir(filepath.Dir(composePath))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestEndpointErrorsNameTheDomain(t *testing.T) {
	srv, s := newTestServer(t)
	writeEndpointApp(t, s, "named")
	cookie := superAdminCookie(t, srv.jwt)
	cases := []struct {
		eps  []compose.EndpointConfig
		want string
	}{
		{[]compose.EndpointConfig{{Domain: "ok.example.com", Service: "web"}, {Domain: "", Service: "web"}}, "every endpoint needs a domain"},
		{[]compose.EndpointConfig{{Domain: "bad_domain!", Service: "web"}}, `invalid domain "bad_domain!"`},
		{[]compose.EndpointConfig{{Domain: "nosvc.example.com"}}, "endpoint nosvc.example.com: service is required"},
	}
	for _, tc := range cases {
		w := putEndpoints(t, srv, "named", cookie, tc.eps)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tc.want) {
			t.Errorf("status = %d body = %q, want 400 with %q", w.Code, w.Body.String(), tc.want)
		}
	}
}

func TestWildcardEndpointDomainsSuperAdminOnly(t *testing.T) {
	srv, s := newTestServer(t)
	writeEndpointApp(t, s, "wild", "wild.example.com")
	app, _ := s.GetAppBySlug("wild")
	manage := makeUserCookie(t, srv, s, "mgr", app.ID)
	wildcard := []compose.EndpointConfig{
		{Domain: "wild.example.com", Port: "80", Service: "web"},
		{Domain: "w*.example.com", Port: "80", Service: "web"},
	}

	w := putEndpoints(t, srv, "wild", manage, wildcard)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "wildcard") {
		t.Fatalf("manage: status = %d body = %q, want 409 wildcard", w.Code, w.Body.String())
	}
	if w := putEndpoints(t, srv, "wild", superAdminCookie(t, srv.jwt), wildcard); w.Code != http.StatusOK {
		t.Fatalf("super_admin: status = %d; body: %s", w.Code, w.Body.String())
	}
	// A wildcard a super_admin set up does not lock manage users out.
	wildcard[0].Port = "8080"
	if w := putEndpoints(t, srv, "wild", manage, wildcard); w.Code != http.StatusOK {
		t.Fatalf("manage keeping existing wildcard: status = %d; body: %s", w.Code, w.Body.String())
	}
}

// Domain ownership still sees an app whose .env is broken or whose compose
// file is a link.
func TestOtherAppDomainsParseFallbacks(t *testing.T) {
	srv, s := newTestServer(t)
	envPath := writeEndpointApp(t, s, "badenv", "badenv.example.com")
	os.WriteFile(filepath.Join(filepath.Dir(envPath), ".env"), []byte("A=\"unclosed\n"), 0o600)

	linkDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "compose.yml")
	os.WriteFile(target, []byte("services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: linked.example.com\n"), 0o600)
	linkPath := filepath.Join(linkDir, "docker-compose.yml")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertApp(&store.App{Name: "linked", Slug: "linked", ComposePath: linkPath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}

	writeEndpointApp(t, s, "mine")
	for _, domain := range []string{"badenv.example.com", "linked.example.com"} {
		w := putEndpoints(t, srv, "mine", superAdminCookie(t, srv.jwt), []compose.EndpointConfig{{Domain: domain, Port: "80", Service: "web"}})
		if w.Code != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409; body: %s", domain, w.Code, w.Body.String())
		}
	}
}
