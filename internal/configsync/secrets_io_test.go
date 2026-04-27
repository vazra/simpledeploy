package configsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestWriteAppSecrets_Mode0600(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	if err := os.MkdirAll(filepath.Join(appsDir, "x"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sec := &AppSecrets{Version: Version, Slug: "x", BackupConfigs: []BackupSecretsEntry{{ID: "u1", TargetConfigEnc: "blob"}}}
	if err := syncer.WriteAppSecrets("x", sec); err != nil {
		t.Fatalf("WriteAppSecrets: %v", err)
	}
	info, err := os.Stat(filepath.Join(appsDir, "x", appSecretsName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("mode = %o, want 0600", mode)
	}
}

func TestWriteGlobalSecrets_Mode0600(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	g := &GlobalSecrets{Version: Version, Users: []UserSecretsEntry{{Username: "u", PasswordHash: "h"}}}
	if err := syncer.WriteGlobalSecrets(g); err != nil {
		t.Fatalf("WriteGlobalSecrets: %v", err)
	}
	info, err := os.Stat(filepath.Join(dataDir, globalSecretsName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("mode = %o, want 0600", mode)
	}
}

func TestReadWriteRoundTripAppSecrets(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	if err := os.MkdirAll(filepath.Join(appsDir, "x"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := &AppSecrets{
		Version: Version,
		Slug:    "x",
		BackupConfigs: []BackupSecretsEntry{
			{ID: "uuid-1", TargetConfigEnc: "blob1"},
			{ID: "uuid-2", TargetConfigEnc: "\x00\x01\xff binary"},
		},
	}
	if err := syncer.WriteAppSecrets("x", want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := syncer.ReadAppSecrets("x")
	if err != nil || got == nil {
		t.Fatalf("read: %v / nil=%v", err, got == nil)
	}
	if got.Slug != "x" || len(got.BackupConfigs) != 2 || got.BackupConfigs[1].TargetConfigEnc != want.BackupConfigs[1].TargetConfigEnc {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestReadWriteRoundTripGlobalSecrets(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	want := &GlobalSecrets{
		Version: Version,
		Users:   []UserSecretsEntry{{Username: "alice", PasswordHash: "$2a$10$h"}},
		APIKeys: []APIKeySecretsEntry{{KeyHash: "kh", Username: "alice", Name: "ci"}},
		Registries: []RegistrySecretsEntry{
			{ID: "r1", UsernameEnc: "u-enc", PasswordEnc: "p-enc"},
		},
		Webhooks: []WebhookSecretsEntry{{Name: "wh", URL: "https://x"}},
		DBBackup: &DBBackupSecretsEntry{TargetConfigEnc: "tce"},
	}
	if err := syncer.WriteGlobalSecrets(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := syncer.ReadGlobalSecrets()
	if err != nil || got == nil {
		t.Fatalf("read: %v / nil=%v", err, got == nil)
	}
	if len(got.Users) != 1 || got.Users[0].PasswordHash != "$2a$10$h" {
		t.Errorf("users mismatch: %+v", got.Users)
	}
	if got.DBBackup == nil || got.DBBackup.TargetConfigEnc != "tce" {
		t.Errorf("db backup mismatch: %+v", got.DBBackup)
	}
	if len(got.Registries) != 1 || got.Registries[0].UsernameEnc != "u-enc" {
		t.Errorf("registries mismatch: %+v", got.Registries)
	}
}

// TestWriteAppSidecar_SplitsSecrets asserts the non-secret sidecar does NOT
// contain encrypted data and the secrets sidecar does.
func TestWriteAppSidecar_SplitsSecrets(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "Split", Slug: "split", ComposePath: "/apps/split/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	const ciphertext = "VERY_SECRET_CIPHERTEXT_BLOB"
	bc := &store.BackupConfig{
		AppID: app.ID, Strategy: "postgres", Target: "s3",
		ScheduleCron: "0 2 * * *", TargetConfigJSON: ciphertext,
		RetentionMode: "count", RetentionCount: 3,
	}
	if err := st.CreateBackupConfig(bc); err != nil {
		t.Fatalf("create backup: %v", err)
	}

	if err := syncer.WriteAppSidecar("split"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}

	sidecarPath := filepath.Join(appsDir, "split", appSidecarName)
	secretsPath := filepath.Join(appsDir, "split", appSecretsName)

	sidecarRaw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if strings.Contains(string(sidecarRaw), ciphertext) {
		t.Errorf("simpledeploy.yml MUST NOT contain ciphertext, got:\n%s", sidecarRaw)
	}
	secretsRaw, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("read secrets: %v", err)
	}
	if !strings.Contains(string(secretsRaw), ciphertext) {
		t.Errorf("simpledeploy.secrets.yml MUST contain ciphertext, got:\n%s", secretsRaw)
	}
	// And the backup config UUID must appear in both, correlating them.
	if bc.UUID == "" {
		t.Fatal("backup config UUID empty")
	}
	if !strings.Contains(string(sidecarRaw), bc.UUID) {
		t.Errorf("simpledeploy.yml missing UUID %s", bc.UUID)
	}
	if !strings.Contains(string(secretsRaw), bc.UUID) {
		t.Errorf("simpledeploy.secrets.yml missing UUID %s", bc.UUID)
	}
}
