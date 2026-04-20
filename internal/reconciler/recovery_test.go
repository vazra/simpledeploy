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

// TestReconcilerRemovesSidecarOnComposeDelete verifies that when a compose file
// is removed from disk, Reconcile removes the app from the DB and also deletes
// the per-app sidecar (simpledeploy.yml).
func TestReconcilerRemovesSidecarOnComposeDelete(t *testing.T) {
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

	// Second reconcile: app should be removed from DB and sidecar deleted.
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	if _, err := st.GetAppBySlug("webapp"); err == nil {
		t.Fatal("expected app to be gone from DB after compose deletion, but it still exists")
	}
	if _, err := os.Stat(sidecarPath); err == nil {
		t.Fatal("expected sidecar to be deleted after compose deletion, but it still exists")
	}
}
