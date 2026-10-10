package configsync

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func seedAccessFixture(t *testing.T, st *store.Store, slug string) *store.App {
	t.Helper()
	if err := st.UpsertApp(&store.App{Name: slug, Slug: slug, ComposePath: "/apps/" + slug + "/docker-compose.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	app, err := st.GetAppBySlug(slug)
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	alice, err := st.CreateUser("alice", "h", "viewer", "", "")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	if _, err := st.CreateUser("bob", "h", "viewer", "", ""); err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	if err := st.GrantAppAccess(alice.ID, app.ID); err != nil {
		t.Fatalf("GrantAppAccess: %v", err)
	}
	return app
}

func listAccess(t *testing.T, st *store.Store, appID int64) []string {
	t.Helper()
	u, err := st.ListAccessForApp(appID)
	if err != nil {
		t.Fatalf("ListAccessForApp: %v", err)
	}
	return u
}

// TestImportAppSidecarSkipAccess: the git pull path (SkipAccess) never changes
// user_app_access; the default (disaster recovery) import still does.
func TestImportAppSidecarSkipAccess(t *testing.T) {
	st := openTestStore(t)
	syncer := New(st, t.TempDir(), t.TempDir())
	app := seedAccessFixture(t, st, "app1")

	data := &AppSidecar{
		Version: Version,
		App:     AppMeta{Slug: "app1", DisplayName: "app1"},
		Access:  []AccessEntry{{Username: "bob"}},
	}
	if err := syncer.ImportAppSidecarWithOptions(data, ImportOptions{SkipAccess: true}); err != nil {
		t.Fatalf("ImportAppSidecarWithOptions: %v", err)
	}
	if got := listAccess(t, st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Fatalf("SkipAccess import changed access: %v", got)
	}

	if err := syncer.ImportAppSidecar(data); err != nil {
		t.Fatalf("ImportAppSidecar: %v", err)
	}
	if got := listAccess(t, st, app.ID); !slices.Equal(got, []string{"bob"}) {
		t.Fatalf("DR import access = %v, want [bob]", got)
	}
}

// TestImportAppSidecarIfMissingRestoresAccess: the empty-DB (DR) path still
// restores access grants from the sidecar on disk.
func TestImportAppSidecarIfMissingRestoresAccess(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	syncer := New(st, appsDir, t.TempDir())
	if err := st.UpsertApp(&store.App{Name: "app1", Slug: "app1", ComposePath: "/x", Status: "running"}, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	app, _ := st.GetAppBySlug("app1")
	if _, err := st.CreateUser("bob", "h", "viewer", "", ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sc := AppSidecar{Version: Version, App: AppMeta{Slug: "app1"}, Access: []AccessEntry{{Username: "bob"}}}
	if err := atomicWriteYAMLMode(filepath.Join(appsDir, "app1", appSidecarName), 0644, sc); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	imported, err := syncer.ImportAppSidecarIfMissing("app1")
	if err != nil || !imported {
		t.Fatalf("ImportAppSidecarIfMissing = %v, %v", imported, err)
	}
	if got := listAccess(t, st, app.ID); !slices.Equal(got, []string{"bob"}) {
		t.Fatalf("DR access = %v, want [bob]", got)
	}
}

func TestRestoreSidecarAccess(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	syncer := New(st, appsDir, t.TempDir())
	seedAccessFixture(t, st, "app1")

	path := filepath.Join(appsDir, "app1", appSidecarName)
	pulled := AppSidecar{
		Version: Version,
		App:     AppMeta{Slug: "app1", DisplayName: "Pulled Name"},
		AlertRules: []AlertRuleEntry{
			{Metric: "cpu_pct", Operator: ">", Threshold: 90, DurationSec: 60, Webhook: "ops", Enabled: true},
		},
		Access: []AccessEntry{{Username: "bob"}},
	}
	if err := atomicWriteYAMLMode(path, 0644, pulled); err != nil {
		t.Fatal(err)
	}

	changed, err := syncer.RestoreSidecarAccess("app1")
	if err != nil || !changed {
		t.Fatalf("RestoreSidecarAccess = %v, %v; want true, nil", changed, err)
	}
	got, err := syncer.ReadAppSidecar("app1")
	if err != nil || got == nil {
		t.Fatalf("ReadAppSidecar: %v", err)
	}
	if len(got.Access) != 1 || got.Access[0].Username != "alice" {
		t.Errorf("access after restore = %+v, want [alice]", got.Access)
	}
	if got.App.DisplayName != "Pulled Name" || len(got.AlertRules) != 1 {
		t.Errorf("restore changed non-access fields: %+v", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0644 {
		t.Errorf("mode after restore = %o, want 0644", fi.Mode().Perm())
	}

	changed, err = syncer.RestoreSidecarAccess("app1")
	if err != nil || changed {
		t.Fatalf("second RestoreSidecarAccess = %v, %v; want false, nil", changed, err)
	}

	// Duplicates / ordering are not a difference.
	pulled.Access = []AccessEntry{{Username: "alice"}, {Username: "alice"}}
	if err := atomicWriteYAMLMode(path, 0644, pulled); err != nil {
		t.Fatal(err)
	}
	if changed, err := syncer.RestoreSidecarAccess("app1"); err != nil || changed {
		t.Fatalf("duplicate entries: %v, %v; want false, nil", changed, err)
	}

	// App not in DB: no grants at all.
	ghost := AppSidecar{Version: Version, App: AppMeta{Slug: "ghost"}, Access: []AccessEntry{{Username: "bob"}}}
	if err := atomicWriteYAMLMode(filepath.Join(appsDir, "ghost", appSidecarName), 0644, ghost); err != nil {
		t.Fatal(err)
	}
	if changed, err := syncer.RestoreSidecarAccess("ghost"); err != nil || !changed {
		t.Fatalf("ghost: %v, %v; want true, nil", changed, err)
	}
	if g, _ := syncer.ReadAppSidecar("ghost"); g == nil || len(g.Access) != 0 {
		t.Errorf("ghost access after restore = %+v, want none", g)
	}

	// Missing sidecar is a no-op.
	if changed, err := syncer.RestoreSidecarAccess("nope"); err != nil || changed {
		t.Fatalf("missing sidecar: %v, %v; want false, nil", changed, err)
	}
}

// TestApplyAppSidecarDashboardOnlyAccess: with git sync running, the FS->DB
// path (file watcher, boot-time reload) applies the other sidecar fields but
// never access; the DR import into an app without state still restores it.
func TestApplyAppSidecarDashboardOnlyAccess(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	syncer := New(st, appsDir, t.TempDir())
	app := seedAccessFixture(t, st, "app1")
	syncer.SetAccessFromDashboardOnly(true)

	sc := AppSidecar{Version: Version, App: AppMeta{Slug: "app1", DisplayName: "Renamed"}, Access: []AccessEntry{{Username: "bob"}}}
	if err := atomicWriteYAMLMode(filepath.Join(appsDir, "app1", appSidecarName), 0644, sc); err != nil {
		t.Fatal(err)
	}
	if err := syncer.ReconcileDBFromFS(context.Background()); err != nil {
		t.Fatalf("ReconcileDBFromFS: %v", err)
	}
	if got := listAccess(t, st, app.ID); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("access after FS reload = %v, want [alice]", got)
	}
	if got, _ := st.GetAppBySlug("app1"); got == nil || got.Name != "Renamed" {
		t.Errorf("display name not applied: %+v", got)
	}

	// DR import for an app with no DB state restores grants regardless.
	if err := st.UpsertApp(&store.App{Name: "app2", Slug: "app2", ComposePath: "/x", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	app2, _ := st.GetAppBySlug("app2")
	sc2 := AppSidecar{Version: Version, App: AppMeta{Slug: "app2"}, Access: []AccessEntry{{Username: "bob"}}}
	if err := atomicWriteYAMLMode(filepath.Join(appsDir, "app2", appSidecarName), 0644, sc2); err != nil {
		t.Fatal(err)
	}
	if imported, err := syncer.ImportAppSidecarIfMissing("app2"); err != nil || !imported {
		t.Fatalf("ImportAppSidecarIfMissing = %v, %v", imported, err)
	}
	if got := listAccess(t, st, app2.ID); !slices.Equal(got, []string{"bob"}) {
		t.Errorf("DR access = %v, want [bob]", got)
	}

	// Without git sync the file is authoritative again.
	syncer.SetAccessFromDashboardOnly(false)
	loaded, err := syncer.LoadAppFromFS("app1")
	if err != nil {
		t.Fatalf("LoadAppFromFS: %v", err)
	}
	if err := syncer.ApplyAppSidecar("app1", loaded); err != nil {
		t.Fatalf("ApplyAppSidecar: %v", err)
	}
	if got := listAccess(t, st, app.ID); !slices.Equal(got, []string{"bob"}) {
		t.Errorf("access with dashboard-only off = %v, want [bob]", got)
	}
}

func TestRestoreSidecarAccessTo(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	syncer := New(st, appsDir, t.TempDir())
	seedAccessFixture(t, st, "app1")

	sc := AppSidecar{Version: Version, App: AppMeta{Slug: "app1"}, Access: []AccessEntry{{Username: "bob"}}}
	if err := atomicWriteYAMLMode(filepath.Join(appsDir, "app1", appSidecarName), 0644, sc); err != nil {
		t.Fatal(err)
	}
	// The given list wins over the DB (alice).
	changed, err := syncer.RestoreSidecarAccessTo("app1", []string{"carol", "bob", "carol"})
	if err != nil || !changed {
		t.Fatalf("RestoreSidecarAccessTo = %v, %v; want true, nil", changed, err)
	}
	got, _ := syncer.ReadAppSidecar("app1")
	var names []string
	for _, a := range got.Access {
		names = append(names, a.Username)
	}
	if !slices.Equal(names, []string{"bob", "carol"}) {
		t.Errorf("access = %v, want [bob carol]", names)
	}
	if changed, err := syncer.RestoreSidecarAccessTo("app1", []string{"bob", "carol"}); err != nil || changed {
		t.Fatalf("second call = %v, %v; want false, nil", changed, err)
	}
	if changed, err := syncer.RestoreSidecarAccessTo("app1", nil); err != nil || !changed {
		t.Fatalf("empty list = %v, %v; want true, nil", changed, err)
	}
	if got, _ := syncer.ReadAppSidecar("app1"); len(got.Access) != 0 {
		t.Errorf("access after empty restore = %+v, want none", got.Access)
	}
}

// TestSidecarReadersRefuseSymlinks: app sidecar readers refuse symlinked app
// directories, symlinked sidecar files and slugs escaping apps_dir.
func TestSidecarReadersRefuseSymlinks(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	syncer := New(st, appsDir, t.TempDir())

	outside := t.TempDir()
	sc := AppSidecar{Version: Version, App: AppMeta{Slug: "linked"}, Access: []AccessEntry{{Username: "bob"}}}
	if err := atomicWriteYAMLMode(filepath.Join(outside, appSidecarName), 0644, sc); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteYAMLMode(filepath.Join(outside, appSecretsName), 0600, AppSecrets{Version: Version}); err != nil {
		t.Fatal(err)
	}

	// Symlinked app directory.
	if err := os.Symlink(outside, filepath.Join(appsDir, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.ReadAppSidecar("linked"); err == nil {
		t.Error("ReadAppSidecar followed a symlinked app dir")
	}
	if _, err := syncer.LoadAppFromFS("linked"); err == nil {
		t.Error("LoadAppFromFS followed a symlinked app dir")
	}
	if _, err := syncer.ReadAppSecrets("linked"); err == nil {
		t.Error("ReadAppSecrets followed a symlinked app dir")
	}
	if err := st.UpsertApp(&store.App{Name: "linked", Slug: "linked", ComposePath: "/x", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	if imported, err := syncer.ImportAppSidecarIfMissing("linked"); err == nil || imported {
		t.Errorf("ImportAppSidecarIfMissing on a symlinked app dir = %v, %v; want error", imported, err)
	}

	// Symlinked sidecar file inside a real app dir.
	if err := os.MkdirAll(filepath.Join(appsDir, "real"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, appSidecarName), filepath.Join(appsDir, "real", appSidecarName)); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.ReadAppSidecar("real"); err == nil {
		t.Error("ReadAppSidecar followed a symlinked sidecar file")
	}
	if _, err := syncer.RestoreSidecarAccess("real"); err == nil {
		t.Error("RestoreSidecarAccess followed a symlinked sidecar file")
	}

	// Slug escaping apps_dir.
	if _, err := syncer.ReadAppSidecar("../" + filepath.Base(outside)); err == nil {
		t.Error("ReadAppSidecar accepted a slug escaping apps_dir")
	}
}

// TestWriteAppSidecarRefusesSymlinkedAppDir: writes never land outside apps_dir.
func TestWriteAppSidecarRefusesSymlinkedAppDir(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	syncer := New(st, appsDir, t.TempDir())
	if err := st.UpsertApp(&store.App{Name: "linked", Slug: "linked", ComposePath: "/x", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	victim := t.TempDir()
	if err := os.Symlink(victim, filepath.Join(appsDir, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := syncer.WriteAppSidecar("linked"); err == nil {
		t.Fatal("WriteAppSidecar wrote through a symlinked app dir")
	}
	entries, _ := os.ReadDir(victim)
	if len(entries) != 0 {
		t.Fatalf("files written outside apps_dir: %v", entries)
	}
}

// TestAtomicWriteDoesNotFollowSymlinks: a planted "<file>.tmp" symlink (the
// old fixed temp name) or a symlink at the target itself is never followed.
func TestAtomicWriteDoesNotFollowSymlinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, appSidecarName)
	if err := os.Symlink(victim, path+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteYAMLMode(path, 0644, AppSidecar{Version: Version}); err != nil {
		t.Fatalf("atomicWriteYAMLMode: %v", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "original" {
		t.Fatalf("victim modified: %q", b)
	}
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("target not replaced by a regular file: %v %v", fi, err)
	}
}

func TestEnsureGitignoreRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore(dir, []string{"*.secrets.yml"}); err == nil {
		t.Fatal("ensureGitignore followed a symlinked .gitignore")
	}
	if b, _ := os.ReadFile(victim); string(b) != "original\n" {
		t.Fatalf("victim modified: %q", b)
	}
}

func TestEnsureGitignoreKeepsMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(p, []byte("*\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore(dir, []string{"*.secrets.yml"}); err != nil {
		t.Fatalf("ensureGitignore: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", fi.Mode().Perm())
	}
}

func TestReadTombstoneRefusesSymlink(t *testing.T) {
	st := openTestStore(t)
	syncer := New(st, t.TempDir(), t.TempDir())
	if err := os.MkdirAll(syncer.ArchiveDir(), 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "t.yml")
	if err := os.WriteFile(target, []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(syncer.ArchiveDir(), "x.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.ReadTombstone("x"); err == nil {
		t.Fatal("ReadTombstone followed a symlink")
	}
}
