package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/config"
)

func TestWriteInitConfigGeneratesSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := writeInitConfig(path, false); err != nil {
		t.Fatalf("writeInitConfig: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", fi.Mode().Perm())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}
	if len(cfg.MasterSecret) != 64 {
		t.Fatalf("master_secret len = %d, want 64 hex chars", len(cfg.MasterSecret))
	}
	if _, err := hex.DecodeString(cfg.MasterSecret); err != nil {
		t.Fatalf("master_secret not hex: %v", err)
	}

	// Two runs never share a secret.
	other := filepath.Join(t.TempDir(), "config.yaml")
	if err := writeInitConfig(other, false); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := config.Load(other)
	if cfg2.MasterSecret == cfg.MasterSecret {
		t.Fatal("master_secret repeated across runs")
	}
}

func TestWriteInitConfigRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	orig := []byte("master_secret: keep-me\n")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeInitConfig(path, false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want refusal mentioning --force", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(orig) {
		t.Fatal("existing config was modified")
	}
}

func TestWriteInitConfigForceTightensMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("old: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeInitConfig(path, true); err != nil {
		t.Fatalf("writeInitConfig --force: %v", err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600 after --force", fi.Mode().Perm())
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("forced config does not load: %v", err)
	}
}

// TestWriteInitConfigForceReplacesViaRename: --force swaps in a fresh file
// (temp file + rename) instead of truncating in place, so a symlink at the
// path is replaced, not followed, and no temp files are left behind.
func TestWriteInitConfigForceReplacesViaRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere.yaml")
	orig := []byte("master_secret: keep-me\nextra_key: 1\n")
	if err := os.WriteFile(target, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := writeInitConfig(path, true); err != nil {
		t.Fatalf("writeInitConfig --force: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != string(orig) {
		t.Fatalf("symlink target was written through: %q", got)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want regular 0600", fi.Mode())
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "extra_key") || strings.Contains(string(data), "keep-me") {
		t.Fatalf("old content survived --force:\n%s", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftover files in config dir: %v", entries)
	}
}
