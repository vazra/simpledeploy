package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/reconciler"
	"github.com/vazra/simpledeploy/internal/store"
)

// stubDeployer reports success for every action without running docker.
type stubDeployer struct{}

func (stubDeployer) Deploy(context.Context, *compose.AppConfig, ...deployer.RegistryAuth) deployer.DeployResult {
	return deployer.DeployResult{}
}
func (stubDeployer) RollbackDeploy(context.Context, *compose.AppConfig, int, *int64, ...deployer.RegistryAuth) deployer.DeployResult {
	return deployer.DeployResult{}
}
func (stubDeployer) Teardown(context.Context, string) error { return nil }
func (stubDeployer) Restart(context.Context, *compose.AppConfig) deployer.DeployResult {
	return deployer.DeployResult{}
}
func (stubDeployer) Stop(context.Context, string) error  { return nil }
func (stubDeployer) Start(context.Context, string) error { return nil }
func (stubDeployer) Pull(context.Context, *compose.AppConfig, []deployer.RegistryAuth) deployer.DeployResult {
	return deployer.DeployResult{}
}
func (stubDeployer) Scale(context.Context, *compose.AppConfig, map[string]int) error { return nil }
func (stubDeployer) Status(context.Context, string) ([]deployer.ServiceStatus, error) {
	return nil, nil
}
func (stubDeployer) Cancel(context.Context, *compose.AppConfig) error { return nil }

// bootServe runs the config steps of `serve` in boot order (no git sync)
// up to and including the reconciler's initial pass. The returned func is
// the graceful-shutdown flush.
func bootServe(t *testing.T, cfg *config.Config) (*store.Store, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		t.Fatal(err)
	}
	syncer := configsync.New(db, cfg.AppsDir, cfg.DataDir)
	initConfigSync(db, syncer, cfg.DataDir)
	if err := reloadConfigFromFS(ctx, db, syncer, cfg); err != nil {
		t.Fatalf("reloadConfigFromFS: %v", err)
	}
	if err := reconciler.New(db, stubDeployer{}, nil, cfg.AppsDir, cfg, syncer).Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return db, func() {
		if err := syncer.Close(); err != nil {
			t.Errorf("syncer close: %v", err)
		}
		db.Close()
	}
}

// TestServeBootRecoversConfigAfterDBWipe mirrors the disaster-recovery E2E:
// configure an app, stop, delete the DB, start again. Global config comes
// back from config.yml and the app's alert rules, backup configs and access
// grants from its simpledeploy.yml, which must not be pruned.
func TestServeBootRecoversConfigAfterDBWipe(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		DataDir:      filepath.Join(root, "data"),
		AppsDir:      filepath.Join(root, "apps"),
		MasterSecret: "test-secret-0123456789abcdef0123456789",
	}
	appDir := filepath.Join(cfg.AppsDir, "dr-nginx")
	for _, d := range []string{cfg.DataDir, appDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx:alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run: the reconciler adopts the app, then it is configured.
	db, shutdown := bootServe(t, cfg)
	app, err := db.GetAppBySlug("dr-nginx")
	if err != nil {
		t.Fatalf("app not adopted: %v", err)
	}
	wh := &store.Webhook{Name: "dr-hook", Type: "slack", URL: "http://127.0.0.1:9/dr-hook"}
	if err := db.CreateWebhook(wh); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser("dr-admin", "$2a$10$admin", "super_admin", "", ""); err != nil {
		t.Fatal(err)
	}
	editor, err := db.CreateUser("dr-editor", "$2a$10$editor", "manage", "DR Editor", "editor@dr.local")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GrantAppAccess(editor.ID, app.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateAlertRule(&store.AlertRule{AppID: &app.ID, Metric: "cpu_pct", Operator: ">", Threshold: 90, DurationSec: 300, WebhookID: wh.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateBackupConfig(&store.BackupConfig{AppID: app.ID, Strategy: "volume", Target: "local", ScheduleCron: "0 3 * * *", TargetConfigJSON: "enc:target", RetentionMode: "count", RetentionCount: 7}); err != nil {
		t.Fatal(err)
	}
	shutdown()

	sidecar := filepath.Join(appDir, "simpledeploy.yml")
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("app sidecar not written on shutdown: %v", err)
	}
	for _, f := range []string{"simpledeploy.db", "simpledeploy.db-wal", "simpledeploy.db-shm"} {
		if err := os.Remove(filepath.Join(cfg.DataDir, f)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	// Restart on the empty DB.
	db, shutdown = bootServe(t, cfg)
	defer shutdown()

	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("app sidecar deleted on restart: %v", err)
	}
	u, err := db.GetUserByUsername("dr-editor")
	if err != nil || u.Role != "manage" || u.DisplayName != "DR Editor" {
		t.Fatalf("editor not restored: %+v, %v", u, err)
	}
	webhooks, _ := db.ListWebhooks()
	if len(webhooks) != 1 || webhooks[0].Name != "dr-hook" || webhooks[0].URL != "http://127.0.0.1:9/dr-hook" {
		t.Fatalf("webhook not restored: %+v", webhooks)
	}
	hook := webhooks[0]
	app, err = db.GetAppBySlug("dr-nginx")
	if err != nil {
		t.Fatalf("app not re-adopted: %v", err)
	}
	rules, _ := db.ListAlertRules(&app.ID)
	if len(rules) != 1 || rules[0].Metric != "cpu_pct" || rules[0].Operator != ">" || rules[0].Threshold != 90 ||
		rules[0].DurationSec != 300 || rules[0].WebhookID != hook.ID || !rules[0].Enabled {
		t.Errorf("alert rule not restored: %+v", rules)
	}
	backups, _ := db.ListBackupConfigs(&app.ID)
	if len(backups) != 1 || backups[0].ScheduleCron != "0 3 * * *" || backups[0].TargetConfigJSON != "enc:target" {
		t.Errorf("backup config not restored: %+v", backups)
	}
	if access, _ := db.ListAccessForApp(app.ID); len(access) != 1 || access[0] != "dr-editor" {
		t.Errorf("access grant not restored: %v", access)
	}
}
