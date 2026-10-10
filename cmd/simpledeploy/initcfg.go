package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/config"
)

// writeInitConfig writes a default config with a random master_secret to
// path. An existing file is only replaced when force is set: it holds the
// master_secret that encrypts stored credentials, and overwriting it makes
// them unreadable. --force replaces the whole file with defaults and a new
// master_secret. The file is written via temp file + rename (never partly
// written, a symlink at path is replaced rather than followed) and always
// ends up mode 0600.
func writeInitConfig(path string, force bool) error {
	if _, err := os.Lstat(path); err == nil {
		if !force {
			return fmt.Errorf("config file %s already exists; refusing to overwrite it because its master_secret encrypts stored credentials (re-run with --force to replace it)", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check config file: %w", err)
	}

	secret, err := auth.GenerateRandomSecret(32)
	if err != nil {
		return fmt.Errorf("generate master_secret: %w", err)
	}
	cfg := config.DefaultConfig()
	cfg.MasterSecret = secret

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := cfg.SaveAtomic(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
