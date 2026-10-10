package configsync

import (
	"path/filepath"
)

const (
	appSecretsName    = "simpledeploy.secrets.yml"
	globalSecretsName = "secrets.yml"
)

func (s *Syncer) globalSecretsPath() string {
	return filepath.Join(s.dataDir, globalSecretsName)
}

// WriteAppSecrets writes the per-app secrets sidecar at mode 0600.
func (s *Syncer) WriteAppSecrets(slug string, secrets *AppSecrets) error {
	path, err := s.appFilePath(slug, appSecretsName)
	if err != nil {
		return err
	}
	s.MarkSelfWrite(path)
	return atomicWriteYAMLMode(path, 0600, secrets)
}

// ReadAppSecrets reads the per-app secrets sidecar. Returns (nil, nil) if
// absent. A symlinked app directory or secrets file is refused.
func (s *Syncer) ReadAppSecrets(slug string) (*AppSecrets, error) {
	path, err := s.appFilePath(slug, appSecretsName)
	if err != nil {
		return nil, err
	}
	return readYAML[AppSecrets](path)
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
