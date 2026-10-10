package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateSessionKey(t *testing.T) {
	dir := t.TempDir()
	k1, err := loadOrCreateSessionKey(dir)
	if err != nil || len(k1) < 32 {
		t.Fatalf("key = %q, err = %v", k1, err)
	}
	fi, err := os.Stat(filepath.Join(dir, sessionKeyFile))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("stat: %v mode %v", err, fi.Mode())
	}
	k2, err := loadOrCreateSessionKey(dir)
	if err != nil || k2 != k1 {
		t.Fatalf("key not stable: %q vs %q (%v)", k1, k2, err)
	}
}
