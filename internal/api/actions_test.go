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
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

type mockReconcilerFull struct {
	calls []string
}

func (m *mockReconcilerFull) DeployOne(_ context.Context, _, name string) error {
	m.calls = append(m.calls, "DeployOne:"+name)
	return nil
}
func (m *mockReconcilerFull) RemoveOne(_ context.Context, name string) error {
	m.calls = append(m.calls, "RemoveOne:"+name)
	return nil
}
func (m *mockReconcilerFull) RestartOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "RestartOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) StopOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "StopOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) StartOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "StartOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) PullOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "PullOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) ScaleOne(_ context.Context, slug string, _ map[string]int) error {
	m.calls = append(m.calls, "ScaleOne:"+slug)
	return nil
}

func (m *mockReconcilerFull) AppServices(_ context.Context, slug string) ([]deployer.ServiceStatus, error) {
	m.calls = append(m.calls, "AppServices:"+slug)
	return []deployer.ServiceStatus{{Service: "web", State: "running", Health: "healthy"}}, nil
}
func (m *mockReconcilerFull) AppConfig(_ string) (*compose.AppConfig, error) { return nil, nil }
func (m *mockReconcilerFull) RollbackOne(_ context.Context, slug string, versionID int64) error {
	m.calls = append(m.calls, fmt.Sprintf("RollbackOne:%s:%d", slug, versionID))
	return nil
}
func (m *mockReconcilerFull) ListVersions(_ context.Context, slug string) ([]store.ComposeVersion, error) {
	m.calls = append(m.calls, "ListVersions:"+slug)
	return []store.ComposeVersion{}, nil
}
func (m *mockReconcilerFull) ListDeployEvents(_ context.Context, slug string) ([]store.DeployEvent, error) {
	m.calls = append(m.calls, "ListDeployEvents:"+slug)
	return []store.DeployEvent{}, nil
}
func (m *mockReconcilerFull) Reconcile(_ context.Context) error     { return nil }
func (m *mockReconcilerFull) RefreshRoutes(_ context.Context) error { return nil }
func (m *mockReconcilerFull) CancelOne(_ context.Context, slug string) error {
	m.calls = append(m.calls, "CancelOne:"+slug)
	return nil
}
func (m *mockReconcilerFull) IsDeploying(_ string) bool { return false }
func (m *mockReconcilerFull) SubscribeDeployLog(_ string) (<-chan deployer.OutputLine, func(), bool) {
	return nil, nil, false
}

func newActionTestServer(t *testing.T) (*Server, *mockReconcilerFull) {
	t.Helper()
	srv, _ := newTestServer(t)
	mock := &mockReconcilerFull{}
	srv.SetReconciler(mock)
	srv.SetAppsDir(t.TempDir())
	return srv, mock
}

func TestRestartEndpoint(t *testing.T) {
	srv, _ := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/restart", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
}

func TestStopEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/stop", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "StopOne:myapp" {
		t.Errorf("expected StopOne:myapp, got: %v", mock.calls)
	}
}

func TestStartEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/start", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) == 0 || mock.calls[0] != "StartOne:myapp" {
		t.Errorf("expected StartOne:myapp, got: %v", mock.calls)
	}
}

func TestPullEndpoint(t *testing.T) {
	srv, _ := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/pull", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
}

func TestScaleEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	body, _ := json.Marshal(map[string]any{"scales": map[string]int{"web": 3}})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	found := false
	for _, c := range mock.calls {
		if c == "ScaleOne:myapp" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ScaleOne:myapp in calls, got: %v", mock.calls)
	}
}

func TestRollbackEndpoint(t *testing.T) {
	srv, mock := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	verID := seedVersion(t, srv, "myapp")
	body, _ := json.Marshal(map[string]any{"version_id": verID})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/rollback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	want := fmt.Sprintf("RollbackOne:myapp:%d", verID)
	if len(mock.calls) == 0 || mock.calls[0] != want {
		t.Errorf("expected %s, got: %v", want, mock.calls)
	}
}

// seedVersion creates app slug in the store with one compose version and
// returns the version ID.
func seedVersion(t *testing.T, srv *Server, slug string) int64 {
	t.Helper()
	if err := srv.store.UpsertApp(&store.App{Name: slug, Slug: slug, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	app, err := srv.store.GetAppBySlug(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.store.CreateComposeVersion(app.ID, "services:\n  web:\n    image: nginx\n", "h-"+slug); err != nil {
		t.Fatal(err)
	}
	versions, err := srv.store.ListComposeVersions(app.ID)
	if err != nil || len(versions) == 0 {
		t.Fatalf("list versions: %v", err)
	}
	return versions[0].ID
}

func TestScaleEndpointMissingBody(t *testing.T) {
	srv, _ := newActionTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	body, _ := json.Marshal(map[string]any{"scales": map[string]int{}})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// blockingRunner is a deployer.CommandRunner that hands each compose call's
// context to the test and blocks until released. Like exec.CommandContext,
// it fails if that context is canceled.
type blockingRunner struct {
	ctxs    chan context.Context
	release chan struct{}
}

func (b *blockingRunner) Run(ctx context.Context, _ string, args ...string) (string, string, error) {
	if len(args) == 2 && args[1] == "version" {
		return "", "", nil
	}
	b.ctxs <- ctx
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return "", "", ctx.Err()
}

// runnerReconciler routes lifecycle actions through a real deployer so the
// context reaching the CommandRunner is the one the handler passed.
type runnerReconciler struct {
	mockReconcilerFull
	d           *deployer.Deployer
	runner      deployer.CommandRunner
	composePath string
}

func (m *runnerReconciler) StopOne(ctx context.Context, slug string) error {
	return m.d.Stop(ctx, slug)
}
func (m *runnerReconciler) StartOne(ctx context.Context, slug string) error {
	return m.d.Start(ctx, slug)
}
func (m *runnerReconciler) ScaleOne(ctx context.Context, slug string, scales map[string]int) error {
	return m.d.Scale(ctx, &compose.AppConfig{Name: slug, ComposePath: m.composePath}, scales)
}
func (m *runnerReconciler) RollbackOne(ctx context.Context, _ string, _ int64) error {
	_, _, err := m.runner.Run(ctx, "docker", "compose", "up", "-d")
	return err
}

func TestLifecycleActionsSurviveClientDisconnect(t *testing.T) {
	scale, _ := json.Marshal(map[string]any{"scales": map[string]int{"web": 2}})
	cases := []struct {
		name, path string
		body       []byte
		rollback   bool
	}{
		{"stop", "/api/apps/myapp/stop", nil, false},
		{"start", "/api/apps/myapp/start", nil, false},
		{"scale", "/api/apps/myapp/scale", scale, false},
		{"rollback", "/api/apps/myapp/rollback", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			srv.SetAppsDir(t.TempDir())
			runner := &blockingRunner{ctxs: make(chan context.Context, 1), release: make(chan struct{})}
			d, err := deployer.New(runner)
			if err != nil {
				t.Fatal(err)
			}
			composePath := filepath.Join(t.TempDir(), "docker-compose.yml")
			if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			srv.SetReconciler(&runnerReconciler{d: d, runner: runner, composePath: composePath})

			body := tc.body
			if tc.rollback {
				body, _ = json.Marshal(map[string]any{"version_id": seedVersion(t, srv, "myapp")})
			}
			reqCtx, closeTab := context.WithCancel(context.Background())
			defer closeTab()
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body)).WithContext(reqCtx)
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(superAdminCookie(t, srv.jwt))
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				srv.Handler().ServeHTTP(w, req)
				close(done)
			}()

			var opCtx context.Context
			select {
			case opCtx = <-runner.ctxs:
			case <-time.After(5 * time.Second):
				t.Fatal("compose command never ran")
			}
			closeTab()
			if err := opCtx.Err(); err != nil {
				t.Fatalf("compose command context canceled with the request: %v", err)
			}
			if _, ok := opCtx.Deadline(); !ok {
				t.Error("compose command context has no timeout")
			}
			close(runner.release)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not return")
			}
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
		})
	}
}
