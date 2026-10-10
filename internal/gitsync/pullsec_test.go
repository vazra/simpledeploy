package gitsync

// Tests for treating content pulled from the git remote as less trusted than
// the dashboard: committed symlinks, symlinked managed paths, _global.yml and
// access grants.

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/store"
)

type secEnv struct {
	appsDir string
	bareDir string
	st      *store.Store
	cs      *configsync.Syncer
	gs      *Syncer
	recs    *atomic.Int64
	rec     *recordingReconciler
	ctx     context.Context
}

// newSecEnv builds a store-backed configsync + gitsync pair over a local bare
// remote. seed runs before Start so its files land in the initial commit.
func newSecEnv(t *testing.T, seed func(e *secEnv), mutate func(*Config)) *secEnv {
	t.Helper()
	e := &secEnv{
		appsDir: makeAppsDir(t),
		bareDir: makeBareRemote(t),
		st:      openStore(t),
		recs:    &atomic.Int64{},
	}
	e.rec = &recordingReconciler{count: e.recs}
	e.cs = configsync.New(e.st, e.appsDir, t.TempDir())
	t.Cleanup(func() { e.cs.Close() })
	// Alert rules in pulled sidecars reference this webhook.
	if err := e.st.CreateWebhook(&store.Webhook{Name: "ops", Type: "slack", URL: "https://example.com/hook"}); err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if seed != nil {
		seed(e)
	}
	cfg := Config{
		Enabled:          true,
		Remote:           "file://" + e.bareDir,
		Branch:           "main",
		AppsDir:          e.appsDir,
		AuthorName:       "Test",
		AuthorEmail:      "test@test.local",
		AutoPushEnabled:  true,
		AutoApplyEnabled: true,
		WebhookEnabled:   true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	gs, err := New(cfg, e.st, e.cs, e.rec)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.gs = gs
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	e.ctx = ctx
	if err := gs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = gs.Stop() })
	return e
}

// pushFromClone clones the bare remote, lets edit modify the clone, then
// commits everything and pushes to main.
func pushFromClone(t *testing.T, bareDir string, edit func(dir string)) {
	t.Helper()
	dir := t.TempDir()
	if out, err := gitExec(dir, "clone", "-q", "-b", "main", "file://"+bareDir, "."); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	_, _ = gitExec(dir, "config", "user.email", "remote@t.local")
	_, _ = gitExec(dir, "config", "user.name", "remote")
	edit(dir)
	if out, err := gitExec(dir, "add", "-A", "-f"); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if out, err := gitExec(dir, "commit", "-q", "-m", "remote edit"); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	if out, err := gitExec(dir, "push", "-q", "origin", "HEAD:main"); err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
}

func seedApp(t *testing.T, st *store.Store, appsDir, slug string) *store.App {
	t.Helper()
	compose := filepath.Join(appsDir, slug, "docker-compose.yml")
	writeFile(t, compose, "services:\n  web:\n    image: nginx\n")
	if err := st.UpsertApp(&store.App{Name: slug, Slug: slug, ComposePath: compose, Status: "running"}, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	app, err := st.GetAppBySlug(slug)
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	return app
}

func seedUser(t *testing.T, st *store.Store, username, role string) *store.User {
	t.Helper()
	u, err := st.CreateUser(username, "hash-"+username, role, "", "")
	if err != nil {
		t.Fatalf("CreateUser %s: %v", username, err)
	}
	return u
}

func accessFor(t *testing.T, st *store.Store, appID int64) []string {
	t.Helper()
	users, err := st.ListAccessForApp(appID)
	if err != nil {
		t.Fatalf("ListAccessForApp: %v", err)
	}
	return users
}

func sidecarAccess(t *testing.T, cs *configsync.Syncer, slug string) []string {
	t.Helper()
	sc, err := cs.ReadAppSidecar(slug)
	if err != nil || sc == nil {
		t.Fatalf("ReadAppSidecar %s: %v (nil=%v)", slug, err, sc == nil)
	}
	var out []string
	for _, a := range sc.Access {
		out = append(out, a.Username)
	}
	return out
}

const pulledSidecarWithBob = `version: 1
app:
  slug: app1
  display_name: app1
alert_rules:
  - metric: cpu_pct
    operator: ">"
    threshold: 90
    duration_sec: 60
    webhook: ops
    enabled: true
access:
  - username: bob
`

// TestPullChecksOutCommittedSymlinkAsFile: a symlink committed on the remote
// must land as a plain file (core.symlinks=false), never as a link that could
// redirect reads of a managed file to an arbitrary host path. The app is
// still blocked because git tracks the path as a symlink.
func TestPullChecksOutCommittedSymlinkAsFile(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "host-secret")
	writeFile(t, secret, "TOP SECRET\n")

	e := newSecEnv(t, func(e *secEnv) {
		writeFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml"), "services: {}\n")
	}, nil)

	if out, err := gitExec(e.appsDir, "config", "--get", "core.symlinks"); err != nil || strings.TrimSpace(string(out)) != "false" {
		t.Fatalf("core.symlinks = %q (err %v), want false", out, err)
	}

	pushFromClone(t, e.bareDir, func(dir string) {
		// Commit a symlink entry directly in the index so the test does not
		// depend on the clone's own symlink settings.
		linkText := filepath.Join(t.TempDir(), "linktext")
		if err := os.WriteFile(linkText, []byte(secret), 0600); err != nil {
			t.Fatal(err)
		}
		out, err := gitExec(dir, "hash-object", "-w", linkText)
		if err != nil {
			t.Fatalf("hash-object: %v\n%s", err, out)
		}
		blob := strings.TrimSpace(string(out))
		if out, err := gitExec(dir, "update-index", "--add", "--cacheinfo", "120000,"+blob+",app1/docker-compose.yml"); err != nil {
			t.Fatalf("update-index: %v\n%s", err, out)
		}
		_ = os.Remove(filepath.Join(dir, "app1", "docker-compose.yml"))
		if err := os.Symlink(secret, filepath.Join(dir, "app1", "docker-compose.yml")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	})

	err := e.gs.SyncNow(e.ctx)
	if err == nil || !strings.Contains(err.Error(), "app1/docker-compose.yml") {
		t.Fatalf("SyncNow error = %v, want symlink refusal naming app1/docker-compose.yml", err)
	}

	p := filepath.Join(e.appsDir, "app1", "docker-compose.yml")
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("committed symlink was checked out as a symlink")
	}
	if got := readFile(t, p); strings.Contains(got, "TOP SECRET") || got != secret {
		t.Fatalf("checked-out file content = %q, want link text %q", got, secret)
	}
}

// TestInitRepoSetsCoreSymlinksOnExistingRepo: an existing apps_dir repo gets
// core.symlinks=false on the next start.
func TestInitRepoSetsCoreSymlinksOnExistingRepo(t *testing.T) {
	appsDir := makeAppsDir(t)
	bareDir := makeBareRemote(t)
	writeFile(t, filepath.Join(appsDir, "app1", "docker-compose.yml"), "services: {}\n")

	s := makeSyncer(t, appsDir, bareDir)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = s.Stop()

	if out, err := gitExec(appsDir, "config", "core.symlinks", "true"); err != nil {
		t.Fatalf("reset core.symlinks: %v\n%s", err, out)
	}

	s2 := makeSyncer(t, appsDir, bareDir)
	if err := s2.Start(context.Background()); err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer s2.Stop()
	out, err := gitExec(appsDir, "config", "--get", "core.symlinks")
	if err != nil || strings.TrimSpace(string(out)) != "false" {
		t.Fatalf("core.symlinks after restart = %q (err %v), want false", out, err)
	}
}

// TestPullRefusesSymlinkedManagedPath: when a managed path of an app the pull
// touched is a symlink, even one git does not track, the app's sidecar is not
// imported, reconcile is not triggered, and the error is surfaced in Status.
func TestPullRefusesSymlinkedManagedPath(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.env")
	writeFile(t, outside, "SECRET=1\n")

	var app *store.App
	e := newSecEnv(t, func(e *secEnv) {
		app = seedApp(t, e.st, e.appsDir, "app1")
		if err := e.cs.WriteAppSidecar("app1"); err != nil {
			t.Fatalf("WriteAppSidecar: %v", err)
		}
	}, nil)

	// A symlink at a managed path (e.g. left over from a checkout made before
	// core.symlinks=false, or created by hand).
	if err := os.Symlink(outside, filepath.Join(e.appsDir, "app1", ".env")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if out, _ := gitExec(e.appsDir, "ls-files", "--", "app1/.env"); strings.TrimSpace(string(out)) != "" {
		t.Fatalf("app1/.env unexpectedly tracked: %s", out)
	}

	// The pull touches app1 (not .env itself).
	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), strings.Replace(pulledSidecarWithBob, "access:\n  - username: bob\n", "", 1))
	})

	err := e.gs.SyncNow(e.ctx)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("SyncNow error = %v, want symlink refusal", err)
	}
	if got := e.recs.Load(); got != 0 {
		t.Errorf("reconcile called %d times, want 0", got)
	}
	rules, rerr := e.st.ListAlertRules(&app.ID)
	if rerr != nil {
		t.Fatalf("ListAlertRules: %v", rerr)
	}
	if len(rules) != 0 {
		t.Errorf("sidecar imported despite symlink: %d alert rules", len(rules))
	}
	st := e.gs.Status()
	if !strings.Contains(st.LastSyncError, "symlink") || !strings.Contains(st.LastSyncError, "app1/.env") {
		t.Errorf("LastSyncError = %q, want symlink message naming app1/.env", st.LastSyncError)
	}
	if got := readFile(t, outside); got != "SECRET=1\n" {
		t.Errorf("symlink target modified: %q", got)
	}
}

// TestPullDoesNotApplyGlobalYml: users, roles, registries and DB backup
// settings never change because of a pulled _global.yml.
func TestPullDoesNotApplyGlobalYml(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) {
		seedUser(t, e.st, "alice", "viewer")
		if err := e.cs.WriteRedactedGlobal(); err != nil {
			t.Fatalf("WriteRedactedGlobal: %v", err)
		}
	}, nil)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "_global.yml"), `version: 1
users:
  - username: alice
    role: super_admin
  - username: mallory
    role: super_admin
registries:
  - id: evil
    name: evil
    url: registry.evil.example
webhooks:
  - name: exfil
    type: custom
db_backup_schedule: "* * * * *"
db_backup_target: s3
`)
	})

	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}

	alice, err := e.st.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername alice: %v", err)
	}
	if alice.Role != "viewer" {
		t.Errorf("alice role = %q, want viewer", alice.Role)
	}
	if _, err := e.st.GetUserByUsername("mallory"); err == nil {
		t.Error("pulled _global.yml created user mallory")
	}
	if regs, _ := e.st.ListRegistries(); len(regs) != 0 {
		t.Errorf("pulled _global.yml created registries: %+v", regs)
	}
	if whs, _ := e.st.ListWebhooks(); len(whs) != 1 || whs[0].Name != "ops" {
		t.Errorf("pulled _global.yml changed webhooks: %+v", whs)
	}
	if cfg, _ := e.st.GetDBBackupConfig(); cfg["schedule"] != "" || cfg["target"] != "" {
		t.Errorf("pulled _global.yml changed db backup config: %+v", cfg)
	}
	if got := e.recs.Load(); got != 1 {
		t.Errorf("reconcile called %d times, want 1", got)
	}
}

// TestPullDoesNotChangeAccessGrants: a pulled sidecar that adds bob and drops
// alice changes neither the DB grants nor (after restore) the file on disk,
// while other pulled fields (alert rules) still apply. The restore is pushed.
func TestPullDoesNotChangeAccessGrants(t *testing.T) {
	var app *store.App
	e := newSecEnv(t, func(e *secEnv) {
		app = seedApp(t, e.st, e.appsDir, "app1")
		alice := seedUser(t, e.st, "alice", "viewer")
		seedUser(t, e.st, "bob", "viewer")
		if err := e.st.GrantAppAccess(alice.ID, app.ID); err != nil {
			t.Fatalf("GrantAppAccess: %v", err)
		}
		if err := e.cs.WriteAppSidecar("app1"); err != nil {
			t.Fatalf("WriteAppSidecar: %v", err)
		}
	}, nil)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarWithBob)
	})

	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}

	if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("DB access = %v, want [alice]", got)
	}
	rules, err := e.st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("ListAlertRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Metric != "cpu_pct" {
		t.Errorf("alert rules = %+v, want pulled cpu_pct rule", rules)
	}

	// File on disk restored so the watcher / boot reload / DR import cannot
	// apply the pulled list later. Other pulled fields are kept.
	if got := sidecarAccess(t, e.cs, "app1"); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("sidecar access on disk = %v, want [alice]", got)
	}
	if sc, _ := e.cs.ReadAppSidecar("app1"); sc == nil || len(sc.AlertRules) != 1 {
		t.Errorf("restore dropped pulled alert rules: %+v", sc)
	}
	if out, _ := gitExec(e.appsDir, "status", "--porcelain", "--", "app1/simpledeploy.yml"); strings.TrimSpace(string(out)) != "" {
		t.Errorf("restored sidecar left uncommitted: %s", out)
	}

	// Restore pushed back to the remote.
	remote, err := gitExec(e.bareDir, "show", "main:app1/simpledeploy.yml")
	if err != nil {
		t.Fatalf("show remote sidecar: %v\n%s", err, remote)
	}
	if strings.Contains(string(remote), "bob") || !strings.Contains(string(remote), "alice") {
		t.Errorf("remote sidecar after restore:\n%s", remote)
	}

	found := false
	for _, c := range e.gs.Status().RecentConflicts {
		if c.Path == "app1/simpledeploy.yml" && strings.Contains(c.Description, "access") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an access conflict entry, got %+v", e.gs.Status().RecentConflicts)
	}
}

// TestPullOnlyModeRestoresAccessLocally: with auto-push off the restore is
// committed locally (never pushed) and later pulls still rebase cleanly.
func TestPullOnlyModeRestoresAccessLocally(t *testing.T) {
	var app *store.App
	e := newSecEnv(t, func(e *secEnv) {
		app = seedApp(t, e.st, e.appsDir, "app1")
		alice := seedUser(t, e.st, "alice", "viewer")
		seedUser(t, e.st, "bob", "viewer")
		if err := e.st.GrantAppAccess(alice.ID, app.ID); err != nil {
			t.Fatalf("GrantAppAccess: %v", err)
		}
		if err := e.cs.WriteAppSidecar("app1"); err != nil {
			t.Fatalf("WriteAppSidecar: %v", err)
		}
	}, func(c *Config) { c.AutoPushEnabled = false })

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarWithBob)
	})
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
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
	remote, _ := gitExec(e.bareDir, "show", "main:app1/simpledeploy.yml")
	if !strings.Contains(string(remote), "bob") {
		t.Errorf("pull-only mode pushed the restore:\n%s", remote)
	}

	// A later unrelated remote change still applies.
	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "docker-compose.yml"), "services:\n  web:\n    image: nginx:2\n")
	})
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("second SyncNow: %v", err)
	}
	if got := readFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml")); !strings.Contains(got, "nginx:2") {
		t.Errorf("second pull not applied: %q", got)
	}
	if got := sidecarAccess(t, e.cs, "app1"); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("sidecar access after second pull = %v, want [alice]", got)
	}
	if got := e.gs.Status().CommitsBehind; got != 0 {
		t.Errorf("CommitsBehind = %d, want 0", got)
	}
}

// TestPullNewAppSidecarAccessStripped: a new app added on the remote cannot
// ship access grants; the later DR-style import (reconciler deploy path)
// imports its alert rules but no access.
func TestPullNewAppSidecarAccessStripped(t *testing.T) {
	e := newSecEnv(t, func(e *secEnv) {
		seedUser(t, e.st, "bob", "viewer")
		writeFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml"), "services: {}\n")
	}, nil)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "newapp", "docker-compose.yml"), "services:\n  web:\n    image: nginx\n")
		writeFile(t, filepath.Join(dir, "newapp", "simpledeploy.yml"), strings.ReplaceAll(pulledSidecarWithBob, "app1", "newapp"))
	})
	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if got := sidecarAccess(t, e.cs, "newapp"); len(got) != 0 {
		t.Errorf("new app sidecar access on disk = %v, want none", got)
	}

	// Reconciler deploys the new app and runs the DR import.
	app := seedApp(t, e.st, e.appsDir, "newapp")
	imported, err := e.cs.ImportAppSidecarIfMissing("newapp")
	if err != nil || !imported {
		t.Fatalf("ImportAppSidecarIfMissing = %v, %v", imported, err)
	}
	if got := accessFor(t, e.st, app.ID); len(got) != 0 {
		t.Errorf("new app access = %v, want none", got)
	}
	if rules, _ := e.st.ListAlertRules(&app.ID); len(rules) != 1 {
		t.Errorf("new app alert rules = %+v, want 1", rules)
	}
}

// TestPushRetryRebaseRestoresAccess: remote changes that arrive through the
// push retry rebase (local commit + diverged remote) get the same access
// restore as a regular pull.
func TestPushRetryRebaseRestoresAccess(t *testing.T) {
	var app *store.App
	e := newSecEnv(t, func(e *secEnv) {
		app = seedApp(t, e.st, e.appsDir, "app1")
		alice := seedUser(t, e.st, "alice", "viewer")
		seedUser(t, e.st, "bob", "viewer")
		if err := e.st.GrantAppAccess(alice.ID, app.ID); err != nil {
			t.Fatalf("GrantAppAccess: %v", err)
		}
		if err := e.cs.WriteAppSidecar("app1"); err != nil {
			t.Fatalf("WriteAppSidecar: %v", err)
		}
	}, nil)

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarWithBob)
	})

	// Local change; its push is rejected (non-fast-forward) and retried
	// after fetch + rebase.
	writeFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml"), "services:\n  web:\n    image: nginx:local\n")
	prev := e.gs.Status().HeadSHA
	e.gs.EnqueueCommit([]string{filepath.Join(e.appsDir, "app1", "docker-compose.yml")}, "local edit")
	drainCommits(t, e.gs, 10*time.Second)
	waitForHeadUpdate(t, e.gs, prev, 10*time.Second)

	deadline := time.Now().Add(10 * time.Second)
	var remote []byte
	for time.Now().Before(deadline) {
		remote, _ = gitExec(e.bareDir, "show", "main:app1/docker-compose.yml")
		if strings.Contains(string(remote), "nginx:local") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(string(remote), "nginx:local") {
		t.Fatalf("local change never reached the remote: %s", remote)
	}
	if got := sidecarAccess(t, e.cs, "app1"); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("sidecar access on disk = %v, want [alice]", got)
	}
	remoteSidecar, _ := gitExec(e.bareDir, "show", "main:app1/simpledeploy.yml")
	if strings.Contains(string(remoteSidecar), "bob") {
		t.Errorf("remote sidecar still grants bob:\n%s", remoteSidecar)
	}
	if got := accessFor(t, e.st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("DB access = %v, want [alice]", got)
	}
}

func TestAppSidecarSlug(t *testing.T) {
	cases := map[string]string{
		"app1/simpledeploy.yml":   "app1",
		"simpledeploy.yml":        "",
		"a/b/simpledeploy.yml":    "",
		".git/simpledeploy.yml":   "",
		"app1/docker-compose.yml": "",
	}
	for in, want := range cases {
		got, ok := appSidecarSlug(in)
		if got != want || ok != (want != "") {
			t.Errorf("appSidecarSlug(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestFindManagedSymlinks(t *testing.T) {
	appsDir := t.TempDir()
	target := t.TempDir()
	writeFile(t, filepath.Join(appsDir, "ok", "docker-compose.yml"), "x")
	writeFile(t, filepath.Join(appsDir, "bad", "docker-compose.yml"), "x")
	writeFile(t, filepath.Join(appsDir, "quiet", "docker-compose.yml"), "x")
	link := func(dst, rel string) {
		t.Helper()
		if err := os.Symlink(dst, filepath.Join(appsDir, rel)); err != nil {
			t.Fatal(err)
		}
	}
	link(filepath.Join(target, "f"), "bad/simpledeploy.yml")
	link(filepath.Join(target, "e"), "quiet/.env")
	link(target, "linkedapp")
	link(filepath.Join(target, "g"), "_global.yml")
	link(filepath.Join(target, "h"), ".gitignore")

	cases := []struct {
		name  string
		scope symlinkScope
		want  map[string][]string
	}{
		{
			name: "touched apps and changed root files",
			scope: symlinkScope{changed: []string{
				"ok/docker-compose.yml", "bad/docker-compose.yml", "_global.yml", ".github/workflows/x.yml",
			}},
			want: map[string][]string{"bad": {"bad/simpledeploy.yml"}, "": {"_global.yml"}},
		},
		{
			name:  "untracked links the pull did not touch",
			scope: symlinkScope{changed: []string{"ok/simpledeploy.yml"}},
			want:  map[string][]string{},
		},
		{
			name:  "touched symlinked app folder",
			scope: symlinkScope{changed: []string{"linkedapp/docker-compose.yml"}},
			want:  map[string][]string{"linkedapp": {"linkedapp"}},
		},
		{
			name: "tracked symlinks regardless of diff",
			scope: symlinkScope{
				changed: []string{"ok/docker-compose.yml"},
				tracked: []string{"ok/data/link", "quiet/.env", ".gitignore", ".github/link"},
			},
			want: map[string][]string{"ok": {"ok/data/link"}, "quiet": {"quiet/.env"}, "": {".gitignore"}},
		},
		{
			name:  "all when the diff is unknown",
			scope: symlinkScope{all: true},
			want: map[string][]string{
				"bad":       {"bad/simpledeploy.yml"},
				"quiet":     {"quiet/.env"},
				"linkedapp": {"linkedapp"},
				"":          {".gitignore", "_global.yml"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := findManagedSymlinks(appsDir, tc.scope)
			if err != nil {
				t.Fatalf("findManagedSymlinks: %v", err)
			}
			if !maps.EqualFunc(got, tc.want, slices.Equal[[]string]) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	got, _ := findManagedSymlinks(appsDir, symlinkScope{changed: []string{"_global.yml"}})
	if msg := symlinkMessage(got); !strings.Contains(msg, "all apps") {
		t.Errorf("root symlink should block all apps: %s", msg)
	}
}

// pushFromSymlinkClone is pushFromClone for an operator clone with
// core.symlinks=true, so real symlinks created by edit are committed as
// mode 120000. It bypasses gitExec's core.symlinks=false override.
func pushFromSymlinkClone(t *testing.T, bareDir string, edit func(dir string)) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "core.symlinks=true", "-C", dir}, args...)...)
		cmd.Env = scrubbedGitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("clone", "-q", "-b", "main", "file://"+bareDir, ".")
	run("config", "core.symlinks", "true")
	run("config", "user.email", "remote@t.local")
	run("config", "user.name", "remote")
	edit(dir)
	run("add", "-A", "-f")
	run("commit", "-q", "-m", "remote symlink")
	run("push", "-q", "origin", "HEAD:main")
}

const pulledSidecarNoAccess = `version: 1
app:
  slug: app1
  display_name: app1
alert_rules:
  - metric: cpu_pct
    operator: ">"
    threshold: 90
    duration_sec: 60
    webhook: ops
    enabled: true
`

// TestPullIgnoresUntrackedSymlinksElsewhere: symlinks an operator made by
// hand that git does not track and the pull did not touch (a symlinked app
// folder, a symlinked .env in another app, a symlinked _global.yml) block
// neither the pull nor other apps.
func TestPullIgnoresUntrackedSymlinksElsewhere(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "docker-compose.yml"), "services: {}\n")
	writeFile(t, filepath.Join(outside, ".env"), "SECRET=1\n")
	writeFile(t, filepath.Join(outside, "_global.yml"), "version: 1\n")

	var app1 *store.App
	e := newSecEnv(t, func(e *secEnv) {
		app1 = seedApp(t, e.st, e.appsDir, "app1")
		seedApp(t, e.st, e.appsDir, "app2")
		if err := e.cs.WriteAppSidecar("app1"); err != nil {
			t.Fatalf("WriteAppSidecar: %v", err)
		}
	}, nil)

	for dst, rel := range map[string]string{
		outside:                               "linked",
		filepath.Join(outside, ".env"):        "app2/.env",
		filepath.Join(outside, "_global.yml"): "_global.yml",
	} {
		if err := os.Symlink(dst, filepath.Join(e.appsDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("symlink %s: %v", rel, err)
		}
	}

	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarNoAccess)
	})

	if err := e.gs.SyncNow(e.ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if msg := e.gs.Status().LastSyncError; msg != "" {
		t.Errorf("LastSyncError = %q, want none", msg)
	}
	if rules, _ := e.st.ListAlertRules(&app1.ID); len(rules) != 1 {
		t.Errorf("app1 alert rules = %+v, want pulled rule imported", rules)
	}
	if got := e.recs.Load(); got != 1 {
		t.Errorf("reconcile called %d times, want 1", got)
	}
}

// TestPullBlocksTrackedSymlinkRegardlessOfDiff: a symlink committed from an
// operator clone with core.symlinks=true is tracked as mode 120000. It is
// checked out as a plain file, and its app stays blocked on the pull that
// adds it and on later pulls that only touch other apps. The other apps'
// sidecars still import.
func TestPullBlocksTrackedSymlinkRegardlessOfDiff(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.env")
	writeFile(t, outside, "SECRET=1\n")

	var app1 *store.App
	e := newSecEnv(t, func(e *secEnv) {
		app1 = seedApp(t, e.st, e.appsDir, "app1")
		seedApp(t, e.st, e.appsDir, "app2")
		if err := e.cs.WriteAppSidecar("app1"); err != nil {
			t.Fatalf("WriteAppSidecar: %v", err)
		}
	}, nil)

	pushFromSymlinkClone(t, e.bareDir, func(dir string) {
		if err := os.Symlink(outside, filepath.Join(dir, "app2", ".env")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	})
	if out, err := gitExec(e.bareDir, "ls-tree", "main", "--", "app2/.env"); err != nil || !strings.HasPrefix(string(out), "120000 ") {
		t.Fatalf("app2/.env not tracked as a symlink: %q (err %v)", out, err)
	}

	err := e.gs.SyncNow(e.ctx)
	if err == nil || !strings.Contains(err.Error(), "app2/.env") {
		t.Fatalf("first SyncNow error = %v, want symlink refusal naming app2/.env", err)
	}
	if fi, lerr := os.Lstat(filepath.Join(e.appsDir, "app2", ".env")); lerr != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("app2/.env checked out as a symlink (lstat err %v)", lerr)
	}

	// The next pull only touches app1; app2's tracked symlink still blocks.
	pushFromClone(t, e.bareDir, func(dir string) {
		writeFile(t, filepath.Join(dir, "app1", "simpledeploy.yml"), pulledSidecarNoAccess)
	})
	err = e.gs.SyncNow(e.ctx)
	if err == nil || !strings.Contains(err.Error(), "app2/.env") || strings.Contains(err.Error(), "app1") {
		t.Fatalf("second SyncNow error = %v, want symlink refusal naming only app2", err)
	}
	if rules, _ := e.st.ListAlertRules(&app1.ID); len(rules) != 1 {
		t.Errorf("app1 alert rules = %+v, want pulled rule imported", rules)
	}
	// Only the second pull changed an unblocked app, so it alone reconciles.
	if got := e.recs.Load(); got != 1 {
		t.Errorf("reconcile called %d times, want 1", got)
	}
}

// TestStartToleratesRepoSafetyConfigFailure: when system git cannot write
// the repo config (a stale .git/config.lock here; "dubious ownership" in the
// field), Start still succeeds, and pulls stay safe because every git call
// passes -c core.symlinks=false.
func TestStartToleratesRepoSafetyConfigFailure(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "host-secret")
	writeFile(t, secret, "TOP SECRET\n")

	e := newSecEnv(t, func(e *secEnv) {
		writeFile(t, filepath.Join(e.appsDir, "app1", "docker-compose.yml"), "services: {}\n")
	}, nil)
	_ = e.gs.Stop()

	// Persisted value lost, and git can no longer write the config.
	if out, err := gitExec(e.appsDir, "config", "core.symlinks", "true"); err != nil {
		t.Fatalf("set core.symlinks=true: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(e.appsDir, ".git", "config.lock"), "")
	if out, err := gitExec(e.appsDir, "config", "core.symlinks", "false"); err == nil {
		t.Fatalf("precondition: git config write succeeded despite config.lock: %s", out)
	}

	gs, err := New(e.gs.cfg, e.st, e.cs, &countingReconciler{count: e.recs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := gs.Start(e.ctx); err != nil {
		t.Fatalf("Start with unwritable repo config: %v", err)
	}
	t.Cleanup(func() { _ = gs.Stop() })

	pushFromSymlinkClone(t, e.bareDir, func(dir string) {
		p := filepath.Join(dir, "app1", "docker-compose.yml")
		_ = os.Remove(p)
		if err := os.Symlink(secret, p); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	})
	if err := gs.SyncNow(e.ctx); err == nil || !strings.Contains(err.Error(), "app1/docker-compose.yml") {
		t.Fatalf("SyncNow error = %v, want symlink refusal naming app1/docker-compose.yml", err)
	}
	fi, err := os.Lstat(filepath.Join(e.appsDir, "app1", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("committed symlink checked out as a symlink without the persisted core.symlinks=false")
	}
}
