package gitsync

// Tests for keeping access grants dashboard-managed across pulls: blocked
// apps, grants changed while pulled files are on disk, and pull-only mode.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5"

	"github.com/vazra/simpledeploy/internal/store"
)

// recordingReconciler counts ReconcileAfterSync calls and keeps their paths.
type recordingReconciler struct {
	count *atomic.Int64
	mu    sync.Mutex
	paths [][]string
}

func (r *recordingReconciler) ReconcileAfterSync(_ context.Context, paths []string) error {
	r.count.Add(1)
	r.mu.Lock()
	r.paths = append(r.paths, slices.Clone(paths))
	r.mu.Unlock()
	return nil
}

func (r *recordingReconciler) allPaths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, p := range r.paths {
		out = append(out, p...)
	}
	return out
}

// seedAliceApp seeds app1 with alice granted access and a bob user without
// access, and writes the sidecar.
func seedAliceApp(t *testing.T, e *secEnv) *store.App {
	t.Helper()
	app := seedApp(t, e.st, e.appsDir, "app1")
	alice := seedUser(t, e.st, "alice", "viewer")
	seedUser(t, e.st, "bob", "viewer")
	if err := e.st.GrantAppAccess(alice.ID, app.ID); err != nil {
		t.Fatalf("GrantAppAccess: %v", err)
	}
	if err := e.cs.WriteAppSidecar("app1"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}
	return app
}

// TestStartEnablesDashboardOnlyAccess: a running git sync makes the
// configsync FS->DB apply path skip access lists; Stop turns it off.
func TestStartEnablesDashboardOnlyAccess(t *testing.T) {
	e := newSecEnv(t, nil, nil)
	if !e.cs.AccessFromDashboardOnly() {
		t.Fatal("AccessFromDashboardOnly = false after Start")
	}
	_ = e.gs.Stop()
	if e.cs.AccessFromDashboardOnly() {
		t.Fatal("AccessFromDashboardOnly = true after Stop")
	}
}

// TestPullBlockedAppKeepsDashboardAccess: a pull that changes an app's
// access list and also adds a tracked symlink to that app blocks the app,
// but its sidecar access is still restored, the DB keeps the dashboard
// grants, and a watcher-style reload of the file changes nothing.
func TestPullBlockedAppKeepsDashboardAccess(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.env")
	writeFile(t, outside, "SECRET=1\n")

	var app *store.App
	e := newSecEnv(t, func(e *secEnv) { app = seedAliceApp(t, e) }, nil)

	pushFromSymlinkClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarWithBob)
		if err := os.Symlink(outside, filepath.Join(dir, "app1", ".env")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	})

	err := e.gs.SyncNow(e.ctx)
	if err == nil || !strings.Contains(err.Error(), "app1/.env") {
		t.Fatalf("SyncNow error = %v, want symlink refusal naming app1/.env", err)
	}
	if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("DB access = %v, want [alice]", got)
	}
	if got := sidecarAccess(t, e.cs, "app1"); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("sidecar access on disk = %v, want [alice]", got)
	}
	if out, _ := gitExec(e.appsDir, "status", "--porcelain", "--", "app1/simpledeploy.yml"); strings.TrimSpace(string(out)) != "" {
		t.Errorf("restored sidecar left uncommitted: %s", out)
	}

	// What the reconciler's file watcher does when the sidecar changes.
	loaded, err := e.cs.LoadAppFromFS("app1")
	if err != nil {
		t.Fatalf("LoadAppFromFS: %v", err)
	}
	if err := e.cs.ApplyAppSidecar("app1", loaded); err != nil {
		t.Fatalf("ApplyAppSidecar: %v", err)
	}
	if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("DB access after watcher apply = %v, want [alice]", got)
	}

	// Even a file that still carried the pulled list would not change grants.
	writeFile(t, filepath.Join(e.appsDir, "app1", "simpledeploy.yml"), pulledSidecarWithBob)
	loaded, err = e.cs.LoadAppFromFS("app1")
	if err != nil {
		t.Fatalf("LoadAppFromFS: %v", err)
	}
	if err := e.cs.ApplyAppSidecar("app1", loaded); err != nil {
		t.Fatalf("ApplyAppSidecar: %v", err)
	}
	if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("DB access after applying pulled list = %v, want [alice]", got)
	}
}

// TestPullRestoresDBAccessChangedDuringPull: grants changed between the
// rebase and the access restore (e.g. something applied the pulled file
// first) are reset to the pre-pull snapshot, and the file matches it.
func TestPullRestoresDBAccessChangedDuringPull(t *testing.T) {
	var app *store.App
	e := newSecEnv(t, func(e *secEnv) { app = seedAliceApp(t, e) }, nil)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarWithBob)
	})

	// Drive applyFetched's steps by hand to change the DB in the window
	// between the rebase and the restore. The worker is idle (no poll loop).
	prev := e.gs.currentHeadSHA()
	snap := e.gs.snapshotAccess()
	if _, err := e.gs.fetchAndInspect(); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, _, err := rebaseServerWins(e.appsDir, "main"); err != nil {
		t.Fatalf("rebase: %v", err)
	}
	repo, err := git.PlainOpen(e.appsDir)
	if err != nil {
		t.Fatal(err)
	}
	e.gs.repo = repo
	e.gs.updateHeadSHA()
	if err := e.st.ReplaceAppAccess(app.ID, []string{"bob"}); err != nil {
		t.Fatalf("ReplaceAppAccess: %v", err)
	}

	chk := e.gs.securePulledTree(prev, e.gs.currentHeadSHA(), snap)
	if !chk.committed {
		t.Error("access restore not committed")
	}
	if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("DB access = %v, want [alice]", got)
	}
	if got := sidecarAccess(t, e.cs, "app1"); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("sidecar access on disk = %v, want [alice]", got)
	}
}

// handFormattedSidecar is a sidecar as someone might write it by hand. Its
// layout differs from what SimpleDeploy writes, so an access restore
// rewrites every line and conflicts with any later remote edit.
func handFormattedSidecar(threshold string) string {
	return `# app1 settings
version: 1
app: {slug: app1, display_name: app1}
alert_rules:
  - {metric: cpu_pct, operator: ">", threshold: ` + threshold + `, duration_sec: 60, webhook: ops, enabled: true}
access: [{username: bob}]
`
}

// TestPullOnlyModeKeepsRemoteEditsAfterAccessRestore: in pull-only mode the
// access restore commit stays local. A later remote edit to the same
// sidecar must still apply instead of losing to that commit on rebase.
func TestPullOnlyModeKeepsRemoteEditsAfterAccessRestore(t *testing.T) {
	var app *store.App
	e := newSecEnv(t, func(e *secEnv) { app = seedAliceApp(t, e) },
		func(c *Config) { c.AutoPushEnabled = false })

	threshold := func() float64 {
		t.Helper()
		rules, err := e.st.ListAlertRules(&app.ID)
		if err != nil || len(rules) != 1 {
			t.Fatalf("alert rules = %+v (err %v), want 1", rules, err)
		}
		return rules[0].Threshold
	}

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), handFormattedSidecar("90"))
	})
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("first SyncNow: %v", err)
	}
	if got := threshold(); got != 90 {
		t.Fatalf("threshold after first pull = %v, want 90", got)
	}
	if out, _ := gitExec(e.appsDir, "log", "-1", "--format=%s"); strings.TrimSpace(string(out)) != restoreAccessSubject {
		t.Fatalf("HEAD subject = %q, want local access restore commit", out)
	}

	for _, want := range []string{"75", "60"} {
		pushFromClone(t, e.bareDir, func(dir string) {
			writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), handFormattedSidecar(want))
		})
		if err := e.gs.SyncNow(e.ctx); err != nil {
			t.Fatalf("SyncNow (threshold %s): %v", want, err)
		}
		if got := readFile(t, filepath.Join(e.appsDir, "app1", "simpledeploy.yml")); !strings.Contains(got, "threshold: "+want) {
			t.Errorf("sidecar on disk lost remote threshold %s:\n%s", want, got)
		}
		if wantF, _ := strconv.ParseFloat(want, 64); threshold() != wantF {
			t.Errorf("DB threshold = %v, want %s", threshold(), want)
		}
		if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
			t.Errorf("DB access = %v, want [alice]", got)
		}
		if got := sidecarAccess(t, e.cs, "app1"); !slices.Equal(got, []string{"alice"}) {
			t.Errorf("sidecar access on disk = %v, want [alice]", got)
		}
		if out, _ := gitExec(e.appsDir, "status", "--porcelain", "--", "app1/simpledeploy.yml"); strings.TrimSpace(string(out)) != "" {
			t.Errorf("sidecar left uncommitted: %s", out)
		}
	}
	if remote, _ := gitExec(e.bareDir, "show", "main:app1/simpledeploy.yml"); strings.Contains(string(remote), "alice") {
		t.Errorf("pull-only mode pushed the restore:\n%s", remote)
	}
	if rebaseInProgress(e.appsDir) {
		t.Error("rebase left in progress")
	}
}

// TestPullBlockedAppStillReconcilesOthers: a symlink in one app does not
// hold back the rest of the pull. The other app's sidecar is imported and
// the reconcile runs for its paths only.
func TestPullBlockedAppStillReconcilesOthers(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.env")
	writeFile(t, outside, "SECRET=1\n")

	var app1, app2 *store.App
	e := newSecEnv(t, func(e *secEnv) {
		app1 = seedApp(t, e.st, e.appsDir, "app1")
		app2 = seedApp(t, e.st, e.appsDir, "app2")
	}, nil)

	pushFromSymlinkClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarNoAccess)
		writeFile(t, filepath.Join(dir, "app2", "simpledeploy.yml"), strings.ReplaceAll(pulledSidecarNoAccess, "app1", "app2"))
		writeFile(t, filepath.Join(dir, "app2", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
		if err := os.Symlink(outside, filepath.Join(dir, "app2", ".env")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	})

	err := e.gs.SyncNow(e.ctx)
	if err == nil || !strings.Contains(err.Error(), "app2/.env") || strings.Contains(err.Error(), "app1") {
		t.Fatalf("SyncNow error = %v, want symlink refusal naming only app2", err)
	}
	if !strings.Contains(err.Error(), "other changes from this pull were applied") {
		t.Errorf("error does not say the rest of the pull applied: %v", err)
	}
	if rules, _ := e.st.ListAlertRules(&app1.ID); len(rules) != 1 {
		t.Errorf("app1 alert rules = %+v, want pulled rule imported", rules)
	}
	if rules, _ := e.st.ListAlertRules(&app2.ID); len(rules) != 0 {
		t.Errorf("blocked app2 sidecar imported: %+v", rules)
	}
	if got := e.recs.Load(); got != 1 {
		t.Fatalf("reconcile called %d times, want 1", got)
	}
	paths := e.rec.allPaths()
	if !slices.Contains(paths, "app1/simpledeploy.yml") {
		t.Errorf("reconcile paths = %v, want app1/simpledeploy.yml", paths)
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "app2/") {
			t.Errorf("reconcile paths include blocked app2: %v", paths)
		}
	}
}
