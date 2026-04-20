package store

import "fmt"

const GitSyncConflictMetric = "gitsync_conflict"

// InsertConflictAlert records a gitsync conflict in alert_history using
// rule_id=0 as a sentinel (no real alert rule). The metric column is set to
// GitSyncConflictMetric so callers can filter by it.
func (s *Store) InsertConflictAlert(path, remoteSHA, description string) error {
	_, err := s.db.Exec(`
		INSERT INTO alert_history (rule_id, value, metric, app_slug, operator, threshold)
		VALUES (0, 0, ?, ?, ?, 0)
	`, GitSyncConflictMetric, path, description)
	if err != nil {
		return fmt.Errorf("InsertConflictAlert: %w", err)
	}
	return nil
}
