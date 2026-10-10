package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadWithSecret(t *testing.T, secret string) (*Config, string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "tls:\n  mode: \"off\"\nmaster_secret: \"" + secret + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prev)
	cfg, err := Load(path)
	return cfg, logs.String(), err
}

func TestLoadFlagsPlaceholderSecrets(t *testing.T) {
	for _, s := range []string{
		"change-me-to-a-random-string",
		"PASTE_RANDOM_STRING_HERE",
		"PASTE_LONG_RANDOM_STRING_HERE",
		"generate-a-random-string-here",
		"a1b2c3d4e5f6...",
		"changeme",
		"CHANGEME",
		"secret",
		" secret ",
		"<64 random hex characters>",
		"<output of openssl rand -hex 32>",
	} {
		t.Run(s, func(t *testing.T) {
			cfg, logs, err := loadWithSecret(t, s)
			if err != nil {
				t.Fatalf("placeholder must not stop startup: %v", err)
			}
			if !cfg.PlaceholderMasterSecret() {
				t.Fatalf("placeholder %q not flagged", s)
			}
			if !strings.Contains(logs, "openssl rand -hex 32") {
				t.Errorf("warning does not tell the operator how to fix it: %q", logs)
			}
		})
	}
}

func TestLoadWarnsOnShortSecret(t *testing.T) {
	cfg, logs, err := loadWithSecret(t, "short-but-not-a-placeholder")
	if err != nil {
		t.Fatalf("short secret must only warn, got error: %v", err)
	}
	if cfg.MasterSecret != "short-but-not-a-placeholder" {
		t.Fatalf("MasterSecret = %q", cfg.MasterSecret)
	}
	if !strings.Contains(logs, "WARNING: master_secret is shorter than 32") {
		t.Fatalf("expected short-secret warning, logs: %q", logs)
	}
}

func TestLoadStrongSecretNoWarning(t *testing.T) {
	_, logs, err := loadWithSecret(t, strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs, "master_secret") {
		t.Fatalf("unexpected warning: %q", logs)
	}
}
