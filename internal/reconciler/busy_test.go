package reconciler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

// newDeployerEnv is newTestEnv with a real deployer on a fake docker, so
// IsDeploying reads the same in-flight tracking as in production.
func newDeployerEnv(t *testing.T) (*Reconciler, *deployer.Deployer, *deployer.MockRunner, *store.Store, string) {
	t.Helper()
	r, _, st, appsDir := newTestEnv(t)
	runner := &deployer.MockRunner{}
	d, err := deployer.New(runner)
	if err != nil {
		t.Fatal(err)
	}
	r.deployer = d
	return r, d, runner, st, appsDir
}

// hold marks slug as having a deploy in flight until release is called.
func hold(t *testing.T, d *deployer.Deployer, slug string) (release func()) {
	t.Helper()
	if _, ok := d.Tracker.TrackWithLog(slug, func() {}); !ok {
		t.Fatalf("%s already in flight", slug)
	}
	return func() { d.Tracker.DoneWithLog(slug, "deploy") }
}

func TestRollbackWhileDeployingIsRefused(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, d, runner, st, appsDir := newDeployerEnv(t)
	_, ver := seedApp(t, st, appsDir, "app", "services:\n  web:\n    image: nginx:0.9\n")
	release := hold(t, d, "app")
	defer release()

	err := r.RollbackOne(context.Background(), "app", ver)
	if !errors.Is(err, deployer.ErrDeployInProgress) {
		t.Fatalf("err = %v, want ErrDeployInProgress", err)
	}
	if got := readCompose(t, appsDir, "app"); got != safeCompose {
		t.Errorf("compose was overwritten: %q", got)
	}
	if runner.HasCall("docker", "up") {
		t.Error("compose up ran during another deploy")
	}
}

// skipper reports tracked operations as skipped, as when another deploy
// takes the app between the reconciler's check and the deployer's.
type skipper struct{ *mockDeployer }

func (skipper) RollbackDeploy(context.Context, *compose.AppConfig, int, *int64, ...deployer.RegistryAuth) deployer.DeployResult {
	return deployer.DeployResult{Skipped: true}
}
func (skipper) Restart(context.Context, *compose.AppConfig) deployer.DeployResult {
	return deployer.DeployResult{Skipped: true}
}
func (skipper) Pull(context.Context, *compose.AppConfig, []deployer.RegistryAuth) deployer.DeployResult {
	return deployer.DeployResult{Skipped: true}
}

func TestRollbackSkippedRestoresComposeFile(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	r.deployer = skipper{mock}
	_, ver := seedApp(t, st, appsDir, "app", "services:\n  web:\n    image: nginx:0.9\n")

	err := r.RollbackOne(context.Background(), "app", ver)
	if !errors.Is(err, deployer.ErrDeployInProgress) {
		t.Fatalf("err = %v, want ErrDeployInProgress", err)
	}
	if got := readCompose(t, appsDir, "app"); got != safeCompose {
		t.Errorf("compose = %q, want the current file restored", got)
	}
	if evs, _ := st.ListDeployEvents("app"); len(evs) != 0 {
		t.Errorf("deploy events = %+v, want none for a rollback that never ran", evs)
	}
}

func TestRestartAndPullSkippedReportBusy(t *testing.T) {
	r, mock, st, appsDir := newTestEnv(t)
	r.deployer = skipper{mock}
	seedApp(t, st, appsDir, "app", safeCompose)

	if err := r.RestartOne(context.Background(), "app"); !errors.Is(err, deployer.ErrDeployInProgress) {
		t.Errorf("RestartOne err = %v, want ErrDeployInProgress", err)
	}
	if err := r.PullOne(context.Background(), "app"); !errors.Is(err, deployer.ErrDeployInProgress) {
		t.Errorf("PullOne err = %v, want ErrDeployInProgress", err)
	}
}

// A watcher reconcile skipped because another deploy of the app was running
// is retried once that deploy finishes, so the newer file gets deployed.
func TestSkippedReconcileRetriesAfterDeploy(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	old := retryPoll
	retryPoll = 10 * time.Millisecond
	t.Cleanup(func() { retryPoll = old })

	r, d, runner, st, appsDir := newDeployerEnv(t)
	seedApp(t, st, appsDir, "app", safeCompose)
	release := hold(t, d, "app")
	if err := os.WriteFile(filepath.Join(appsDir, "app", "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx:2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if runner.HasCall("docker", "up") {
		t.Fatal("compose up ran while another deploy was in flight")
	}

	release()
	deadline := time.Now().Add(5 * time.Second)
	for !runner.HasCall("docker", "up") {
		if time.Now().After(deadline) {
			t.Fatal("skipped reconcile was not retried after the running deploy finished")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
