package client

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// configDir can be overridden in tests.
var configDir string

type ClientConfig struct {
	Contexts       map[string]Context `yaml:"contexts"`
	CurrentContext string             `yaml:"current_context"`
}

type Context struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

func ClientConfigPath() string {
	if configDir != "" {
		return filepath.Join(configDir, "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".simpledeploy", "config.yaml")
}

func LoadClientConfig() (*ClientConfig, error) {
	path := ClientConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ClientConfig{Contexts: make(map[string]Context)}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg ClientConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]Context)
	}
	return &cfg, nil
}

func SaveClientConfig(cfg *ClientConfig) error {
	path := ClientConfigPath()
	os.MkdirAll(filepath.Dir(path), 0700)
	data, _ := yaml.Marshal(cfg)
	return os.WriteFile(path, data, 0600)
}

func (cfg *ClientConfig) AddContext(name, url, apiKey string) {
	cfg.Contexts[name] = Context{URL: url, APIKey: apiKey}
}

func (cfg *ClientConfig) UseContext(name string) error {
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context %q not found", name)
	}
	cfg.CurrentContext = name
	return nil
}

func (cfg *ClientConfig) GetCurrentContext() (*Context, error) {
	if cfg.CurrentContext == "" {
		return nil, fmt.Errorf("no current context set")
	}
	ctx, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok {
		return nil, fmt.Errorf("context %q not found", cfg.CurrentContext)
	}
	return &ctx, nil
}
