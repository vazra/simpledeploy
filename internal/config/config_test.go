package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DataDir != "/var/lib/simpledeploy" {
		t.Errorf("DataDir = %q, want /var/lib/simpledeploy", cfg.DataDir)
	}
	if cfg.AppsDir != "/etc/simpledeploy/apps" {
		t.Errorf("AppsDir = %q, want /etc/simpledeploy/apps", cfg.AppsDir)
	}
	if cfg.ListenAddr != ":443" {
		t.Errorf("ListenAddr = %q, want :443", cfg.ListenAddr)
	}
	if cfg.ManagementPort != 8443 {
		t.Errorf("ManagementPort = %d, want 8443", cfg.ManagementPort)
	}
	if cfg.TLS.Mode != "auto" {
		t.Errorf("TLS.Mode = %q, want auto", cfg.TLS.Mode)
	}
	if len(cfg.Metrics.Tiers) != 5 {
		t.Errorf("Metrics.Tiers len = %d, want 5", len(cfg.Metrics.Tiers))
	}
	if cfg.RateLimit.Requests != 200 {
		t.Errorf("RateLimit.Requests = %d, want 200", cfg.RateLimit.Requests)
	}
}

func TestLoadConfig(t *testing.T) {
	yaml := `
data_dir: /tmp/sd-test
apps_dir: /tmp/sd-apps
listen_addr: ":8080"
management_port: 9090
domain: test.example.com
tls:
  mode: "off"
  email: test@example.com
master_secret: "test-secret"
metrics:
  tiers:
    - name: raw
      interval: 10s
      retention: 12h
ratelimit:
  requests: 100
  window: 30s
  burst: 20
  by: ip
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DataDir != "/tmp/sd-test" {
		t.Errorf("DataDir = %q, want /tmp/sd-test", cfg.DataDir)
	}
	if cfg.ManagementPort != 9090 {
		t.Errorf("ManagementPort = %d, want 9090", cfg.ManagementPort)
	}
	if cfg.TLS.Mode != "off" {
		t.Errorf("TLS.Mode = %q, want off", cfg.TLS.Mode)
	}
	if cfg.MasterSecret != "test-secret" {
		t.Errorf("MasterSecret = %q, want test-secret", cfg.MasterSecret)
	}
	if len(cfg.Metrics.Tiers) != 1 {
		t.Errorf("Metrics.Tiers len = %d, want 1", len(cfg.Metrics.Tiers))
	}
	if cfg.RateLimit.Burst != 20 {
		t.Errorf("RateLimit.Burst = %d, want 20", cfg.RateLimit.Burst)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(":::invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidate_ValidModes(t *testing.T) {
	validModes := []string{"", "auto", "custom", "off", "local"}
	for _, mode := range validModes {
		cfg := DefaultConfig()
		cfg.MasterSecret = "test-secret"
		cfg.TLS.Mode = mode
		if err := cfg.Validate(); err != nil {
			t.Errorf("mode %q: expected no error, got %v", mode, err)
		}
	}
}

func TestValidate_InvalidMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MasterSecret = "test-secret"
	cfg.TLS.Mode = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for mode 'invalid', got nil")
	}
}

func TestLoad_ValidLocalMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "tls:\n  mode: local\nmaster_secret: test-secret\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.TLS.Mode != "local" {
		t.Errorf("expected mode 'local', got %q", cfg.TLS.Mode)
	}
}

func TestLoad_InvalidMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "tls:\n  mode: bogus\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid tls mode, got nil")
	}
}

func TestGitSyncConfig_Parses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
master_secret: "s3cr3t"
tls:
  mode: "off"
git_sync:
  enabled: true
  remote: "git@github.com:owner/repo.git"
  branch: "prod"
  author_name: "Bot"
  author_email: "bot@example.com"
  poll_interval: 120s
  webhook_secret: "whsec"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	gs := cfg.GitSync
	if !gs.Enabled {
		t.Error("expected GitSync.Enabled=true")
	}
	if gs.Remote != "git@github.com:owner/repo.git" {
		t.Errorf("Remote=%q", gs.Remote)
	}
	if gs.Branch != "prod" {
		t.Errorf("Branch=%q, want prod", gs.Branch)
	}
	if gs.AuthorName != "Bot" {
		t.Errorf("AuthorName=%q, want Bot", gs.AuthorName)
	}
	if gs.PollInterval != 120*1e9 {
		t.Errorf("PollInterval=%v, want 120s", gs.PollInterval)
	}
	if gs.WebhookSecret != "whsec" {
		t.Errorf("WebhookSecret=%q, want whsec", gs.WebhookSecret)
	}
}

func TestGitSyncConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
master_secret: "s3cr3t"
tls:
  mode: "off"
git_sync:
  enabled: true
  remote: "https://github.com/owner/repo.git"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	gs := cfg.GitSync
	if gs.Branch != "main" {
		t.Errorf("Branch=%q, want main", gs.Branch)
	}
	if gs.AuthorName != "SimpleDeploy" {
		t.Errorf("AuthorName=%q, want SimpleDeploy", gs.AuthorName)
	}
	if gs.AuthorEmail != "bot@simpledeploy.local" {
		t.Errorf("AuthorEmail=%q, want bot@simpledeploy.local", gs.AuthorEmail)
	}
	if gs.PollInterval != 60*1e9 {
		t.Errorf("PollInterval=%v, want 60s", gs.PollInterval)
	}
}

func TestGitSyncConfig_MissingRemote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
master_secret: "s3cr3t"
tls:
  mode: "off"
git_sync:
  enabled: true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for enabled gitsync with empty remote")
	}
}

func TestGitSyncConfig_DisabledNoRemote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
master_secret: "s3cr3t"
tls:
  mode: "off"
git_sync:
  enabled: false
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// disabled + no remote should be fine
	_, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
