package config

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
	// ExtraListenAddrs are additional addresses for the HTTPS proxy server
	// (e.g. [":50051"]). They serve the same routes, TLS policies and certs
	// as ListenAddr. Empty by default.
	ExtraListenAddrs []string `yaml:"extra_listen_addrs"`
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
	// AllowedBindPaths are host folders apps may bind-mount even though they
	// sit under a protected system folder (e.g. /home/media). SimpleDeploy's
	// data folder and other apps' folders stay protected.
	AllowedBindPaths []string `yaml:"allowed_bind_paths"`
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

// minMasterSecretLen is the length below which Load warns about a weak
// master_secret. `openssl rand -hex 32` produces 64 characters.
const minMasterSecretLen = 32

// placeholderSecrets are example master_secret values from the docs and
// common defaults. Running with one of them means every install shares the
// key that encrypts stored credentials and signs sessions.
var placeholderSecrets = map[string]bool{
	"change-me-to-a-random-string":  true,
	"paste_random_string_here":      true,
	"paste_long_random_string_here": true,
	"generate-a-random-string-here": true,
	"a1b2c3d4e5f6...":               true,
	"...":                           true,
	"changeme":                      true,
	"change-me":                     true,
	"change_me":                     true,
	"secret":                        true,
	"password":                      true,
	"master_secret":                 true,
	"your-secret-here":              true,
}

// isPlaceholderSecret reports whether s is a known example value
// (case-insensitive, surrounding whitespace ignored).
func isPlaceholderSecret(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	// Docs write placeholders as <...>; a real secret never looks like that.
	if strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">") {
		return true
	}
	return placeholderSecrets[v]
}

// PlaceholderMasterSecret reports whether master_secret is a known example
// value from the docs (public, so it must not sign sessions).
func (c *Config) PlaceholderMasterSecret() bool {
	return isPlaceholderSecret(c.MasterSecret)
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
	if err := c.validateExtraListenAddrs(); err != nil {
		return err
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
	if cfg.PlaceholderMasterSecret() {
		// Refusing to start would lock existing installs out (and changing
		// the secret makes stored credentials unreadable). The server instead
		// signs sessions with a random key from data_dir, so the public value
		// never signs sessions.
		log.Printf("WARNING: master_secret is still the example placeholder %q from the docs. Sessions are signed with a random key from data_dir instead, but stored credentials remain protected only by this public value. Generate a real one with `openssl rand -hex 32`, put it in the config, then re-enter registry/S3 credentials and re-create API keys.", cfg.MasterSecret)
	} else if len(cfg.MasterSecret) < minMasterSecretLen {
		log.Printf("WARNING: master_secret is shorter than %d characters; generate a stronger one with `openssl rand -hex 32` (changing it requires re-entering stored registry and backup credentials)", minMasterSecretLen)
	}
	return cfg, nil
}

func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

// SaveAtomic writes the config to path atomically (temp file + rename).
// The temp file is removed when any step fails.
func (c *Config) SaveAtomic(path string) (err error) {
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
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 0600: config.yaml contains master_secret which gates all encrypted
	// blobs and JWT/HMAC signing.
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// effectiveHTTPListenAddr mirrors the serve command's defaulting of
// http_listen_addr: ":80" when TLS is auto/local and the field is empty,
// "" for the opt-out sentinels.
func (c *Config) effectiveHTTPListenAddr() string {
	a := c.HTTPListenAddr
	if a == "" && (c.TLS.Mode == "auto" || c.TLS.Mode == "local") {
		a = ":80"
	}
	switch a {
	case "off", "disabled", "none":
		return ""
	}
	return a
}

// listenAddr is a parsed host:port with the host normalized: "", "0.0.0.0"
// and "::" all mean every interface and become "".
type listenAddr struct {
	raw  string
	host string
	port int
}

func parseListenAddr(a string) (listenAddr, error) {
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return listenAddr{}, err
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return listenAddr{}, fmt.Errorf("port must be 1-65535")
	}
	switch host {
	case "0.0.0.0", "::":
		host = ""
	}
	return listenAddr{raw: a, host: strings.ToLower(host), port: n}, nil
}

// overlaps reports whether two listeners would fight over the same socket:
// same port and the same host, or either one binds every interface.
func (l listenAddr) overlaps(o listenAddr) bool {
	return l.port == o.port && (l.host == o.host || l.host == "" || o.host == "")
}

// validateExtraListenAddrs requires host:port entries with a numeric port
// and rejects entries that overlap listen_addr, the effective
// http_listen_addr or another extra entry (":443", "0.0.0.0:443" and
// "[::]:443" are the same listener; a wildcard overlaps every host on that
// port).
func (c *Config) validateExtraListenAddrs() error {
	var taken []listenAddr
	for _, a := range []string{c.ListenAddr, c.effectiveHTTPListenAddr()} {
		if a == "" {
			continue
		}
		if l, err := parseListenAddr(a); err == nil {
			taken = append(taken, l)
		}
	}
	for _, a := range c.ExtraListenAddrs {
		l, err := parseListenAddr(a)
		if err != nil {
			if _, _, splitErr := net.SplitHostPort(a); splitErr != nil {
				return fmt.Errorf("extra_listen_addrs: %q: want host:port (e.g. \":50051\"): %v", a, splitErr)
			}
			return fmt.Errorf("extra_listen_addrs: %q: %v", a, err)
		}
		for _, t := range taken {
			if l.overlaps(t) {
				return fmt.Errorf("extra_listen_addrs: %q duplicates %q (listen_addr, http_listen_addr or another extra address on the same port)", a, t.raw)
			}
		}
		taken = append(taken, l)
	}
	return nil
}
