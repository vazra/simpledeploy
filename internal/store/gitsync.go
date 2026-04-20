package store

import (
	"database/sql"
	"fmt"
)

const GitSyncConflictMetric = "gitsync_conflict"

// InsertConflictAlert records a gitsync conflict in alert_history with a NULL
// rule_id (no real alert rule). The metric column is set to
// GitSyncConflictMetric so callers can filter by it.
func (s *Store) InsertConflictAlert(path, remoteSHA, description string) error {
	_, err := s.db.Exec(`
		INSERT INTO alert_history (rule_id, value, metric, app_slug, operator, threshold)
		VALUES (?, 0, ?, ?, ?, 0)
	`, sql.NullInt64{Valid: false}, GitSyncConflictMetric, path, description)
	if err != nil {
		return fmt.Errorf("InsertConflictAlert: %w", err)
	}
	return nil
}
