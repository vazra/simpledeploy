package reconciler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/store"
)

func newTestEnv(t *testing.T) (*Reconciler, *docker.MockClient, *store.Store, string) {
	t.Helper()

	// temp SQLite DB
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// mock docker client and deployer
	mock := docker.NewMockClient()
	d := deployer.New(mock)

	// temp apps dir
	appsDir := t.TempDir()

	r := New(st, d, appsDir)
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
    labels:
      simpledeploy.domain: "` + appName + `.example.com"
      simpledeploy.port: "80"
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

	if !mock.HasCall("NetworkCreate:simpledeploy-myapp") {
		t.Error("expected NetworkCreate:simpledeploy-myapp")
	}
	if !mock.HasCall("ContainerCreate:simpledeploy-myapp-web") {
		t.Error("expected ContainerCreate:simpledeploy-myapp-web")
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

	if !mock.HasCall("NetworkRemove:simpledeploy-rmapp") {
		t.Error("expected NetworkRemove:simpledeploy-rmapp")
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

	if !mock.HasCall("NetworkCreate:simpledeploy-watched") {
		t.Error("expected NetworkCreate:simpledeploy-watched after watcher triggered reconcile")
	}
	if !mock.HasCall("ContainerCreate:simpledeploy-watched-web") {
		t.Error("expected ContainerCreate:simpledeploy-watched-web after watcher triggered reconcile")
	}
}
