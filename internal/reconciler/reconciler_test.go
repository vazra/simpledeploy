package reconciler

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/store"
)

type mockDeployer struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockDeployer) Deploy(_ context.Context, app *compose.AppConfig, _ ...deployer.RegistryAuth) deployer.DeployResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Deploy:"+app.Name)
	return deployer.DeployResult{}
}

func (m *mockDeployer) RollbackDeploy(_ context.Context, app *compose.AppConfig, _ int, _ *int64, _ ...deployer.RegistryAuth) deployer.DeployResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "RollbackDeploy:"+app.Name)
	return deployer.DeployResult{}
}

func (m *mockDeployer) Teardown(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Teardown:"+name)
	return nil
}

func (m *mockDeployer) Restart(_ context.Context, app *compose.AppConfig) deployer.DeployResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Restart:"+app.Name)
	return deployer.DeployResult{}
}

func (m *mockDeployer) Stop(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Stop:"+name)
	return nil
}

func (m *mockDeployer) Start(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Start:"+name)
	return nil
}

func (m *mockDeployer) Pull(_ context.Context, app *compose.AppConfig, _ []deployer.RegistryAuth) deployer.DeployResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Pull:"+app.Name)
	return deployer.DeployResult{}
}

func (m *mockDeployer) Scale(_ context.Context, app *compose.AppConfig, _ map[string]int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Scale:"+app.Name)
	return nil
}

func (m *mockDeployer) Status(_ context.Context, name string) ([]deployer.ServiceStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Status:"+name)
	return nil, nil
}

func (m *mockDeployer) Cancel(_ context.Context, app *compose.AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Cancel:"+app.Name)
	return nil
}

func (m *mockDeployer) hasCall(prefix string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.calls {
		if c == prefix {
			return true
		}
	}
	return false
}

func newTestEnv(t *testing.T) (*Reconciler, *mockDeployer, *store.Store, string) {
	t.Helper()

	// temp SQLite DB
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	mock := &mockDeployer{}

	// temp apps dir
	appsDir := t.TempDir()

	mockProxy := proxy.NewMockProxy()
	r := New(st, mock, mockProxy, appsDir, nil, nil)
	return r, mock, st, appsDir
}

func writeComposeFile(t *testing.T, dir, appName string) {
	t.Helper()
	appDir := filepath.Join(dir, appName)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", appDir, err)
	}
	content := `services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    labels:
      simpledeploy.endpoints.0.domain: "` + appName + `.example.com"
      simpledeploy.endpoints.0.port: "80"
`
	path := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
}

func TestReconcileNewApp(t *testing.T) {
	r, mock, st, appsDir := newTestEnv(t)
	ctx := context.Background()

	writeComposeFile(t, appsDir, "myapp")

	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if !mock.hasCall("Deploy:myapp") {
		t.Error("expected Deploy:myapp")
	}

	app, err := st.GetAppBySlug("myapp")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	if app.Status != "running" {
		t.Errorf("expected status running, got %s", app.Status)
	}
}

func TestReconcileRemoveApp(t *testing.T) {
	r, mock, st, appsDir := newTestEnv(t)
	ctx := context.Background()

	writeComposeFile(t, appsDir, "rmapp")

	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	// verify it was deployed
	if _, err := st.GetAppBySlug("rmapp"); err != nil {
		t.Fatalf("app not in store after first reconcile: %v", err)
	}

	// remove the directory
	if err := os.RemoveAll(filepath.Join(appsDir, "rmapp")); err != nil {
		t.Fatalf("remove dir: %v", err)
	}

	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	if !mock.hasCall("Teardown:rmapp") {
		t.Error("expected Teardown:rmapp")
	}

	if _, err := st.GetAppBySlug("rmapp"); err == nil {
		t.Error("expected app deleted from store")
	}
}

func TestReconcileMultipleApps(t *testing.T) {
	r, _, st, appsDir := newTestEnv(t)
	ctx := context.Background()

	writeComposeFile(t, appsDir, "alpha")
	writeComposeFile(t, appsDir, "beta")

	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	apps, err := st.ListApps()
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 2 {
		t.Errorf("expected 2 apps in store, got %d", len(apps))
	}

	slugs := make(map[string]bool, len(apps))
	for _, a := range apps {
		slugs[a.Slug] = true
	}
	if !slugs["alpha"] {
		t.Error("expected alpha in store")
	}
	if !slugs["beta"] {
		t.Error("expected beta in store")
	}
}

func TestReconcileUpdatesProxyRoutes(t *testing.T) {
	r, _, _, appsDir := newTestEnv(t)

	writeComposeFile(t, appsDir, "myapp")
	r.Reconcile(context.Background())

	mockProxy := r.proxy.(*proxy.MockProxy)
	if !mockProxy.HasRoute("myapp.example.com") {
		t.Error("expected proxy route for myapp.example.com")
	}
}

func TestReconcileRemoveUpdatesProxy(t *testing.T) {
	r, _, _, appsDir := newTestEnv(t)

	writeComposeFile(t, appsDir, "myapp")
	r.Reconcile(context.Background())

	os.RemoveAll(filepath.Join(appsDir, "myapp"))
	r.Reconcile(context.Background())

	mockProxy := r.proxy.(*proxy.MockProxy)
	if mockProxy.HasRoute("myapp.example.com") {
		t.Error("expected proxy route removed")
	}
}

func TestReconcileRedeploysOnChange(t *testing.T) {
	r, mock, _, appsDir := newTestEnv(t)
	ctx := context.Background()

	writeComposeFile(t, appsDir, "myapp")
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if !mock.hasCall("Deploy:myapp") {
		t.Fatal("expected initial Deploy:myapp")
	}

	composePath := filepath.Join(appsDir, "myapp", "docker-compose.yml")
	os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx:latest\n    ports:\n      - \"8080:80\"\n"), 0644)

	mock.mu.Lock()
	mock.calls = nil
	mock.mu.Unlock()

	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if !mock.hasCall("Deploy:myapp") {
		t.Error("expected redeploy after compose change")
	}
}

func TestReconcileSkipsUnchanged(t *testing.T) {
	r, mock, _, appsDir := newTestEnv(t)
	ctx := context.Background()

	writeComposeFile(t, appsDir, "myapp")
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	mock.mu.Lock()
	mock.calls = nil
	mock.mu.Unlock()

	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if mock.hasCall("Deploy:myapp") {
		t.Error("should NOT redeploy unchanged app")
	}
}

func TestWatcherTriggersReconcile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	r, mock, _, appsDir := newTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- r.Watch(ctx)
	}()

	// give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// write compose file to trigger watcher
	writeComposeFile(t, appsDir, "watched")

	// wait for debounce + some margin
	time.Sleep(2 * time.Second)

	cancel()
	<-done

	if !mock.hasCall("Deploy:watched") {
		t.Error("expected Deploy:watched after watcher triggered reconcile")
	}
}

func TestClassifyStatus(t *testing.T) {
	cases := []struct {
		name string
		svcs []deployer.ServiceStatus
		want string
	}{
		{"no services -> blank", nil, ""},
		{"all running healthy", []deployer.ServiceStatus{
			{Service: "web", State: "running", Health: "healthy"},
			{Service: "db", State: "running"},
		}, "running"},
		{"any restarting", []deployer.ServiceStatus{
			{Service: "web", State: "restarting"},
			{Service: "db", State: "running"},
		}, "unstable"},
		{"any unhealthy", []deployer.ServiceStatus{
			{Service: "web", State: "running", Health: "unhealthy"},
		}, "unstable"},
		{"running + exited -> degraded", []deployer.ServiceStatus{
			{Service: "web", State: "running"},
			{Service: "worker", State: "exited"},
		}, "degraded"},
		{"all exited -> stopped", []deployer.ServiceStatus{
			{Service: "web", State: "exited"},
		}, "stopped"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyStatus(c.svcs)
			if got != c.want {
				t.Errorf("classifyStatus(%+v) = %q, want %q", c.svcs, got, c.want)
			}
		})
	}
}
