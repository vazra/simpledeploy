package configsync

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const archiveDirName = "archive"

// Tombstone is the snapshot written to {data_dir}/archive/<slug>.yml when
// an app is archived (its dir disappeared from apps_dir).
type Tombstone struct {
	Version       int                 `yaml:"version" json:"version"`
	ArchivedAt    time.Time           `yaml:"archived_at" json:"archived_at"`
	App           AppMeta             `yaml:"app" json:"app"`
	AlertRules    []AlertRuleEntry    `yaml:"alert_rules,omitempty" json:"alert_rules,omitempty"`
	BackupConfigs []BackupConfigEntry `yaml:"backup_configs,omitempty" json:"backup_configs,omitempty"`
	Access        []AccessEntry       `yaml:"access,omitempty" json:"access,omitempty"`
}

// ArchiveDir returns the directory where tombstones are stored.
func (s *Syncer) ArchiveDir() string {
	return filepath.Join(s.dataDir, archiveDirName)
}

// WriteTombstone builds a tombstone from current DB state for the slug and
// writes it atomically to {data_dir}/archive/<slug>.yml (mode 0644).
func (s *Syncer) WriteTombstone(slug string, archivedAt time.Time) error {
	sidecar, err := s.buildAppSidecar(slug)
	if err != nil {
		return fmt.Errorf("build sidecar: %w", err)
	}
	tomb := Tombstone{
		Version:       Version,
		ArchivedAt:    archivedAt.UTC(),
		App:           sidecar.App,
		AlertRules:    sidecar.AlertRules,
		BackupConfigs: sidecar.BackupConfigs,
		Access:        sidecar.Access,
	}
	return s.writeTombstoneFile(slug, &tomb)
}

func (s *Syncer) writeTombstoneFile(slug string, t *Tombstone) error {
	if err := os.MkdirAll(s.ArchiveDir(), 0755); err != nil {
		return err
	}
	path := filepath.Join(s.ArchiveDir(), slug+".yml")
	out, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadTombstone reads the tombstone for a slug. Returns the parsed struct or an
// os error (use os.IsNotExist to detect a missing file).
func (s *Syncer) ReadTombstone(slug string) (*Tombstone, error) {
	path := filepath.Join(s.ArchiveDir(), slug+".yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Tombstone
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// DeleteTombstone removes the tombstone for a slug. Missing file is not an error.
func (s *Syncer) DeleteTombstone(slug string) error {
	path := filepath.Join(s.ArchiveDir(), slug+".yml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
