package configsync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppFromFS_RoundTrip(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	if err := os.MkdirAll(filepath.Join(appsDir, "x"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	wantSidecar := &AppSidecar{
		Version: Version,
		App:     AppMeta{Slug: "x", DisplayName: "X App"},
		AlertRules: []AlertRuleEntry{
			{Metric: "cpu", Operator: ">", Threshold: 0.9, DurationSec: 60, Webhook: "wh", Enabled: true},
		},
		BackupConfigs: []BackupConfigEntry{
			{ID: "uuid-1", Strategy: "postgres", Target: "s3", ScheduleCron: "0 * * * *", RetentionMode: "count", RetentionCount: 7, VerifyUpload: true},
		},
		Access: []AccessEntry{{Username: "alice"}},
	}
	if err := atomicWriteYAML(filepath.Join(appsDir, "x", appSidecarName), wantSidecar); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	wantSecrets := &AppSecrets{
		Version:       Version,
		Slug:          "x",
		BackupConfigs: []BackupSecretsEntry{{ID: "uuid-1", TargetConfigEnc: "blob"}},
	}
	if err := syncer.WriteAppSecrets("x", wantSecrets); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	got, err := syncer.LoadAppFromFS("x")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Slug != "x" {
		t.Errorf("slug = %q", got.Slug)
	}
	if got.Sidecar == nil {
		t.Fatal("sidecar nil")
	}
	if got.Sidecar.App.DisplayName != "X App" {
		t.Errorf("display name = %q", got.Sidecar.App.DisplayName)
	}
	if len(got.Sidecar.AlertRules) != 1 || got.Sidecar.AlertRules[0].Metric != "cpu" {
		t.Errorf("alert rules = %+v", got.Sidecar.AlertRules)
	}
	if len(got.Sidecar.BackupConfigs) != 1 || got.Sidecar.BackupConfigs[0].ID != "uuid-1" {
		t.Errorf("backup configs = %+v", got.Sidecar.BackupConfigs)
	}
	if len(got.Sidecar.Access) != 1 || got.Sidecar.Access[0].Username != "alice" {
		t.Errorf("access = %+v", got.Sidecar.Access)
	}
	if got.Secrets == nil || len(got.Secrets.BackupConfigs) != 1 || got.Secrets.BackupConfigs[0].TargetConfigEnc != "blob" {
		t.Errorf("secrets = %+v", got.Secrets)
	}
}

func TestLoadAppFromFS_Missing(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	got, err := syncer.LoadAppFromFS("ghost")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("nil result")
	}
	if got.Slug != "ghost" {
		t.Errorf("slug = %q", got.Slug)
	}
	if got.Sidecar != nil {
		t.Errorf("sidecar = %+v, want nil", got.Sidecar)
	}
	if got.Secrets != nil {
		t.Errorf("secrets = %+v, want nil", got.Secrets)
	}
}

func TestLoadGlobalFromFS_RoundTrip(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	wantSidecar := &GlobalSidecar{
		Version: Version,
		Users: []UserEntry{
			{Username: "alice", Role: "manage", DisplayName: "Alice", Email: "a@x"},
		},
		APIKeys:    []APIKeyEntry{{Username: "alice", Name: "ci"}},
		Registries: []RegistryEntry{},
		Webhooks:   []WebhookEntry{},
	}
	if err := atomicWriteYAML(filepath.Join(dataDir, globalSidecar), wantSidecar); err != nil {
		t.Fatalf("write global: %v", err)
	}

	wantSecrets := &GlobalSecrets{
		Version: Version,
		Users:   []UserSecretsEntry{{Username: "alice", PasswordHash: "$2a$10$h"}},
		APIKeys: []APIKeySecretsEntry{{KeyHash: "kh", Username: "alice", Name: "ci"}},
	}
	if err := syncer.WriteGlobalSecrets(wantSecrets); err != nil {
		t.Fatalf("write global secrets: %v", err)
	}

	got, err := syncer.LoadGlobalFromFS()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Sidecar == nil {
		t.Fatal("sidecar nil")
	}
	if len(got.Sidecar.Users) != 1 || got.Sidecar.Users[0].Username != "alice" {
		t.Errorf("users = %+v", got.Sidecar.Users)
	}
	if len(got.Sidecar.APIKeys) != 1 || got.Sidecar.APIKeys[0].Name != "ci" {
		t.Errorf("api keys = %+v", got.Sidecar.APIKeys)
	}
	if got.Secrets == nil || len(got.Secrets.Users) != 1 || got.Secrets.Users[0].PasswordHash != "$2a$10$h" {
		t.Errorf("secrets = %+v", got.Secrets)
	}
}

func TestLoadGlobalFromFS_Missing(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	got, err := syncer.LoadGlobalFromFS()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("nil result")
	}
	if got.Sidecar != nil {
		t.Errorf("sidecar = %+v, want nil", got.Sidecar)
	}
	if got.Secrets != nil {
		t.Errorf("secrets = %+v, want nil", got.Secrets)
	}
}
