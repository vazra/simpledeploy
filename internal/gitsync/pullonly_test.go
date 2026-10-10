package gitsync

// Tests for pull-only mode (auto-push off): local changes are committed but
// never pushed, and pending changes never block a pull.

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

func pullOnly(c *Config) { c.AutoPushEnabled = false }

// remoteHead returns the commit the bare remote's main branch points at.
func remoteHead(t *testing.T, bareDir string) string {
	t.Helper()
	out, err := gitExec(bareDir, "rev-parse", "refs/heads/main")
	if err != nil {
		t.Fatalf("rev-parse remote main: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitOut runs git in dir and returns trimmed output, failing the test on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitExec(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func hasConflict(s *Syncer, path string) bool {
	for _, c := range s.Status().RecentConflicts {
		if c.Path == path && strings.Contains(c.Description, "server-wins") {
			return true
		}
	}
	return false
}

// TestPullOnlyCommitsDashboardEditLocally: a dashboard change is committed
// with the usual bot message, and nothing reaches the remote.
func TestPullOnlyCommitsDashboardEditLocally(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) { seedApp(t, e.st, e.appsDir, "app1") }, pullOnly)
	before := remoteHead(t, e.bareDir)

	compose := filepath.Join(e.appsDir, "app1", "docker-compose.yml")
	writeFile(t, compose, "services:\n  web:\n    image: nginx:local\n")
	prev := e.gs.Status().HeadSHA
	e.gs.EnqueueCommit([]string{compose}, "deploy:app1")
	waitForHeadUpdate(t, e.gs, prev, 5*time.Second)

	msg := gitOut(t, e.appsDir, "log", "-1", "--format=%B")
	if !strings.HasPrefix(msg, syncCommitSubject) || !isBotCommit(msg) || !strings.Contains(msg, "Reason: deploy:app1") {
		t.Errorf("local commit message = %q", msg)
	}
	if out := gitOut(t, e.appsDir, "status", "--porcelain"); out != "" {
		t.Errorf("working tree not clean after commit: %s", out)
	}
	if got := remoteHead(t, e.bareDir); got != before {
		t.Errorf("pull-only mode pushed: remote main %s, want %s", got, before)
	}
	if st := e.gs.Status(); st.LastSyncError != "" || st.AutoPushEnabled {
		t.Errorf("status = error %q, auto-push %v", st.LastSyncError, st.AutoPushEnabled)
	}
}

// TestPullOnlyUncommittedEditDoesNotBlockPull: an edit and a deletion that
// never got a commit are committed locally before the rebase, so a remote
// change to another file applies.
func TestPullOnlyUncommittedEditDoesNotBlockPull(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) {
		seedApp(t, e.st, e.appsDir, "app1")
		seedApp(t, e.st, e.appsDir, "app2")
		writeFile(t, filepath.Join(e.appsDir, "app1", ".env"), "A=1\n")
	}, pullOnly)

	app1 := filepath.Join(e.appsDir, "app1", "docker-compose.yml")
	writeFile(t, app1, "services:\n  web:\n    image: nginx:local\n")
	if err := os.Remove(filepath.Join(e.appsDir, "app1", ".env")); err != nil {
		t.Fatal(err)
	}

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app2", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
	})
	remote := remoteHead(t, e.bareDir)

	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app2", "docker-compose.yml")); !strings.Contains(got, "nginx:2") {
		t.Errorf("remote change not applied: %q", got)
	}
	if got := readFile(t, app1); !strings.Contains(got, "nginx:local") {
		t.Errorf("local edit lost: %q", got)
	}
	if _, err := os.Stat(filepath.Join(e.appsDir, "app1", ".env")); !os.IsNotExist(err) {
		t.Errorf("deleted app1/.env is back (stat err %v)", err)
	}
	if out := gitOut(t, e.appsDir, "status", "--porcelain"); out != "" {
		t.Errorf("working tree not clean: %s", out)
	}
	if out := gitOut(t, e.appsDir, "ls-files", "--", "app1/.env"); out != "" {
		t.Errorf("deletion not committed: %s", out)
	}
	msg := gitOut(t, e.appsDir, "log", "-1", "--format=%B")
	if !strings.HasPrefix(msg, syncCommitSubject) || !isBotCommit(msg) {
		t.Errorf("HEAD is not the local bot commit: %q", msg)
	}
	if _, err := gitExec(e.appsDir, "merge-base", "--is-ancestor", "origin/main", "HEAD"); err != nil {
		t.Errorf("origin/main not applied under the local commit: %v", err)
	}
	if got := remoteHead(t, e.bareDir); got != remote {
		t.Errorf("pull-only mode pushed: remote main %s, want %s", got, remote)
	}
	st := e.gs.Status()
	if st.LastSyncError != "" || st.CommitsBehind != 0 {
		t.Errorf("status = error %q, behind %d", st.LastSyncError, st.CommitsBehind)
	}
	// Only what the pull changed is reconciled, not the local edit.
	paths := e.rec.allPaths()
	if !slices.Contains(paths, "app2/docker-compose.yml") || slices.Contains(paths, "app1/docker-compose.yml") {
		t.Errorf("reconcile paths = %v", paths)
	}
}

// TestPullOnlyNewUntrackedAppDoesNotBlockPull: untracked managed files (new
// apps created in the dashboard) are committed locally, including one the
// pull also adds; files git sync does not manage stay untracked.
func TestPullOnlyNewUntrackedAppDoesNotBlockPull(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) { seedApp(t, e.st, e.appsDir, "app1") }, pullOnly)

	sharedCompose := "services:\n  web:\n    image: nginx:shared\n"
	writeFile(t, filepath.Join(e.appsDir, "newapp", "docker-compose.yml"), "services:\n  web:\n    image: nginx:new\n")
	writeFile(t, filepath.Join(e.appsDir, "newapp", ".env"), "B=2\n")
	writeFile(t, filepath.Join(e.appsDir, "newapp", "simpledeploy.yml"), "version: 1\napp:\n  slug: newapp\n  display_name: newapp\n")
	writeFile(t, filepath.Join(e.appsDir, "newapp", "data", "state.db"), "x")
	writeFile(t, filepath.Join(e.appsDir, "shared", "docker-compose.yml"), sharedCompose)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
		writeFile(t, filepath.Join(dir, "shared", "docker-compose.yml"), sharedCompose)
	})
	remote := remoteHead(t, e.bareDir)

	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml")); !strings.Contains(got, "nginx:2") {
		t.Errorf("remote change not applied: %q", got)
	}
	tracked := gitOut(t, e.appsDir, "ls-files", "--", "newapp", "shared")
	want := "newapp/.env\nnewapp/docker-compose.yml\nnewapp/simpledeploy.yml\nshared/docker-compose.yml"
	if tracked != want {
		t.Errorf("tracked files = %q, want %q", tracked, want)
	}
	if out := gitOut(t, e.appsDir, "status", "--porcelain"); out != "" {
		t.Errorf("working tree not clean: %s", out)
	}
	if hasConflict(e.gs, "shared/docker-compose.yml") {
		t.Error("identical file added on both sides reported as a conflict")
	}
	if got := remoteHead(t, e.bareDir); got != remote {
		t.Errorf("pull-only mode pushed: remote main %s, want %s", got, remote)
	}
	if files := gitOut(t, e.bareDir, "ls-tree", "-r", "--name-only", "main"); strings.Contains(files, "newapp/") {
		t.Errorf("local app reached the remote:\n%s", files)
	}
}

// TestPullOnlyConflictKeepsLocalVersion: local and remote changes to the same
// file (an edit, and a new app added on both sides) follow the server-wins
// policy; the rest of the pull applies, and later pulls keep working.
func TestPullOnlyConflictKeepsLocalVersion(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) {
		seedApp(t, e.st, e.appsDir, "app1")
		seedApp(t, e.st, e.appsDir, "app2")
	}, pullOnly)

	app1 := filepath.Join(e.appsDir, "app1", "docker-compose.yml")
	writeFile(t, app1, "services:\n  web:\n    image: nginx:X\n")
	prev := e.gs.Status().HeadSHA
	e.gs.EnqueueCommit([]string{app1}, "deploy:app1")
	waitForHeadUpdate(t, e.gs, prev, 5*time.Second)
	web := filepath.Join(e.appsDir, "web", "docker-compose.yml")
	writeFile(t, web, "services:\n  web:\n    image: web:local\n")

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "docker-compose.yml"), "services:\n  web:\n    image: nginx:Y\n")
		writeFile(t, filepath.Join(dir, "web", "docker-compose.yml"), "services:\n  web:\n    image: web:remote\n")
		writeFile(t, filepath.Join(dir, "app2", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
	})
	remote := remoteHead(t, e.bareDir)

	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if got := readFile(t, app1); !strings.Contains(got, "nginx:X") {
		t.Errorf("app1 = %q, want local nginx:X", got)
	}
	if got := readFile(t, web); !strings.Contains(got, "web:local") {
		t.Errorf("web = %q, want local web:local", got)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app2", "docker-compose.yml")); !strings.Contains(got, "nginx:2") {
		t.Errorf("non-conflicting remote change not applied: %q", got)
	}
	for _, p := range []string{"app1/docker-compose.yml", "web/docker-compose.yml"} {
		if !hasConflict(e.gs, p) {
			t.Errorf("no server-wins conflict recorded for %s: %+v", p, e.gs.Status().RecentConflicts)
		}
	}
	if rebaseInProgress(e.appsDir) {
		t.Fatal("rebase left in progress")
	}
	if out := gitOut(t, e.appsDir, "status", "--porcelain"); out != "" {
		t.Errorf("working tree not clean: %s", out)
	}
	if got := remoteHead(t, e.bareDir); got != remote {
		t.Errorf("pull-only mode pushed: remote main %s, want %s", got, remote)
	}

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app2", "docker-compose.yml"), "services:\n  web:\n    image: nginx:3\n")
	})
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("second SyncNow: %v", err)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app2", "docker-compose.yml")); !strings.Contains(got, "nginx:3") {
		t.Errorf("second pull not applied: %q", got)
	}
	if got := readFile(t, app1); !strings.Contains(got, "nginx:X") {
		t.Errorf("app1 after second pull = %q, want local nginx:X", got)
	}
}

// handFormattedAliceSidecar is handFormattedSidecar with the dashboard's
// access list, so the pull needs no access restore and the file stays as
// written until SimpleDeploy re-renders it from the DB.
func handFormattedAliceSidecar(threshold string) string {
	return strings.Replace(handFormattedSidecar(threshold), "access: [{username: bob}]", "access: [{username: alice}]", 1)
}

// TestPullOnlyRewriteOfPulledSidecarYieldsToRemote: after a pull imports a
// hand-written sidecar, the server re-renders it from the DB (store mutation
// hook -> debounced sidecar write) while the import is suppressed. That
// rewrite is committed locally before the next rebase, but it must not win
// over later remote edits to the same file.
func TestPullOnlyRewriteOfPulledSidecarYieldsToRemote(t *testing.T) {
	var app *store.App
	e := newSecEnv(t, func(e *secEnv) { app = seedAliceApp(t, e) }, pullOnly)

	// Hooks as wired by the server (cmd/simpledeploy/main.go).
	e.st.SetMutationHook(func(scope store.MutationScope, slug string) {
		if scope == store.ScopeApp && slug != "" {
			e.cs.ScheduleAppWrite(slug)
		}
	})
	t.Cleanup(func() { e.st.SetMutationHook(nil) })
	e.cs.SetSidecarWriteHook(func(path, reason string) {
		if path == "" {
			e.gs.EnqueueCommit(nil, reason)
			return
		}
		e.gs.EnqueueCommit([]string{path}, reason)
	})

	sidecar := filepath.Join(e.appsDir, "app1", "simpledeploy.yml")
	for _, want := range []string{"90", "75", "60"} {
		pushFromClone(t, e.bareDir, func(dir string) {
			writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), handFormattedAliceSidecar(want))
		})
		remote := remoteHead(t, e.bareDir)
		if err := e.gs.SyncNow(e.ctx); err != nil {
			t.Fatalf("SyncNow (threshold %s): %v", want, err)
		}
		rules, err := e.st.ListAlertRules(&app.ID)
		wantF, _ := strconv.ParseFloat(want, 64)
		if err != nil || len(rules) != 1 || rules[0].Threshold != wantF {
			t.Fatalf("alert rules after pull %s = %+v (err %v)", want, rules, err)
		}
		if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
			t.Errorf("DB access = %v, want [alice]", got)
		}

		waitFor(t, 5*time.Second, "sidecar re-rendered from the DB", func() bool {
			b, _ := os.ReadFile(sidecar)
			return !strings.Contains(string(b), "# app1 settings")
		})
		waitFor(t, 5*time.Second, "import suppress window to end", func() bool { return !e.gs.suppress.Load() })
		if got := readFile(t, sidecar); !strings.Contains(got, "threshold: "+want) {
			t.Errorf("re-rendered sidecar lost threshold %s:\n%s", want, got)
		}
		if !e.gs.isPullRewrite("app1/simpledeploy.yml") {
			t.Error("re-rendered sidecar not recorded as a rewrite of pulled config")
		}
		if hasConflict(e.gs, "app1/simpledeploy.yml") {
			t.Errorf("rewrite of pulled config reported as a conflict: %+v", e.gs.Status().RecentConflicts)
		}
		if got := remoteHead(t, e.bareDir); got != remote {
			t.Errorf("pull-only mode pushed: remote main %s, want %s", got, remote)
		}
	}
	if got := gitOut(t, e.bareDir, "show", "main:app1/simpledeploy.yml"); got+"\n" != handFormattedAliceSidecar("60") {
		t.Errorf("remote sidecar changed by the server:\n%s", got)
	}
}

// TestFailedApplyIsRetried: a pull blocked by a change git sync does not
// commit (an edited tracked file outside the managed set) reports the
// refusal, keeps CommitsBehind, and applies on a later sync once the file is
// restored, even though the remote did not change again.
func TestFailedApplyIsRetried(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) { seedApp(t, e.st, e.appsDir, "app1") }, pullOnly)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "README.md"), "repo notes\n")
	})
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("first SyncNow: %v", err)
	}
	writeFile(t, filepath.Join(e.appsDir, "README.md"), "edited by hand\n")
	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
	})

	for i := range 2 {
		err := e.gs.SyncNow(e.ctx)
		if err == nil || !strings.Contains(err.Error(), "rebase refused") {
			t.Fatalf("SyncNow #%d error = %v, want rebase refused", i+1, err)
		}
		st := e.gs.Status()
		if !strings.Contains(st.LastSyncError, "rebase refused") || st.CommitsBehind != 1 {
			t.Fatalf("status #%d = error %q, behind %d", i+1, st.LastSyncError, st.CommitsBehind)
		}
	}
	if out := gitOut(t, e.appsDir, "ls-files", "--", "README.md"); out != "README.md" {
		t.Fatalf("README.md not tracked: %q", out)
	}

	gitOut(t, e.appsDir, "checkout", "--", "README.md")
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow after restoring README.md: %v", err)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml")); !strings.Contains(got, "nginx:2") {
		t.Errorf("pending remote change not applied on retry: %q", got)
	}
	if st := e.gs.Status(); st.LastSyncError != "" || st.CommitsBehind != 0 {
		t.Errorf("status after retry = error %q, behind %d", st.LastSyncError, st.CommitsBehind)
	}
}

// TestEnsureRepoIdentity: a repo with no git identity at any level gets a
// repo-level one (rebasing local commits needs a committer); an identity
// that is already set is left alone.
func TestEnsureRepoIdentity(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "gitconfig")
	writeFile(t, empty, "")
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	dir := t.TempDir()
	gitOut(t, dir, "init", "-q", "-b", "main")
	ensureRepoIdentity(dir, "Bot", "bot@example.com")
	if got := gitOut(t, dir, "config", "--local", "--get", "user.email"); got != "bot@example.com" {
		t.Errorf("user.email = %q, want bot@example.com", got)
	}
	if got := gitOut(t, dir, "config", "--local", "--get", "user.name"); got != "Bot" {
		t.Errorf("user.name = %q, want Bot", got)
	}

	gitOut(t, dir, "config", "user.email", "ops@example.com")
	ensureRepoIdentity(dir, "Bot", "bot@example.com")
	if got := gitOut(t, dir, "config", "--get", "user.email"); got != "ops@example.com" {
		t.Errorf("existing user.email overwritten: %q", got)
	}
}

// TestAutoPushPullPushesPendingChanges: with auto-push on, a change that
// never got a commit is committed before the rebase and pushed after it.
func TestAutoPushPullPushesPendingChanges(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) {
		seedApp(t, e.st, e.appsDir, "app1")
		seedApp(t, e.st, e.appsDir, "app2")
	}, nil)

	writeFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml"), "services:\n  web:\n    image: nginx:local\n")
	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app2", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
	})
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app2", "docker-compose.yml")); !strings.Contains(got, "nginx:2") {
		t.Errorf("remote change not applied: %q", got)
	}
	if got := gitOut(t, e.bareDir, "show", "main:app1/docker-compose.yml"); !strings.Contains(got, "nginx:local") {
		t.Errorf("pending local change not pushed: %q", got)
	}
	if head := gitOut(t, e.appsDir, "rev-parse", "HEAD"); head != remoteHead(t, e.bareDir) {
		t.Errorf("local HEAD %s != remote main %s", head, remoteHead(t, e.bareDir))
	}
}

// TestPullOnlyLocalDeletionWinsOverRemoteEdit: an app file deleted locally
// (e.g. the app was removed in the dashboard) and edited on the remote stays
// deleted under the server-wins policy; the pull does not fail.
func TestPullOnlyLocalDeletionWinsOverRemoteEdit(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) {
		seedApp(t, e.st, e.appsDir, "app1")
		seedApp(t, e.st, e.appsDir, "app2")
	}, pullOnly)

	gone := filepath.Join(e.appsDir, "app2", "docker-compose.yml")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	prev := e.gs.Status().HeadSHA
	e.gs.EnqueueCommit(nil, "app deleted: app2")
	waitForHeadUpdate(t, e.gs, prev, 5*time.Second)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app2", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
		writeFile(t, filepath.Join(dir, "app1", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
	})
	remote := remoteHead(t, e.bareDir)

	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Errorf("locally deleted file restored by the pull (stat err %v)", err)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml")); !strings.Contains(got, "nginx:2") {
		t.Errorf("remote change to app1 not applied: %q", got)
	}
	if !hasConflict(e.gs, "app2/docker-compose.yml") {
		t.Errorf("no server-wins conflict recorded: %+v", e.gs.Status().RecentConflicts)
	}
	if rebaseInProgress(e.appsDir) {
		t.Fatal("rebase left in progress")
	}
	if out := gitOut(t, e.appsDir, "status", "--porcelain"); out != "" {
		t.Errorf("working tree not clean: %s", out)
	}
	if got := remoteHead(t, e.bareDir); got != remote {
		t.Errorf("pull-only mode pushed: remote main %s, want %s", got, remote)
	}
}
