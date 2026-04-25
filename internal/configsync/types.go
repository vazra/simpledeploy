package configsync

import "time"

// Version is the sidecar schema version written to every file.
const Version = 1

// AppSidecar is the per-app YAML sidecar written to {apps_dir}/{slug}/simpledeploy.yml.
type AppSidecar struct {
	Version       int                 `yaml:"version"`
	App           AppMeta             `yaml:"app"`
	AlertRules    []AlertRuleEntry    `yaml:"alert_rules,omitempty"`
	BackupConfigs []BackupConfigEntry `yaml:"backup_configs,omitempty"`
	Access        []AccessEntry       `yaml:"access,omitempty"`
}

// AppMeta holds identifying info about the app.
type AppMeta struct {
	Slug        string `yaml:"slug"`
	DisplayName string `yaml:"display_name"`
}

// AlertRuleEntry is a portable alert rule (webhook referenced by name, not ID).
type AlertRuleEntry struct {
	Metric      string  `yaml:"metric"`
	Operator    string  `yaml:"operator"`
	Threshold   float64 `yaml:"threshold"`
	DurationSec int     `yaml:"duration_sec"`
	Webhook     string  `yaml:"webhook"` // Webhook.Name
	Enabled     bool    `yaml:"enabled"`
}

// BackupConfigEntry mirrors store.BackupConfig without DB IDs.
type BackupConfigEntry struct {
	Strategy        string `yaml:"strategy"`
	Target          string `yaml:"target"`
	ScheduleCron    string `yaml:"schedule_cron"`
	TargetConfigEnc string `yaml:"target_config_enc"` // stored as-is (encrypted blob)
	RetentionMode   string `yaml:"retention_mode"`
	RetentionCount  int    `yaml:"retention_count"`
	RetentionDays   *int   `yaml:"retention_days"`
	VerifyUpload    bool   `yaml:"verify_upload"`
	PreHooks        string `yaml:"pre_hooks,omitempty"`
	PostHooks       string `yaml:"post_hooks,omitempty"`
	Paths           string `yaml:"paths,omitempty"`
}

// AccessEntry records a user who has explicit access to this app.
type AccessEntry struct {
	Username string `yaml:"username"`
}

// GlobalSidecar is the global YAML sidecar written to {data_dir}/config.yml.
type GlobalSidecar struct {
	Version        int               `yaml:"version"`
	Users          []UserEntry       `yaml:"users,omitempty"`
	APIKeys        []APIKeyEntry     `yaml:"api_keys,omitempty"`
	Registries     []RegistryEntry   `yaml:"registries,omitempty"`
	Webhooks       []WebhookEntry    `yaml:"webhooks,omitempty"`
	DBBackupConfig map[string]string `yaml:"db_backup_config,omitempty"`
}

// UserEntry mirrors store.User with password hash.
type UserEntry struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
	Role         string `yaml:"role"`
	DisplayName  string `yaml:"display_name,omitempty"`
	Email        string `yaml:"email,omitempty"`
}

// APIKeyEntry mirrors store.APIKeyRecord (key hash preserved, no plaintext).
type APIKeyEntry struct {
	Username  string     `yaml:"username"`
	KeyHash   string     `yaml:"key_hash"`
	Name      string     `yaml:"name"`
	ExpiresAt *time.Time `yaml:"expires_at,omitempty"`
}

// RegistryEntry mirrors store.Registry with encrypted credential blobs.
type RegistryEntry struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	UsernameEnc string `yaml:"username_enc"`
	PasswordEnc string `yaml:"password_enc"`
}

// WebhookEntry mirrors store.Webhook (URL stored plaintext as in DB).
type WebhookEntry struct {
	Name             string `yaml:"name"`
	Type             string `yaml:"type"`
	URL              string `yaml:"url"`
	TemplateOverride string `yaml:"template_override,omitempty"`
	HeadersJSON      string `yaml:"headers_json,omitempty"`
}

// RedactedGlobalSidecar is a git-safe view of global config.
// Stored at {apps_dir}/_global.yml. Contains NO secrets — no password
// hashes, no api-key hashes, no encrypted credentials, no webhook URLs.
// Used by gitsync for portable config sharing; never used for DR.
type RedactedGlobalSidecar struct {
	Version          int                `yaml:"version"`
	Users            []RedactedUser     `yaml:"users,omitempty"`
	Registries       []RedactedRegistry `yaml:"registries,omitempty"`
	Webhooks         []RedactedWebhook  `yaml:"webhooks,omitempty"`
	DBBackupSchedule string             `yaml:"db_backup_schedule,omitempty"`
	DBBackupTarget   string             `yaml:"db_backup_target,omitempty"`
}

// RedactedUser holds non-secret user fields.
type RedactedUser struct {
	Username    string `yaml:"username"`
	Role        string `yaml:"role"`
	DisplayName string `yaml:"display_name,omitempty"`
	Email       string `yaml:"email,omitempty"`
}

// RedactedRegistry holds non-secret registry fields (URL is the registry hostname, not a secret).
type RedactedRegistry struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// RedactedWebhook holds non-secret webhook fields (no URL, no headers, no template).
type RedactedWebhook struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}
