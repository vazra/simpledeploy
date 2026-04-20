package gitsync

import (
	"path/filepath"
	"testing"
	"time"

	appcfg "github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestResolveConfig_EmptyDBUsesYAML(t *testing.T) {
	st := openTestStore(t)
	yaml := &appcfg.GitSyncConfig{
		Enabled:      true,
		Remote:       "git@github.com:owner/repo.git",
		Branch:       "main",
		PollInterval: 30 * time.Second,
	}
	cfg, err := ResolveConfig(st, yaml, "/apps", "secret")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Error("expected Enabled=true from YAML")
	}
	if cfg.Remote != yaml.Remote {
		t.Errorf("Remote: got %q, want %q", cfg.Remote, yaml.Remote)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval: got %v, want 30s", cfg.PollInterval)
	}
}

func TestResolveConfig_DBWinsOverYAML(t *testing.T) {
	st := openTestStore(t)
	_ = st.SetGitSyncConfig(map[string]string{
		"enabled":       "true",
		"remote":        "git@github.com:db/repo.git",
		"branch":        "production",
		"poll_interval": "120",
	})

	yaml := &appcfg.GitSyncConfig{
		Enabled: true,
		Remote:  "git@github.com:yaml/repo.git",
		Branch:  "main",
	}
	cfg, err := ResolveConfig(st, yaml, "/apps", "secret")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if cfg.Remote != "git@github.com:db/repo.git" {
		t.Errorf("DB remote not used: got %q", cfg.Remote)
	}
	if cfg.Branch != "production" {
		t.Errorf("DB branch not used: got %q", cfg.Branch)
	}
	if cfg.PollInterval != 120*time.Second {
		t.Errorf("DB poll interval not used: got %v", cfg.PollInterval)
	}
}

func TestResolveConfig_DefaultsApplied(t *testing.T) {
	st := openTestStore(t)
	_ = st.SetGitSyncConfig(map[string]string{
		"enabled": "true",
		"remote":  "file:///tmp/bare.git",
	})

	cfg, err := ResolveConfig(st, nil, "/apps", "secret")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if cfg.Branch != "main" {
		t.Errorf("Branch default: got %q", cfg.Branch)
	}
	if cfg.AuthorName != "SimpleDeploy" {
		t.Errorf("AuthorName default: got %q", cfg.AuthorName)
	}
	if cfg.PollInterval != 60*time.Second {
		t.Errorf("PollInterval default: got %v", cfg.PollInterval)
	}
}

func TestResolveConfig_DisabledWhenNeitherSet(t *testing.T) {
	st := openTestStore(t)
	cfg, err := ResolveConfig(st, nil, "/apps", "secret")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if cfg.Enabled {
		t.Error("expected Enabled=false when no DB and no YAML")
	}
}

// TestResolveConfig_TogglesDefaultTrueWhenMissing: DB keys present but toggle keys absent.
// All four toggles should default to true.
func TestResolveConfig_TogglesDefaultTrueWhenMissing(t *testing.T) {
	st := openTestStore(t)
	_ = st.SetGitSyncConfig(map[string]string{
		"enabled": "true",
		"remote":  "file:///tmp/bare.git",
		// no poll_enabled / auto_push_enabled / auto_apply_enabled / webhook_enabled
	})

	cfg, err := ResolveConfig(st, nil, "/apps", "secret")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if !cfg.PollEnabled {
		t.Error("PollEnabled should default to true when key missing")
	}
	if !cfg.AutoPushEnabled {
		t.Error("AutoPushEnabled should default to true when key missing")
	}
	if !cfg.AutoApplyEnabled {
		t.Error("AutoApplyEnabled should default to true when key missing")
	}
	if !cfg.WebhookEnabled {
		t.Error("WebhookEnabled should default to true when key missing")
	}
}

// TestResolveConfig_TogglesOverriddenFromDB: explicitly set to false in DB.
func TestResolveConfig_TogglesOverriddenFromDB(t *testing.T) {
	st := openTestStore(t)
	_ = st.SetGitSyncConfig(map[string]string{
		"enabled":            "true",
		"remote":             "file:///tmp/bare.git",
		"poll_enabled":       "false",
		"auto_push_enabled":  "false",
		"auto_apply_enabled": "false",
		"webhook_enabled":    "false",
	})

	cfg, err := ResolveConfig(st, nil, "/apps", "secret")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if cfg.PollEnabled {
		t.Error("PollEnabled should be false from DB")
	}
	if cfg.AutoPushEnabled {
		t.Error("AutoPushEnabled should be false from DB")
	}
	if cfg.AutoApplyEnabled {
		t.Error("AutoApplyEnabled should be false from DB")
	}
	if cfg.WebhookEnabled {
		t.Error("WebhookEnabled should be false from DB")
	}
}

// TestResolveConfig_YAMLPathTogglesTrueByDefault: when using YAML (no DB), toggles all true.
func TestResolveConfig_YAMLPathTogglesTrueByDefault(t *testing.T) {
	st := openTestStore(t)
	yaml := &appcfg.GitSyncConfig{
		Enabled: true,
		Remote:  "git@github.com:owner/repo.git",
	}
	cfg, err := ResolveConfig(st, yaml, "/apps", "secret")
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if !cfg.PollEnabled || !cfg.AutoPushEnabled || !cfg.AutoApplyEnabled || !cfg.WebhookEnabled {
		t.Errorf("all toggles should be true via YAML path: poll=%v push=%v apply=%v webhook=%v",
			cfg.PollEnabled, cfg.AutoPushEnabled, cfg.AutoApplyEnabled, cfg.WebhookEnabled)
	}
}
