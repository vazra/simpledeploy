package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

// eventStrategy records when the backup/restore itself runs, so tests can
// check it happens between pre- and post-hooks.
type eventStrategy struct {
	events *[]string
	detect []DetectedService
}

func (s *eventStrategy) Type() string { return "mock" }

func (s *eventStrategy) Detect(*compose.AppConfig) []DetectedService { return s.detect }

func (s *eventStrategy) Backup(context.Context, BackupOpts) (*BackupResult, error) {
	*s.events = append(*s.events, "backup")
	return &BackupResult{Reader: io.NopCloser(strings.NewReader("data")), Filename: "b.tar.gz"}, nil
}

func (s *eventStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	*s.events = append(*s.events, "restore")
	return nil
}

type hookSchedFixture struct {
	sched  *Scheduler
	st     *mockStore
	exec   *mockExecutor
	events *[]string
	alerts []string
}

func newHookSched(t *testing.T, pre, post string, withExec bool) *hookSchedFixture {
	t.Helper()
	events := &[]string{}
	f := &hookSchedFixture{st: newMockStore(), events: events}
	f.st.configs[1] = &store.BackupConfig{ID: 1, AppID: 10, Strategy: "mock", Target: "mock", PreHooks: pre, PostHooks: post}
	var exec ContainerExecutor
	if withExec {
		f.exec = newMockExecutor()
		f.exec.events = events
		exec = f.exec
	}
	f.sched = NewScheduler(f.st, exec)
	f.sched.RegisterStrategy("mock", &eventStrategy{events: events})
	tgt := newMockTarget()
	f.sched.RegisterTargetFactory("mock", func(string) (Target, error) { return tgt, nil })
	f.sched.SetAlertFunc(func(_, _, _, eventType string) { f.alerts = append(f.alerts, eventType) })
	return f
}

func (f *hookSchedFixture) calls() int {
	if f.exec == nil {
		return 0
	}
	return len(f.exec.calls)
}

func TestSchedulerRunsHooksAroundBackup(t *testing.T) {
	f := newHookSched(t,
		`[{"type":"stop","service":"web"},{"type":"exec","service":"cache","command":"redis-cli SAVE"}]`,
		`[{"type":"start","service":"web"}]`, true)

	if err := f.sched.RunBackup(context.Background(), 1); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if got := strings.Join(*f.events, ","); got != "stop:web,exec:cache,backup,start:web" {
		t.Fatalf("order = %s", got)
	}
	for _, c := range f.exec.calls {
		if c.project != "simpledeploy-testapp" {
			t.Errorf("hook %+v ran against project %q, want simpledeploy-testapp", c, c.project)
		}
	}
	if f.st.successCall == nil || f.st.failedCall != nil {
		t.Fatalf("run not marked success: failed=%+v", f.st.failedCall)
	}
}

func TestSchedulerPreHookFailureFailsRun(t *testing.T) {
	f := newHookSched(t,
		`[{"type":"stop","service":"web"},{"type":"exec","service":"web","command":"sync"}]`,
		`[{"type":"start","service":"web"}]`, true)
	f.exec.failOn["exec:web"] = errors.New("sync: not found")

	if err := f.sched.RunBackup(context.Background(), 1); err == nil {
		t.Fatal("RunBackup: expected error")
	}
	if f.st.failedCall == nil || !strings.Contains(f.st.failedCall.errMsg, "pre-hook") || !strings.Contains(f.st.failedCall.errMsg, "sync: not found") {
		t.Fatalf("failed run = %+v", f.st.failedCall)
	}
	if f.st.successCall != nil {
		t.Fatal("run marked success")
	}
	// The backup never ran, and post-hooks still restarted the stopped service.
	if got := strings.Join(*f.events, ","); got != "stop:web,exec:web,start:web" {
		t.Fatalf("order = %s", got)
	}
	if len(f.alerts) != 1 || f.alerts[0] != "backup_failed" {
		t.Errorf("alerts = %v", f.alerts)
	}
}

func TestSchedulerPostHookFailureIsWarning(t *testing.T) {
	f := newHookSched(t, "", `[{"type":"start","service":"web"},{"type":"exec","service":"web","command":"true"}]`, true)
	f.exec.failOn["start:web"] = errors.New("start failed")

	if err := f.sched.RunBackup(context.Background(), 1); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if f.st.successCall == nil || f.st.failedCall != nil {
		t.Fatalf("run should succeed, failed=%+v", f.st.failedCall)
	}
	if got := strings.Join(*f.events, ","); got != "backup,start:web,exec:web" {
		t.Fatalf("order = %s", got)
	}
}

func TestSchedulerHooksWithoutExecutorFailRun(t *testing.T) {
	f := newHookSched(t, `[{"type":"stop","service":"web"}]`, "", false)
	if err := f.sched.RunBackup(context.Background(), 1); err == nil {
		t.Fatal("RunBackup: expected error")
	}
	if f.st.failedCall == nil || !strings.Contains(f.st.failedCall.errMsg, "not connected to Docker") {
		t.Fatalf("failed run = %+v", f.st.failedCall)
	}
	if len(*f.events) != 0 {
		t.Fatalf("backup ran without its hooks: %v", *f.events)
	}
}

// Hooks from a sidecar or git sync skip the API, so the run re-checks them.
func TestSchedulerRejectsInvalidStoredHooks(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n  db:\n    image: postgres:16\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ pre, post, want string }{
		"unknown type":      {`[{"type":"rm","service":"web"}]`, "", "unknown type"},
		"other app service": {`[{"type":"stop","service":"cache"}]`, "", "not a service of this app"},
		"container name":    {"", `[{"type":"exec","service":"simpledeploy-other-db-1","command":"id"}]`, "not a service of this app"},
		"bad json":          {`[{"type":`, "", "Pre-backup hooks"},
		"stops strategy":    {`[{"type":"stop","service":"db"}]`, "", "cannot be stopped"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newHookSched(t, tc.pre, tc.post, true)
			f.st.composePath = composePath
			f.sched.RegisterStrategy("mock", &eventStrategy{events: f.events, detect: []DetectedService{{ServiceName: "db", ContainerName: "simpledeploy-testapp-db-1"}}})
			if err := f.sched.RunBackup(context.Background(), 1); err == nil {
				t.Fatal("RunBackup: expected error")
			}
			if f.st.failedCall == nil || !strings.Contains(f.st.failedCall.errMsg, tc.want) {
				t.Fatalf("failed run = %+v, want %q", f.st.failedCall, tc.want)
			}
			if f.calls() != 0 || len(*f.events) != 0 {
				t.Fatalf("something ran: %v", *f.events)
			}
		})
	}
}

func TestSchedulerSkipsLegacyWizardHooks(t *testing.T) {
	f := newHookSched(t,
		`[{"type":"stop_container","service":{"service_name":"web","container_name":"simpledeploy-testapp-web-1"}}]`,
		`[{"type":"start_container","service":{"service_name":"web"}}]`, true)
	if err := f.sched.RunBackup(context.Background(), 1); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if f.calls() != 0 {
		t.Fatalf("legacy hooks ran: %+v", f.exec.calls)
	}
	if f.st.successCall == nil {
		t.Fatal("run not marked success")
	}
}

func TestSchedulerRunsHooksAroundRestore(t *testing.T) {
	spoolDir(t)
	f := newHookSched(t, `[{"type":"stop","service":"web"}]`, `[{"type":"start","service":"web"}]`, true)
	tgt := newMockTarget()
	tgt.uploaded["b.tar.gz"] = []byte("payload")
	f.sched.RegisterTargetFactory("mock", func(string) (Target, error) { return tgt, nil })
	f.st.runs[7] = &store.BackupRun{ID: 7, BackupConfigID: 1, Status: "success", FilePath: "b.tar.gz", Checksum: checksumOf(t, "payload")}

	if err := f.sched.RunRestore(context.Background(), 7); err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if got := strings.Join(*f.events, ","); got != "stop:web,restore,start:web" {
		t.Fatalf("order = %s", got)
	}

	*f.events = nil
	f.exec.failOn["stop:web"] = errors.New("boom")
	if err := f.sched.RunRestore(context.Background(), 7); err == nil || !strings.Contains(err.Error(), "pre-hook") {
		t.Fatalf("RunRestore err = %v, want pre-hook failure", err)
	}
	if got := strings.Join(*f.events, ","); got != "stop:web,start:web" {
		t.Fatalf("order after failed pre-hook = %s", got)
	}
}
