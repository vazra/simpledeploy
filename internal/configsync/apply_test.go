package configsync

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestApplyAppSidecar_FullReplace(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed webhook + app + pre-existing rules/backups/access (to be replaced).
	wh := &store.Webhook{Name: "slack", Type: "slack", URL: "https://x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	app := &store.App{Name: "Old Name", Slug: "myapp", ComposePath: "/x/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	stale := &store.AlertRule{AppID: &app.ID, Metric: "stale", Operator: ">", Threshold: 1, DurationSec: 60, WebhookID: wh.ID, Enabled: true}
	if err := st.CreateAlertRule(stale); err != nil {
		t.Fatalf("create stale rule: %v", err)
	}
	staleBC := &store.BackupConfig{AppID: app.ID, Strategy: "volume", Target: "local", ScheduleCron: "0 1 * * *", RetentionMode: "count", RetentionCount: 5}
	if err := st.CreateBackupConfig(staleBC); err != nil {
		t.Fatalf("create stale backup: %v", err)
	}

	// Build LoadedApp matching desired FS state.
	loaded := &LoadedApp{
		Slug: "myapp",
		Sidecar: &AppSidecar{
			Version: Version,
			App:     AppMeta{Slug: "myapp", DisplayName: "New Name"},
			AlertRules: []AlertRuleEntry{
				{Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 300, Webhook: "slack", Enabled: true},
			},
			BackupConfigs: []BackupConfigEntry{
				{ID: "uuid-1", Strategy: "postgres", Target: "s3", ScheduleCron: "0 2 * * *", RetentionMode: "count", RetentionCount: 7, VerifyUpload: true},
			},
		},
		Secrets: &AppSecrets{
			Version: Version,
			Slug:    "myapp",
			BackupConfigs: []BackupSecretsEntry{
				{ID: "uuid-1", TargetConfigEnc: "enc-blob"},
			},
		},
	}

	if err := syncer.ApplyAppSidecar("myapp", loaded); err != nil {
		t.Fatalf("ApplyAppSidecar: %v", err)
	}

	// Display name updated.
	got, err := st.GetAppBySlug("myapp")
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	if got.Name != "New Name" {
		t.Errorf("display name = %q, want New Name", got.Name)
	}

	// Alert rules: only 1 (stale dropped).
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 || rules[0].Metric != "cpu" {
		t.Fatalf("want 1 cpu rule, got %+v", rules)
	}

	// Backup configs: 1 with secrets joined.
	bcs, err := st.ListBackupConfigs(&app.ID)
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(bcs) != 1 || bcs[0].UUID != "uuid-1" || bcs[0].TargetConfigJSON != "enc-blob" {
		t.Fatalf("backup configs mismatch: %+v", bcs)
	}
}

func TestApplyAppSidecar_UnknownWebhookSkipped(t *testing.T) {
	st := openTestStore(t)
	syncer := New(st, t.TempDir(), t.TempDir())

	app := &store.App{Name: "a", Slug: "a", ComposePath: "/x/y.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	loaded := &LoadedApp{
		Slug: "a",
		Sidecar: &AppSidecar{
			Version: Version,
			App:     AppMeta{Slug: "a", DisplayName: "a"},
			AlertRules: []AlertRuleEntry{
				{Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 60, Webhook: "ghost", Enabled: true},
			},
		},
	}
	if err := syncer.ApplyAppSidecar("a", loaded); err != nil {
		t.Fatalf("ApplyAppSidecar: %v", err)
	}
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected rule with unknown webhook to be dropped, got %d rules", len(rules))
	}
}

func TestApplyAppSidecar_BackupWithoutSecretsEntry(t *testing.T) {
	st := openTestStore(t)
	syncer := New(st, t.TempDir(), t.TempDir())

	app := &store.App{Name: "a", Slug: "a", ComposePath: "/x/y.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	loaded := &LoadedApp{
		Slug: "a",
		Sidecar: &AppSidecar{
			Version: Version,
			App:     AppMeta{Slug: "a", DisplayName: "a"},
			BackupConfigs: []BackupConfigEntry{
				{ID: "no-secret-uuid", Strategy: "volume", Target: "local", ScheduleCron: "0 1 * * *", RetentionMode: "count", RetentionCount: 3},
			},
		},
		Secrets: nil, // missing entirely
	}
	if err := syncer.ApplyAppSidecar("a", loaded); err != nil {
		t.Fatalf("ApplyAppSidecar: %v", err)
	}
	bcs, err := st.ListBackupConfigs(&app.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(bcs) != 1 || bcs[0].TargetConfigJSON != "" {
		t.Fatalf("want 1 backup w/ empty enc, got %+v", bcs)
	}
}

func TestApplyGlobalSidecar_NilSecretsPreservesPasswordHash(t *testing.T) {
	st := openTestStore(t)
	syncer := New(st, t.TempDir(), t.TempDir())

	// Pre-existing user with a password hash.
	if _, err := st.CreateUser("alice", "$2a$10$existinghash", "admin", "Alice", "a@x"); err != nil {
		t.Fatalf("create user: %v", err)
	}

	loaded := &LoadedGlobal{
		Sidecar: &GlobalSidecar{
			Version: Version,
			Users: []UserEntry{
				{Username: "alice", Role: "admin", DisplayName: "Alice Updated", Email: "a@x"},
			},
		},
		Secrets: nil,
	}
	if err := syncer.ApplyGlobalSidecar(loaded); err != nil {
		t.Fatalf("ApplyGlobalSidecar: %v", err)
	}
	users, err := st.ListUsersWithHashes()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("want 1 user, got %d", len(users))
	}
	if users[0].PasswordHash != "$2a$10$existinghash" {
		t.Errorf("password hash wiped: %q", users[0].PasswordHash)
	}
	if users[0].DisplayName != "Alice Updated" {
		t.Errorf("display name not applied: %q", users[0].DisplayName)
	}
}

func TestApplyGlobalSidecar_FullReplaceDeletesMissing(t *testing.T) {
	st := openTestStore(t)
	syncer := New(st, t.TempDir(), t.TempDir())

	// Pre-existing webhook + user.
	wh := &store.Webhook{Name: "stale-hook", Type: "slack", URL: "https://x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if _, err := st.CreateUser("stale-user", "$2a$10$h", "admin", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := st.CreateUser("alice", "$2a$10$h", "admin", "", ""); err != nil {
		t.Fatalf("create alice: %v", err)
	}

	loaded := &LoadedGlobal{
		Sidecar: &GlobalSidecar{
			Version: Version,
			Users:   []UserEntry{{Username: "alice", Role: "admin"}},
		},
		Secrets: &GlobalSecrets{Version: Version, Users: []UserSecretsEntry{{Username: "alice", PasswordHash: "$2a$10$h"}}},
	}
	if err := syncer.ApplyGlobalSidecar(loaded); err != nil {
		t.Fatalf("ApplyGlobalSidecar: %v", err)
	}

	users, err := st.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "alice" {
		t.Errorf("stale user not deleted: %+v", users)
	}
	whs, err := st.ListWebhooks()
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	if len(whs) != 0 {
		t.Errorf("stale webhook not deleted: %+v", whs)
	}
}
