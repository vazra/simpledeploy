package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// expandHome resolves a leading "~" or "~/" to the current user's home dir.
// Other paths pass through unchanged. Empty input returns "".
func expandHome(p string) string {
	if p == "" {
		return p
	}
	if p == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

type Config struct {
	DataDir        string          `yaml:"data_dir"`
	AppsDir        string          `yaml:"apps_dir"`
	ListenAddr     string          `yaml:"listen_addr"`
	HTTPListenAddr string          `yaml:"http_listen_addr"`
	ManagementPort int             `yaml:"management_port"`
	// ManagementAddr is the bind address for the dashboard listener.
	// Defaults to "127.0.0.1" so the plain-HTTP dashboard is not exposed
	// to the network without an explicit operator decision (front it with
	// Caddy, or set ManagementAddr: "" to bind all interfaces).
	ManagementAddr string `yaml:"management_addr"`
	Domain         string          `yaml:"domain"`
	TLS            TLSConfig       `yaml:"tls"`
	MasterSecret   string          `yaml:"master_secret"`
	Metrics        MetricsConfig   `yaml:"metrics"`
	RateLimit      RateLimitConfig `yaml:"ratelimit"`
	LoginRateLimit RateLimitConfig `yaml:"login_ratelimit"`
	Registries     []string        `yaml:"registries"`
	TrustedProxies []string        `yaml:"trusted_proxies"`
	LogBufferSize  int             `yaml:"log_buffer_size"`
	PublicHost      string          `yaml:"public_host"`
	RecipesIndexURL string          `yaml:"recipes_index_url"`
	GitSync         GitSyncConfig   `yaml:"git_sync"`
}

// GitSyncConfig controls optional git-backed config sync.
type GitSyncConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Remote        string        `yaml:"remote"`
	Branch        string        `yaml:"branch"`
	AuthorName    string        `yaml:"author_name"`
	AuthorEmail   string        `yaml:"author_email"`
	SSHKeyPath    string        `yaml:"ssh_key_path"`
	HTTPSUsername string        `yaml:"https_username"`
	HTTPSToken    string        `yaml:"https_token"`
	PollInterval  time.Duration `yaml:"poll_interval"`
	WebhookSecret string        `yaml:"webhook_secret"`
}

type TLSConfig struct {
	Mode  string `yaml:"mode"`
	Email string `yaml:"email"`
}

type MetricsTier struct {
	Name      string `yaml:"name"`
	Interval  string `yaml:"interval,omitempty"`
	Retention string `yaml:"retention"`
}

type MetricsConfig struct {
	Tiers []MetricsTier `yaml:"tiers"`
}

type RateLimitConfig struct {
	Requests int    `yaml:"requests"`
	Window   string `yaml:"window"`
	Burst    int    `yaml:"burst"`
	By       string `yaml:"by"`
}

func DefaultConfig() *Config {
	return &Config{
		DataDir:        "/var/lib/simpledeploy",
		AppsDir:        "/etc/simpledeploy/apps",
		ListenAddr:     ":443",
		ManagementPort: 8443,
		ManagementAddr: "127.0.0.1",
		TLS: TLSConfig{
			Mode: "auto",
		},
		Metrics: MetricsConfig{
			Tiers: []MetricsTier{
				{Name: "raw", Interval: "10s", Retention: "90m"},
				{Name: "1m", Retention: "7h"},
				{Name: "5m", Retention: "26h"},
				{Name: "1h", Retention: "31d"},
				{Name: "1d", Retention: "400d"},
			},
		},
		RateLimit: RateLimitConfig{
			Requests: 200,
			Window:   "60s",
			Burst:    50,
			By:       "ip",
		},
		LoginRateLimit: RateLimitConfig{
			Requests: 10,
			Window:   "60s",
		},
		LogBufferSize: 500,
	}
}

func (c *Config) applyGitSyncDefaults() {
	if !c.GitSync.Enabled {
		return
	}
	if c.GitSync.Branch == "" {
		c.GitSync.Branch = "main"
	}
	if c.GitSync.AuthorName == "" {
		c.GitSync.AuthorName = "SimpleDeploy"
	}
	if c.GitSync.AuthorEmail == "" {
		c.GitSync.AuthorEmail = "bot@simpledeploy.local"
	}
	if c.GitSync.PollInterval == 0 {
		c.GitSync.PollInterval = 60 * time.Second
	}
}

func (c *Config) Validate() error {
	switch c.TLS.Mode {
	case "", "auto", "custom", "off", "local":
	default:
		return fmt.Errorf("invalid tls.mode %q: must be one of auto, custom, off, local, or empty", c.TLS.Mode)
	}
	if c.MasterSecret == "" {
		return fmt.Errorf("master_secret is required")
	}
	if c.ManagementPort != 0 && (c.ManagementPort < 1 || c.ManagementPort > 65535) {
		return fmt.Errorf("management_port must be 1-65535")
	}
	if c.GitSync.Enabled && c.GitSync.Remote == "" {
		return fmt.Errorf("gitsync.remote is required when gitsync.enabled is true")
	}
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.DataDir = expandHome(cfg.DataDir)
	cfg.AppsDir = expandHome(cfg.AppsDir)
	cfg.GitSync.SSHKeyPath = expandHome(cfg.GitSync.SSHKeyPath)
	cfg.applyGitSyncDefaults()
	if cfg.RecipesIndexURL == "" {
		cfg.RecipesIndexURL = "https://vazra.github.io/simpledeploy-recipes/index.json"
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

// SaveAtomic writes the config to path atomically (temp file + rename).
func (c *Config) SaveAtomic(path string) error {
	data, err := c.Marshal()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	// 0600: config.yaml contains master_secret which gates all encrypted
	// blobs and JWT/HMAC signing.
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
