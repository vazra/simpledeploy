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
	if len(cfg.Metrics.Tiers) != 4 {
		t.Errorf("Metrics.Tiers len = %d, want 4", len(cfg.Metrics.Tiers))
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
