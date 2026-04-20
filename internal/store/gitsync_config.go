package store

import "fmt"

// GetGitSyncConfig returns all key/value pairs from gitsync_config.
// Returns an empty map (not nil) when no rows exist.
func (s *Store) GetGitSyncConfig() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM gitsync_config")
	if err != nil {
		return nil, fmt.Errorf("GetGitSyncConfig: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("GetGitSyncConfig scan: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetGitSyncConfig atomically upserts every key/value pair in kvs.
func (s *Store) SetGitSyncConfig(kvs map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("SetGitSyncConfig begin: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO gitsync_config (key, value) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("SetGitSyncConfig prepare: %w", err)
	}
	defer stmt.Close()
	for k, v := range kvs {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("SetGitSyncConfig exec %q: %w", k, err)
		}
	}
	return tx.Commit()
}

// DeleteGitSyncConfig removes all rows from gitsync_config (disables DB overrides).
func (s *Store) DeleteGitSyncConfig() error {
	_, err := s.db.Exec("DELETE FROM gitsync_config")
	if err != nil {
		return fmt.Errorf("DeleteGitSyncConfig: %w", err)
	}
	return nil
}
