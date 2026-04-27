package configsync

import (
	"fmt"
	"path/filepath"
)

// LoadedApp aggregates an app's sidecar and secrets read from disk.
// Either field may be nil if the corresponding file is absent.
type LoadedApp struct {
	Slug    string
	Sidecar *AppSidecar
	Secrets *AppSecrets
}

// LoadAppFromFS reads {apps_dir}/{slug}/simpledeploy.yml and the matching
// secrets sidecar. Missing files are reported as nil fields, not errors.
func (s *Syncer) LoadAppFromFS(slug string) (*LoadedApp, error) {
	out := &LoadedApp{Slug: slug}
	sidecarPath := filepath.Join(s.appsDir, slug, appSidecarName)
	sc, err := readYAML[AppSidecar](sidecarPath)
	if err != nil {
		return nil, fmt.Errorf("read sidecar %s: %w", sidecarPath, err)
	}
	out.Sidecar = sc
	sec, err := s.ReadAppSecrets(slug)
	if err != nil {
		return nil, fmt.Errorf("read secrets %s: %w", slug, err)
	}
	out.Secrets = sec
	return out, nil
}

// LoadedGlobal aggregates the global sidecar and secrets read from disk.
// Either field may be nil if the corresponding file is absent.
type LoadedGlobal struct {
	Sidecar *GlobalSidecar
	Secrets *GlobalSecrets
}

// LoadGlobalFromFS reads {data_dir}/config.yml and the global secrets sidecar.
// Missing files are reported as nil fields, not errors.
func (s *Syncer) LoadGlobalFromFS() (*LoadedGlobal, error) {
	out := &LoadedGlobal{}
	path := filepath.Join(s.dataDir, globalSidecar)
	sc, err := readYAML[GlobalSidecar](path)
	if err != nil {
		return nil, err
	}
	out.Sidecar = sc
	sec, err := s.ReadGlobalSecrets()
	if err != nil {
		return nil, err
	}
	out.Secrets = sec
	return out, nil
}
