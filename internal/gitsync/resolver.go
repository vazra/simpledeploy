package gitsync

import (
	"strconv"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	appcfg "github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/store"
)

// ResolveConfig builds an effective gitsync.Config by merging DB rows (wins)
// over the YAML-parsed GitSyncConfig. Secrets stored as *_enc keys are
// decrypted with masterSecret. Returns a config with Enabled=false when
// neither source has the feature turned on.
func ResolveConfig(st *store.Store, yamlCfg *appcfg.GitSyncConfig, appsDir, masterSecret string) (*Config, error) {
	dbKV, err := st.GetGitSyncConfig()
	if err != nil {
		return nil, err
	}

	// If the DB has no rows, fall back to YAML.
	if len(dbKV) == 0 {
		if yamlCfg == nil || !yamlCfg.Enabled {
			return &Config{Enabled: false}, nil
		}
		return yamlToConfig(yamlCfg, appsDir), nil
	}

	// DB is authoritative.
	cfg := &Config{
		AppsDir: appsDir,
	}

	if v, ok := dbKV["enabled"]; ok {
		cfg.Enabled = v == "true"
	}
	cfg.Remote = dbKV["remote"]
	cfg.Branch = dbKV["branch"]
	cfg.AuthorName = dbKV["author_name"]
	cfg.AuthorEmail = dbKV["author_email"]
	cfg.SSHKeyPath = dbKV["ssh_key_path"]
	cfg.HTTPSUsername = dbKV["https_username"]

	if enc, ok := dbKV["webhook_secret_enc"]; ok && enc != "" {
		plain, decErr := auth.Decrypt(enc, masterSecret)
		if decErr == nil {
			cfg.WebhookSecret = plain
		}
	}
	if enc, ok := dbKV["https_token_enc"]; ok && enc != "" {
		plain, decErr := auth.Decrypt(enc, masterSecret)
		if decErr == nil {
			cfg.HTTPSToken = plain
		}
	}

	if v, ok := dbKV["poll_interval"]; ok && v != "" {
		if secs, parseErr := strconv.Atoi(v); parseErr == nil && secs > 0 {
			cfg.PollInterval = time.Duration(secs) * time.Second
		}
	}

	applyDefaults(cfg)
	return cfg, nil
}

func yamlToConfig(y *appcfg.GitSyncConfig, appsDir string) *Config {
	cfg := &Config{
		Enabled:       y.Enabled,
		Remote:        y.Remote,
		Branch:        y.Branch,
		AppsDir:       appsDir,
		AuthorName:    y.AuthorName,
		AuthorEmail:   y.AuthorEmail,
		SSHKeyPath:    y.SSHKeyPath,
		HTTPSUsername: y.HTTPSUsername,
		HTTPSToken:    y.HTTPSToken,
		PollInterval:  y.PollInterval,
		WebhookSecret: y.WebhookSecret,
	}
	applyDefaults(cfg)
	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.AuthorName == "" {
		cfg.AuthorName = "SimpleDeploy"
	}
	if cfg.AuthorEmail == "" {
		cfg.AuthorEmail = "bot@simpledeploy.local"
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 60 * time.Second
	}
}
