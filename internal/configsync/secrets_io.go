package configsync

import (
	"path/filepath"
)

const (
	appSecretsName    = "simpledeploy.secrets.yml"
	globalSecretsName = "secrets.yml"
)

func (s *Syncer) appSecretsPath(slug string) string {
	return filepath.Join(s.appsDir, slug, appSecretsName)
}

func (s *Syncer) globalSecretsPath() string {
	return filepath.Join(s.dataDir, globalSecretsName)
}

// WriteAppSecrets writes the per-app secrets sidecar at mode 0600.
func (s *Syncer) WriteAppSecrets(slug string, secrets *AppSecrets) error {
	path := s.appSecretsPath(slug)
	s.MarkSelfWrite(path)
	return atomicWriteYAMLMode(path, 0600, secrets)
}

// ReadAppSecrets reads the per-app secrets sidecar. Returns (nil, nil) if absent.
func (s *Syncer) ReadAppSecrets(slug string) (*AppSecrets, error) {
	return readYAML[AppSecrets](s.appSecretsPath(slug))
}

// WriteGlobalSecrets writes the global secrets sidecar at mode 0600.
func (s *Syncer) WriteGlobalSecrets(g *GlobalSecrets) error {
	path := s.globalSecretsPath()
	s.MarkSelfWrite(path)
	return atomicWriteYAMLMode(path, 0600, g)
}

// ReadGlobalSecrets reads the global secrets sidecar. Returns (nil, nil) if absent.
func (s *Syncer) ReadGlobalSecrets() (*GlobalSecrets, error) {
	return readYAML[GlobalSecrets](s.globalSecretsPath())
}
