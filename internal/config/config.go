package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DataDir        string          `yaml:"data_dir"`
	AppsDir        string          `yaml:"apps_dir"`
	ListenAddr     string          `yaml:"listen_addr"`
	ManagementPort int             `yaml:"management_port"`
	Domain         string          `yaml:"domain"`
	TLS            TLSConfig       `yaml:"tls"`
	MasterSecret   string          `yaml:"master_secret"`
	Metrics        MetricsConfig   `yaml:"metrics"`
	RateLimit      RateLimitConfig `yaml:"ratelimit"`
	Registries     []string        `yaml:"registries"`
	TrustedProxies []string        `yaml:"trusted_proxies"`
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
		TLS: TLSConfig{
			Mode: "auto",
		},
		Metrics: MetricsConfig{
			Tiers: []MetricsTier{
				{Name: "raw", Interval: "10s", Retention: "24h"},
				{Name: "1m", Retention: "7d"},
				{Name: "5m", Retention: "30d"},
				{Name: "1h", Retention: "8760h"},
			},
		},
		RateLimit: RateLimitConfig{
			Requests: 200,
			Window:   "60s",
			Burst:    50,
			By:       "ip",
		},
	}
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
	return cfg, nil
}

func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}
