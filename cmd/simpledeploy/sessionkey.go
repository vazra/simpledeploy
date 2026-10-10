package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/fsutil"
)

// sessionKeyFile holds the random session signing key used when
// master_secret is a public placeholder.
const sessionKeyFile = "session-signing.key"

// loadOrCreateSessionKey returns the session signing key stored in dataDir,
// creating it (0600) on first use.
func loadOrCreateSessionKey(dataDir string) (string, error) {
	path := filepath.Join(dataDir, sessionKeyFile)
	if data, err := fsutil.ReadRegularFile(path); err == nil {
		if key := strings.TrimSpace(string(data)); len(key) >= 32 {
			return key, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	key, err := auth.GenerateRandomSecret(32)
	if err != nil {
		return "", fmt.Errorf("generate: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	if err := fsutil.WriteFileAtomic(path, []byte(key+"\n"), 0o600); err != nil {
		return "", err
	}
	return key, nil
}
