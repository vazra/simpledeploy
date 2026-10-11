package reconciler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/store"
)

// TestReconcileDRRecovery verifies that after Reconcile, an app that exists only
// on disk (no DB row) gets its alert rules + backup configs + access grants
// rehydrated from the sidecar file.
func TestReconcileDRRecovery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	appsDir := t.TempDir()
	dataDir := t.TempDir()

	// Write docker-compose.yml for the app.
	appDir := filepath.Join(appsDir, "myapp")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	composeContent := `services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
`
	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte(composeContent), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	// Seed a webhook in DB (needed for alert rule import).
	wh := &store.Webhook{Name: "pagerduty", Type: "slack", URL: "https://pd.example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Seed a user in DB (needed for access grant import).
	user := &store.User{Username: "alice", PasswordHash: "$2a$10$x", Role: "viewer"}
	if err := st.UpsertUserByUsername(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Write the app sidecar with alert rules, backup configs, and access.
	sidecar := configsync.AppSidecar{
		Version: configsync.Version,
		App:     configsync.AppMeta{Slug: "myapp", DisplayName: "My App"},
		AlertRules: []configsync.AlertRuleEntry{
			{Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 120, Webhook: "pagerduty", Enabled: true},
		},
		BackupConfigs: []configsync.BackupConfigEntry{
			{Strategy: "volume", Target: "local", ScheduleCron: "0 3 * * *", RetentionMode: "count", RetentionCount: 5},
		},
		Access: []configsync.AccessEntry{
			{Username: "alice"},
		},
	}
	sidecarPath := filepath.Join(appDir, "simpledeploy.yml")
	if err := writeSidecarYAML(sidecarPath, sidecar); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	syncer := configsync.New(st, appsDir, dataDir)
	t.Cleanup(func() { syncer.Close() })

	mock := &mockDeployer{}
	r := New(st, mock, proxy.NewMockProxy(), appsDir, nil, syncer)

	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// App row must exist.
	app, err := st.GetAppBySlug("myapp")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}

	// Alert rules must be imported.
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("ListAlertRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Metric != "cpu" {
		t.Errorf("expected 1 cpu alert rule, got %v", rules)
	}

	// Backup configs must be imported.
	cfgs, err := st.ListBackupConfigs(&app.ID)
	if err != nil {
		t.Fatalf("ListBackupConfigs: %v", err)
	}
	if len(cfgs) != 1 || cfgs[0].Strategy != "volume" {
		t.Errorf("expected 1 volume backup config, got %v", cfgs)
	}

	// Access grants must be imported.
	usernames, err := st.ListAccessForApp(app.ID)
	if err != nil {
		t.Fatalf("ListAccessForApp: %v", err)
	}
	if len(usernames) != 1 || usernames[0] != "alice" {
		t.Errorf("expected access grant for alice, got %v", usernames)
	}
}

func writeSidecarYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// TestReconcilerArchivesOnComposeDelete verifies that when a compose file
// is removed from disk, Reconcile archives the app row (sets archived_at)
// and writes a tombstone instead of deleting.
func TestReconcilerArchivesOnComposeDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	appsDir := t.TempDir()
	dataDir := t.TempDir()

	appDir := filepath.Join(appsDir, "webapp")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	composeContent := "services:\n  web:\n    image: nginx:latest\n"
	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	sidecarPath := filepath.Join(appDir, "simpledeploy.yml")
	sidecarContent := "version: 1\napp:\n  slug: webapp\n  display_name: Web App\n"
	if err := os.WriteFile(sidecarPath, []byte(sidecarContent), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	syncer := configsync.New(st, appsDir, dataDir)
	t.Cleanup(func() { syncer.Close() })

	mock := &mockDeployer{}
	r := New(st, mock, proxy.NewMockProxy(), appsDir, nil, syncer)

	// First reconcile: app should be discovered and added to DB.
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if _, err := st.GetAppBySlug("webapp"); err != nil {
		t.Fatalf("app should be in DB after first reconcile: %v", err)
	}

	// Delete the compose file to simulate app removal.
	if err := os.Remove(composePath); err != nil {
		t.Fatalf("remove compose: %v", err)
	}

	// Second reconcile: app should be archived (row remains, archived_at set,
	// tombstone written).
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	app, err := st.GetAppBySlug("webapp")
	if err != nil {
		t.Fatalf("expected app row to remain after archive: %v", err)
	}
	if !app.ArchivedAt.Valid {
		t.Fatal("expected ArchivedAt to be set after archive")
	}
	tombPath := filepath.Join(syncer.ArchiveDir(), "webapp.yml")
	if _, err := os.Stat(tombPath); err != nil {
		t.Fatalf("expected tombstone at %s: %v", tombPath, err)
	}
}

// TestReconcileDRAfterBootPrune runs a live install, wipes its DB, then the
// serve boot order (global import, orphan prune, seed, FS reload, initial
// Reconcile). The app's alert rules, backup configs and access grants must
// come back from simpledeploy.yml, no sidecar may be deleted, and archived
// apps must stay archived.
func TestReconcileDRAfterBootPrune(t *testing.T) {
	ctx := context.Background()
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "simpledeploy.db")
	for _, slug := range []string{"web", "gone", "nocompose"} {
		writeComposeFile(t, appsDir, slug)
	}

	// Live install.
	st1, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	cs1 := configsync.New(st1, appsDir, dataDir)
	r1 := New(st1, &mockDeployer{}, nil, appsDir, nil, cs1)
	if err := r1.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	web, err := st1.GetAppBySlug("web")
	if err != nil {
		t.Fatal(err)
	}
	wh := &store.Webhook{Name: "ops", Type: "slack", URL: "https://hooks.example.com/ops"}
	if err := st1.CreateWebhook(wh); err != nil {
		t.Fatal(err)
	}
	editor, err := st1.CreateUser("editor", "$2a$10$editor", "manage", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st1.GrantAppAccess(editor.ID, web.ID); err != nil {
		t.Fatal(err)
	}
	if err := st1.CreateAlertRule(&store.AlertRule{AppID: &web.ID, Metric: "cpu_pct", Operator: ">", Threshold: 90, DurationSec: 300, WebhookID: wh.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := st1.CreateBackupConfig(&store.BackupConfig{AppID: web.ID, Strategy: "volume", Target: "s3", ScheduleCron: "0 3 * * *", TargetConfigJSON: "enc:target", RetentionMode: "count", RetentionCount: 7}); err != nil {
		t.Fatal(err)
	}
	if err := configsync.RunFirstBootSeedIfNeeded(ctx, st1, cs1, nil); err != nil {
		t.Fatal(err)
	}
	// Archive two apps: one folder removed, one with only its compose removed.
	if err := os.RemoveAll(filepath.Join(appsDir, "gone")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(appsDir, "nocompose", "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	if err := r1.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	_ = cs1.Close()
	st1.Close()

	// Disaster: the DB is lost; data_dir and apps_dir survive.
	for _, f := range []string{"simpledeploy.db", "simpledeploy.db-wal", "simpledeploy.db-shm"} {
		if err := os.Remove(filepath.Join(dataDir, f)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cs := configsync.New(st, appsDir, dataDir)
	if imported, err := cs.ImportGlobalIfEmpty(); err != nil || !imported {
		t.Fatalf("ImportGlobalIfEmpty = %v, %v", imported, err)
	}
	if pruned, err := cs.PruneOrphanSidecars(); err != nil || len(pruned) != 0 {
		t.Fatalf("PruneOrphanSidecars = %v, %v; want nothing pruned", pruned, err)
	}
	st.SetMutationHook(func(scope store.MutationScope, slug string) {
		if scope == store.ScopeApp && slug != "" {
			cs.ScheduleAppWrite(slug)
		}
	})
	if err := configsync.RunFirstBootSeedIfNeeded(ctx, st, cs, nil); err != nil {
		t.Fatal(err)
	}
	if err := cs.ReconcileDBFromFS(ctx); err != nil {
		t.Fatal(err)
	}
	mock := &mockDeployer{}
	if err := New(st, mock, nil, appsDir, nil, cs).Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cs.Close(); err != nil { // flush debounced sidecar writes
		t.Fatal(err)
	}

	web, err = st.GetAppBySlug("web")
	if err != nil {
		t.Fatalf("web not re-created: %v", err)
	}
	if web.ArchivedAt.Valid {
		t.Error("web archived after recovery")
	}
	rules, _ := st.ListAlertRules(&web.ID)
	if len(rules) != 1 || rules[0].Metric != "cpu_pct" || rules[0].Threshold != 90 {
		t.Errorf("alert rules not restored: %+v", rules)
	}
	backups, _ := st.ListBackupConfigs(&web.ID)
	if len(backups) != 1 || backups[0].Target != "s3" || backups[0].TargetConfigJSON != "enc:target" {
		t.Errorf("backup configs not restored: %+v", backups)
	}
	if access, _ := st.ListAccessForApp(web.ID); len(access) != 1 || access[0] != "editor" {
		t.Errorf("access grants not restored: %v", access)
	}
	sc, err := cs.ReadAppSidecar("web")
	if err != nil || sc == nil || len(sc.AlertRules) != 1 || len(sc.BackupConfigs) != 1 || len(sc.Access) != 1 {
		t.Errorf("web sidecar after recovery = %+v, %v", sc, err)
	}
	if _, err := os.Stat(filepath.Join(appsDir, "nocompose", "simpledeploy.yml")); err != nil {
		t.Errorf("archived app's sidecar removed: %v", err)
	}
	for _, slug := range []string{"gone", "nocompose"} {
		a, err := st.GetAppBySlug(slug)
		if err != nil || !a.ArchivedAt.Valid {
			t.Errorf("%s not restored as archived: %+v, %v", slug, a, err)
		}
		if mock.hasCall("Deploy:" + slug) {
			t.Errorf("archived app %s redeployed", slug)
		}
	}
}
