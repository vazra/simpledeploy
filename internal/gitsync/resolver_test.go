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
