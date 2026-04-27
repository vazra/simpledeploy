package configsync

import "time"

// AppSecrets is {apps_dir}/<slug>/simpledeploy.secrets.yml. Mode 0600.
type AppSecrets struct {
	Version       int                  `yaml:"version"`
	Slug          string               `yaml:"slug"`
	BackupConfigs []BackupSecretsEntry `yaml:"backup_configs,omitempty"`
}

type BackupSecretsEntry struct {
	ID              string `yaml:"id"`
	TargetConfigEnc string `yaml:"target_config_enc"`
}

// GlobalSecrets is {data_dir}/secrets.yml. Mode 0600.
type GlobalSecrets struct {
	Version    int                    `yaml:"version"`
	Users      []UserSecretsEntry     `yaml:"users,omitempty"`
	APIKeys    []APIKeySecretsEntry   `yaml:"api_keys,omitempty"`
	Registries []RegistrySecretsEntry `yaml:"registries,omitempty"`
	Webhooks   []WebhookSecretsEntry  `yaml:"webhooks,omitempty"`
	DBBackup   *DBBackupSecretsEntry  `yaml:"db_backup,omitempty"`
}

type UserSecretsEntry struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

type APIKeySecretsEntry struct {
	KeyHash   string     `yaml:"key_hash"`
	Username  string     `yaml:"username"`
	Name      string     `yaml:"name"`
	ExpiresAt *time.Time `yaml:"expires_at,omitempty"`
}

type RegistrySecretsEntry struct {
	ID          string `yaml:"id"`
	UsernameEnc string `yaml:"username_enc"`
	PasswordEnc string `yaml:"password_enc"`
}

type WebhookSecretsEntry struct {
	Name             string `yaml:"name"`
	URL              string `yaml:"url"`
	HeadersJSON      string `yaml:"headers_json,omitempty"`
	TemplateOverride string `yaml:"template_override,omitempty"`
}

type DBBackupSecretsEntry struct {
	TargetConfigEnc string `yaml:"target_config_enc"`
}
