package store

import (
	"path/filepath"
	"testing"
)

func TestGitSyncConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Empty to start.
	kv, err := s.GetGitSyncConfig()
	if err != nil {
		t.Fatalf("GetGitSyncConfig empty: %v", err)
	}
	if len(kv) != 0 {
		t.Errorf("expected empty map, got %v", kv)
	}

	// Set several keys atomically.
	input := map[string]string{
		"enabled": "true",
		"remote":  "git@github.com:owner/repo.git",
		"branch":  "main",
	}
	if err := s.SetGitSyncConfig(input); err != nil {
		t.Fatalf("SetGitSyncConfig: %v", err)
	}

	kv, err = s.GetGitSyncConfig()
	if err != nil {
		t.Fatalf("GetGitSyncConfig after set: %v", err)
	}
	for k, want := range input {
		if got := kv[k]; got != want {
			t.Errorf("key %q: got %q, want %q", k, got, want)
		}
	}

	// Upsert changes one key.
	if err := s.SetGitSyncConfig(map[string]string{"branch": "prod"}); err != nil {
		t.Fatalf("SetGitSyncConfig upsert: %v", err)
	}
	kv, _ = s.GetGitSyncConfig()
	if kv["branch"] != "prod" {
		t.Errorf("branch after upsert: got %q, want %q", kv["branch"], "prod")
	}
	// Original keys still present.
	if kv["remote"] != "git@github.com:owner/repo.git" {
		t.Errorf("remote unexpectedly changed: %q", kv["remote"])
	}

	// Delete clears everything.
	if err := s.DeleteGitSyncConfig(); err != nil {
		t.Fatalf("DeleteGitSyncConfig: %v", err)
	}
	kv, _ = s.GetGitSyncConfig()
	if len(kv) != 0 {
		t.Errorf("expected empty map after delete, got %v", kv)
	}
}
