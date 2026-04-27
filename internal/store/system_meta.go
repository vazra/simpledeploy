package store

import (
	"database/sql"
)

// GetMeta returns the value for the given key. The bool is false when the key
// does not exist.
func (s *Store) GetMeta(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM system_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SetMeta upserts a key/value pair into system_meta.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`
        INSERT INTO system_meta(key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
    `, key, value)
	return err
}
