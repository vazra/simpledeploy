package deployer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

const privilegedCompose = "services:\n  web:\n    image: nginx\n    privileged: true\n"

func wantBlocked(t *testing.T, err error) {
	t.Helper()
	var ve *compose.ViolationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *compose.ViolationError, got %v", err)
	}
	if len(ve.Violations) == 0 || !strings.Contains(ve.Violations[0], "privileged") {
		t.Fatalf("violations = %v, want privileged", ve.Violations)
	}
}

func noComposeRun(t *testing.T, mock *MockRunner) {
	t.Helper()
	for _, sub := range []string{"up", "pull"} {
		if mock.HasCall("docker", "compose", sub) {
			t.Fatalf("docker compose %s ran for a blocked compose file: %+v", sub, mock.Calls)
		}
	}
}

func TestDeployRefusesUnsafeComposeOnDisk(t *testing.T) {
	mock := &MockRunner{}
	fa := &fakeAudit{}
	d := &Deployer{runner: mock, Tracker: NewTracker(), audit: fa}

	res := d.Deploy(context.Background(), testAppWith(t, "myapp", privilegedCompose))
	wantBlocked(t, res.Err)
	noComposeRun(t, mock)
	if res.Status != "failed" || !strings.Contains(res.Output, "privileged") {
		t.Errorf("result = %+v, want failed status and violation in output", res)
	}
	if d.Tracker.IsDeploying("myapp") {
		t.Error("tracker still marks myapp as deploying")
	}
	if evt, ok := fa.last(); !ok || evt.Action != "deploy_failed" {
		t.Errorf("audit = %+v, want deploy_failed", evt)
	}
}

func TestDeployRechecksFileChangedAfterParse(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := testApp(t, "myapp")
	// The caller parsed a safe file; it was swapped before deploy ran.
	if err := os.WriteFile(app.ComposePath, []byte(privilegedCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	wantBlocked(t, d.Deploy(context.Background(), app).Err)
	noComposeRun(t, mock)
}

func TestDeployRefusesUnreadableCompose(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/nonexistent/docker-compose.yml"}
	if res := d.Deploy(context.Background(), app); res.Err == nil {
		t.Fatal("want error for missing compose file")
	}
	noComposeRun(t, mock)
}

func TestRollbackDeployRefusesUnsafeCompose(t *testing.T) {
	mock := &MockRunner{}
	fa := &fakeAudit{}
	d := &Deployer{runner: mock, audit: fa}
	cvID := int64(7)
	res := d.RollbackDeploy(context.Background(), testAppWith(t, "myapp", privilegedCompose), 2, &cvID)
	wantBlocked(t, res.Err)
	noComposeRun(t, mock)
	if evt, ok := fa.last(); !ok || evt.Action != "rollback" || evt.Error == "" {
		t.Errorf("audit = %+v, want rollback with error", evt)
	}
}

func TestPullRefusesUnsafeCompose(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	wantBlocked(t, d.Pull(context.Background(), testAppWith(t, "myapp", privilegedCompose), nil).Err)
	noComposeRun(t, mock)
}

func TestScaleRefusesUnsafeCompose(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	wantBlocked(t, d.Scale(context.Background(), testAppWith(t, "myapp", privilegedCompose), map[string]int{"web": 2}))
	noComposeRun(t, mock)
}

func TestCancelRefusesUnsafeCompose(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock, Tracker: NewTracker()}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Tracker.Track("myapp", cancel)
	wantBlocked(t, d.Cancel(context.Background(), testAppWith(t, "myapp", privilegedCompose)))
	noComposeRun(t, mock)
}

func TestDeployRefusesBrokenDotEnv(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := testApp(t, "myapp")
	if err := os.WriteFile(filepath.Join(filepath.Dir(app.ComposePath), ".env"), []byte("PASS=\"unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := d.Deploy(context.Background(), app)
	if !errors.Is(res.Err, compose.ErrDotEnv) {
		t.Fatalf("err = %v, want ErrDotEnv", res.Err)
	}
	noComposeRun(t, mock)
}
