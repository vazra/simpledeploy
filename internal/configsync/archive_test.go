package configsync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestTombstoneWriteReadDelete(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed webhook.
	wh := &store.Webhook{Name: "slack-arch", Type: "slack", URL: "https://hooks.example.com/x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Seed app.
	app := &store.App{Name: "Arch App", Slug: "archapp", ComposePath: "/apps/archapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// Seed alert rule.
	rule := &store.AlertRule{AppID: &app.ID, Metric: "cpu", Operator: ">", Threshold: 75, DurationSec: 120, WebhookID: wh.ID, Enabled: true}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// Seed backup config.
	retDays := 14
	bc := &store.BackupConfig{
		AppID: app.ID, Strategy: "postgres", Target: "s3",
		ScheduleCron: "0 3 * * *", TargetConfigJSON: "enc",
		RetentionMode: "time", RetentionDays: &retDays, VerifyUpload: true,
	}
	if err := st.CreateBackupConfig(bc); err != nil {
		t.Fatalf("create backup config: %v", err)
	}

	archivedAt := time.Now().UTC().Truncate(time.Second)
	if err := syncer.WriteTombstone("archapp", archivedAt); err != nil {
		t.Fatalf("WriteTombstone: %v", err)
	}

	// File exists at expected path with mode 0644.
	path := filepath.Join(dataDir, archiveDirName, "archapp.yml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat tombstone: %v", err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Fatalf("perm = %o, want 0644", got)
	}

	// Read back and verify fields.
	tomb, err := syncer.ReadTombstone("archapp")
	if err != nil {
		t.Fatalf("ReadTombstone: %v", err)
	}
	if tomb.Version != Version {
		t.Errorf("Version = %d, want %d", tomb.Version, Version)
	}
	if !tomb.ArchivedAt.Equal(archivedAt) {
		t.Errorf("ArchivedAt = %v, want %v", tomb.ArchivedAt, archivedAt)
	}
	if tomb.App.Slug != "archapp" || tomb.App.DisplayName != "Arch App" {
		t.Errorf("App = %+v", tomb.App)
	}
	if len(tomb.AlertRules) != 1 || tomb.AlertRules[0].Metric != "cpu" || tomb.AlertRules[0].Webhook != "slack-arch" {
		t.Errorf("AlertRules = %+v", tomb.AlertRules)
	}
	if len(tomb.BackupConfigs) != 1 || tomb.BackupConfigs[0].Strategy != "postgres" {
		t.Errorf("BackupConfigs = %+v", tomb.BackupConfigs)
	}

	// Delete and verify gone.
	if err := syncer.DeleteTombstone("archapp"); err != nil {
		t.Fatalf("DeleteTombstone: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("tombstone still exists after delete: err=%v", err)
	}

	// Second delete is a no-op.
	if err := syncer.DeleteTombstone("archapp"); err != nil {
		t.Fatalf("DeleteTombstone (missing) returned err: %v", err)
	}
}
