package backup

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/vazra/simpledeploy/internal/events"
	"github.com/vazra/simpledeploy/internal/store"
)

// recordingPublisher records published events with a note taken at publish
// time (e.g. the run's status then).
type recordingPublisher struct {
	mu     sync.Mutex
	note   func() string
	events []string
}

func (p *recordingPublisher) Publish(_ context.Context, e events.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := ""
	if p.note != nil {
		n = p.note()
	}
	p.events = append(p.events, fmt.Sprintf("%s %s app=%s %s", e.Type, e.Topic, e.App, n))
}

func (p *recordingPublisher) got() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

func assertEvents(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %q, want %q", got, want)
		}
	}
}

func TestRunBackupPublishesStartAndFinish(t *testing.T) {
	for _, tc := range []struct {
		name     string
		strategy Strategy
		final    string
	}{
		{"success", &mockStrategy{data: "d", filename: "b.tar.gz"}, "success"},
		{"failure", &mockStrategy{err: errors.New("boom")}, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newMockStore()
			st.configs[1] = &store.BackupConfig{ID: 1, AppID: 10, Strategy: "mock", Target: "mock", RetentionMode: "count", RetentionCount: 5}
			sched := NewScheduler(st, nil)
			sched.RegisterStrategy("mock", tc.strategy)
			sched.RegisterTargetFactory("mock", func(string) (Target, error) { return newMockTarget(), nil })
			pub := &recordingPublisher{note: func() string {
				if r, ok := st.runs[1]; ok {
					return r.Status
				}
				return "no-run"
			}}
			sched.SetEventBus(pub)
			var alertEvents int
			sched.SetAlertFunc(func(int64, string, string, string, string) { alertEvents = len(pub.got()) })

			_ = sched.RunBackup(context.Background(), 1)

			// Started once the run row exists, finished once its status is final.
			assertEvents(t, pub.got(), []string{
				"backup.run app:testapp app=testapp running",
				"backup.run global:backups app=testapp running",
				"backup.run app:testapp app=testapp " + tc.final,
				"backup.run global:backups app=testapp " + tc.final,
			})
			// The UI hears about the result before slow webhook alerts go out.
			if alertEvents != 4 {
				t.Errorf("alert sent after %d events, want 4", alertEvents)
			}
		})
	}
}

func TestRunBackupPublishesWhenRunFailsEarly(t *testing.T) {
	st := newMockStore()
	st.configs[1] = &store.BackupConfig{ID: 1, AppID: 10, Strategy: "unknown", Target: "mock"}
	sched := NewScheduler(st, nil)
	pub := &recordingPublisher{}
	sched.SetEventBus(pub)

	if err := sched.RunBackup(context.Background(), 1); err == nil {
		t.Fatal("expected error for unknown strategy")
	}
	if st.failedCall == nil {
		t.Fatal("run not marked failed")
	}
	if got := pub.got(); len(got) != 4 {
		t.Fatalf("events = %q, want started and finished on both topics", got)
	}
}

func TestRunRestorePublishesStartAndFinish(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"success", nil},
		{"failure", errors.New("boom")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newMockStore()
			st.configs[1] = &store.BackupConfig{ID: 1, AppID: 10, Strategy: "mock", Target: "mock"}
			st.runs[7] = &store.BackupRun{ID: 7, BackupConfigID: 1, Status: "success", FilePath: "b.tar.gz"}
			tgt := newMockTarget()
			tgt.uploaded["b.tar.gz"] = []byte("payload")
			sched := NewScheduler(st, nil)
			sched.RegisterStrategy("mock", &mockStrategy{err: tc.err})
			sched.RegisterTargetFactory("mock", func(string) (Target, error) { return tgt, nil })
			pub := &recordingPublisher{}
			sched.SetEventBus(pub)

			err := sched.RunRestore(context.Background(), 7)
			if (err != nil) != (tc.err != nil) {
				t.Fatalf("RunRestore err = %v, want error %v", err, tc.err != nil)
			}
			assertEvents(t, pub.got(), []string{
				"backup.run app:testapp app=testapp ",
				"backup.run global:backups app=testapp ",
				"backup.run app:testapp app=testapp ",
				"backup.run global:backups app=testapp ",
			})
		})
	}
}

func TestSchedulerWithoutEventBus(t *testing.T) {
	st := newMockStore()
	st.configs[1] = &store.BackupConfig{ID: 1, AppID: 10, Strategy: "mock", Target: "mock"}
	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("mock", &mockStrategy{data: "d", filename: "b.tar.gz"})
	sched.RegisterTargetFactory("mock", func(string) (Target, error) { return newMockTarget(), nil })
	if err := sched.RunBackup(context.Background(), 1); err != nil {
		t.Fatalf("RunBackup without bus: %v", err)
	}
}
