package client

import (
	"os"
	"testing"
)

func withTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := configDir
	configDir = dir
	t.Cleanup(func() { configDir = old })
}

func TestAddAndGetContext(t *testing.T) {
	withTempConfigDir(t)

	cfg := &ClientConfig{Contexts: make(map[string]Context)}
	cfg.AddContext("prod", "https://example.com", "key123")

	ctx, err := cfg.Contexts["prod"], error(nil)
	_ = err
	if ctx.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", ctx.URL)
	}
	if ctx.APIKey != "key123" {
		t.Errorf("APIKey = %q, want key123", ctx.APIKey)
	}
}

func TestUseContext(t *testing.T) {
	withTempConfigDir(t)

	cfg := &ClientConfig{Contexts: make(map[string]Context)}
	cfg.AddContext("prod", "https://example.com", "key123")

	if err := cfg.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}
	if cfg.CurrentContext != "prod" {
		t.Errorf("CurrentContext = %q, want prod", cfg.CurrentContext)
	}

	got, err := cfg.GetCurrentContext()
	if err != nil {
		t.Fatalf("GetCurrentContext: %v", err)
	}
	if got.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", got.URL)
	}
}

func TestUseContextNotFound(t *testing.T) {
	withTempConfigDir(t)

	cfg := &ClientConfig{Contexts: make(map[string]Context)}
	err := cfg.UseContext("missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetCurrentContextNoContext(t *testing.T) {
	cfg := &ClientConfig{Contexts: make(map[string]Context)}
	_, err := cfg.GetCurrentContext()
	if err == nil {
		t.Fatal("expected error when no context set")
	}
}

func TestLoadSaveConfig(t *testing.T) {
	withTempConfigDir(t)

	cfg := &ClientConfig{Contexts: make(map[string]Context)}
	cfg.AddContext("staging", "https://staging.example.com", "stg-key")
	cfg.UseContext("staging")

	if err := SaveClientConfig(cfg); err != nil {
		t.Fatalf("SaveClientConfig: %v", err)
	}

	loaded, err := LoadClientConfig()
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if loaded.CurrentContext != "staging" {
		t.Errorf("CurrentContext = %q, want staging", loaded.CurrentContext)
	}
	ctx, ok := loaded.Contexts["staging"]
	if !ok {
		t.Fatal("context 'staging' not found after load")
	}
	if ctx.URL != "https://staging.example.com" {
		t.Errorf("URL = %q, want https://staging.example.com", ctx.URL)
	}
	if ctx.APIKey != "stg-key" {
		t.Errorf("APIKey = %q, want stg-key", ctx.APIKey)
	}
}

func TestLoadMissing(t *testing.T) {
	withTempConfigDir(t)

	cfg, err := LoadClientConfig()
	if err != nil {
		t.Fatalf("LoadClientConfig with missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Contexts == nil {
		t.Error("Contexts should be initialized, not nil")
	}
}

func TestSavedFilePermissions(t *testing.T) {
	withTempConfigDir(t)

	cfg := &ClientConfig{Contexts: make(map[string]Context)}
	cfg.AddContext("prod", "https://example.com", "secret")
	if err := SaveClientConfig(cfg); err != nil {
		t.Fatalf("SaveClientConfig: %v", err)
	}

	info, err := os.Stat(ClientConfigPath())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("file permissions = %o, want 0600", mode)
	}
}
