package configsync

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

// drInstall is a live install's on-disk state: apps_dir with app folders and
// sidecars, data_dir with config.yml + secrets.yml and the SQLite DB.
type drInstall struct {
	appsDir, dataDir string
}

func (d drInstall) dbPath() string { return filepath.Join(d.dataDir, "simpledeploy.db") }

func (d drInstall) open(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(d.dbPath())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// wipeDB deletes the DB files, as in the disaster-recovery procedure.
func (d drInstall) wipeDB(t *testing.T) {
	t.Helper()
	for _, f := range []string{"simpledeploy.db", "simpledeploy.db-wal", "simpledeploy.db-shm"} {
		if err := os.Remove(filepath.Join(d.dataDir, f)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

// seedDRInstall builds an install with app "web" (compose file, alert rule,
// backup config with an encrypted target, access grant for "editor"), runs
// the first-boot seed so every sidecar is on disk, then closes the DB.
func seedDRInstall(t *testing.T) drInstall {
	t.Helper()
	d := drInstall{appsDir: t.TempDir(), dataDir: t.TempDir()}
	st, err := store.Open(d.dbPath())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	writeCompose(t, d.appsDir, "web")
	app := &store.App{Name: "web", Slug: "web", ComposePath: filepath.Join(d.appsDir, "web", composeFileName), Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}
	wh := &store.Webhook{Name: "ops", Type: "slack", URL: "https://hooks.example.com/ops"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("admin", "$2a$10$admin", "super_admin", "", ""); err != nil {
		t.Fatal(err)
	}
	editor, err := st.CreateUser("editor", "$2a$10$editor", "manage", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.GrantAppAccess(editor.ID, app.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAlertRule(&store.AlertRule{AppID: &app.ID, Metric: "cpu_pct", Operator: ">", Threshold: 90, DurationSec: 300, WebhookID: wh.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateBackupConfig(&store.BackupConfig{AppID: app.ID, Strategy: "volume", Target: "s3", ScheduleCron: "0 3 * * *", TargetConfigJSON: "enc:target", RetentionMode: "count", RetentionCount: 7}); err != nil {
		t.Fatal(err)
	}
	if err := RunFirstBootSeedIfNeeded(context.Background(), st, New(st, d.appsDir, d.dataDir), nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return d
}

func writeCompose(t *testing.T, appsDir, slug string) {
	t.Helper()
	dir := filepath.Join(appsDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, composeFileName), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSidecarFile(t *testing.T, appsDir, slug string) string {
	t.Helper()
	dir := filepath.Join(appsDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, appSidecarName)
	if err := os.WriteFile(p, []byte("version: 1\napp:\n  slug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func assertWebRestored(t *testing.T, st *store.Store) {
	t.Helper()
	app, err := st.GetAppBySlug("web")
	if err != nil {
		t.Fatalf("app row: %v", err)
	}
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Metric != "cpu_pct" || rules[0].Threshold != 90 || rules[0].DurationSec != 300 {
		t.Errorf("alert rules not restored: %+v", rules)
	}
	backups, err := st.ListBackupConfigs(&app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].Target != "s3" || backups[0].TargetConfigJSON != "enc:target" {
		t.Errorf("backup configs not restored: %+v", backups)
	}
	access, err := st.ListAccessForApp(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(access, []string{"editor"}) {
		t.Errorf("access grants not restored: %v", access)
	}
}

// TestDRBootRecoversAppSidecars runs the serve boot steps on a wiped DB:
// the app sidecars must survive until the reconciler re-creates the app
// row, and the per-app config must then be imported from them.
func TestDRBootRecoversAppSidecars(t *testing.T) {
	d := seedDRInstall(t)
	d.wipeDB(t)

	st := d.open(t)
	syncer := New(st, d.appsDir, d.dataDir)
	t.Cleanup(func() { _ = syncer.Close() })

	if imported, err := syncer.ImportGlobalIfEmpty(); err != nil || !imported {
		t.Fatalf("ImportGlobalIfEmpty = %v, %v", imported, err)
	}
	pruned, err := syncer.PruneOrphanSidecars()
	if err != nil {
		t.Fatalf("PruneOrphanSidecars: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("pruned %v; an app folder with a compose file is never an orphan", pruned)
	}
	if err := RunFirstBootSeedIfNeeded(context.Background(), st, syncer, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := syncer.ReconcileDBFromFS(context.Background()); err != nil {
		t.Fatalf("ReconcileDBFromFS: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.appsDir, "web", appSidecarName)); err != nil {
		t.Fatalf("app sidecar gone before the reconciler ran: %v", err)
	}

	// What the reconciler does for an app folder with no DB row.
	if err := st.UpsertApp(&store.App{Name: "web", Slug: "web", ComposePath: filepath.Join(d.appsDir, "web", composeFileName), Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	if imported, err := syncer.ImportAppSidecarIfMissing("web"); err != nil || !imported {
		t.Fatalf("ImportAppSidecarIfMissing = %v, %v", imported, err)
	}
	assertWebRestored(t, st)
}

func TestPruneOrphanSidecarsKeepsAppFolders(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// One live app so the DB is not empty.
	writeCompose(t, appsDir, "alive")
	if err := st.UpsertApp(&store.App{Name: "alive", Slug: "alive", ComposePath: filepath.Join(appsDir, "alive", composeFileName), Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	alive := writeSidecarFile(t, appsDir, "alive")

	// App folder with a compose file but no DB row (e.g. lost with the DB).
	writeCompose(t, appsDir, "lost")
	lost := writeSidecarFile(t, appsDir, "lost")

	// Compose file present as a dangling symlink: still an app folder.
	linked := writeSidecarFile(t, appsDir, "linked")
	if err := os.Symlink(filepath.Join(appsDir, "missing.yml"), filepath.Join(appsDir, "linked", composeFileName)); err != nil {
		t.Fatal(err)
	}

	// Archived app (compose removed, tombstone written), row lost with the DB.
	archived := writeSidecarFile(t, appsDir, "archived")
	if err := syncer.writeTombstoneFile("archived", &Tombstone{Version: Version, ArchivedAt: time.Now().UTC(), App: AppMeta{Slug: "archived"}}); err != nil {
		t.Fatal(err)
	}

	// Archived app whose row is still in the DB.
	archivedRow := writeSidecarFile(t, appsDir, "archived-row")
	if err := st.UpsertApp(&store.App{Name: "archived-row", Slug: "archived-row", Status: "stopped"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkAppArchived("archived-row", time.Now()); err != nil {
		t.Fatal(err)
	}

	// Real orphan: sidecar only.
	ghost := writeSidecarFile(t, appsDir, "ghost")

	pruned, err := syncer.PruneOrphanSidecars()
	if err != nil {
		t.Fatalf("PruneOrphanSidecars: %v", err)
	}
	if !slices.Equal(pruned, []string{"ghost"}) {
		t.Errorf("pruned = %v, want [ghost]", pruned)
	}
	for _, p := range []string{alive, lost, linked, archived, archivedRow} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed: %v", p, err)
		}
	}
	if _, err := os.Stat(ghost); !os.IsNotExist(err) {
		t.Errorf("orphan %s kept", ghost)
	}
}

// TestPruneOrphanSidecarsEmptyDB: a fresh or wiped DB cannot tell an orphan
// from an app it lost, so nothing is pruned.
func TestPruneOrphanSidecarsEmptyDB(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	syncer := New(st, appsDir, t.TempDir())
	ghost := writeSidecarFile(t, appsDir, "ghost")

	pruned, err := syncer.PruneOrphanSidecars()
	if err != nil || len(pruned) != 0 {
		t.Fatalf("PruneOrphanSidecars = %v, %v; want nothing pruned", pruned, err)
	}
	if _, err := os.Stat(ghost); err != nil {
		t.Errorf("sidecar removed on an empty DB: %v", err)
	}
}

// TestReconcileDBFromFSSkipsAppsWithoutRow: boot-time reload leaves sidecars
// of apps the DB does not know yet for the reconciler to import, and does
// not create rows itself.
func TestReconcileDBFromFSSkipsAppsWithoutRow(t *testing.T) {
	d := seedDRInstall(t)
	d.wipeDB(t)
	st := d.open(t)
	syncer := New(st, d.appsDir, d.dataDir)
	if _, err := syncer.ImportGlobalIfEmpty(); err != nil {
		t.Fatal(err)
	}
	if err := syncer.ReconcileDBFromFS(context.Background()); err != nil {
		t.Fatalf("ReconcileDBFromFS: %v", err)
	}
	if _, err := st.GetAppBySlug("web"); err == nil {
		t.Error("ReconcileDBFromFS created an app row; the reconciler owns that")
	}
	if _, err := os.Stat(filepath.Join(d.appsDir, "web", appSidecarName)); err != nil {
		t.Errorf("sidecar removed: %v", err)
	}
}

// TestDRImportFailureKeepsGlobalSidecar: when importing config.yml into an
// empty DB fails, the first-boot seed must not overwrite config.yml with the
// empty or incomplete DB.
func TestDRImportFailureKeepsGlobalSidecar(t *testing.T) {
	cases := []struct {
		name   string
		config string
	}{
		{"malformed yaml", "version: 1\nusers:\n  - username: admin\n    role: [super_admin\nregistries:\n  - name: ghcr\n"},
		// The API key's user is missing: users import, then the key fails
		// before registries.
		{"dangling api key", "version: 1\nusers:\n  - username: admin\n    role: super_admin\napi_keys:\n  - username: gone\n    name: ci\nregistries:\n  - id: reg-1\n    name: ghcr\n    url: ghcr.io\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			dataDir := t.TempDir()
			syncer := New(st, t.TempDir(), dataDir)
			path := filepath.Join(dataDir, globalSidecar)
			if err := os.WriteFile(path, []byte(tc.config), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := syncer.ImportGlobalIfEmpty(); err == nil {
				t.Fatal("expected ImportGlobalIfEmpty to fail")
			}
			if err := RunFirstBootSeedIfNeeded(context.Background(), st, syncer, nil); err != nil {
				t.Fatalf("seed: %v", err)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.config {
				t.Errorf("config.yml rewritten after a failed import:\n%s", raw)
			}
		})
	}
}
