package store

import "database/sql"

// DBStats holds aggregate counts for the system info endpoint.
type DBStats struct {
	Apps         int64 `json:"apps"`
	Users        int64 `json:"users"`
	Metrics      int64 `json:"metrics"`
	RequestStats int64 `json:"request_stats"`
	AlertRules   int64 `json:"alert_rules"`
	BackupRuns   int64 `json:"backup_runs"`
	MigrationVer int64 `json:"migration_version"`
}

// GetDBStats returns aggregate row counts and the current migration version.
func (s *Store) GetDBStats() (DBStats, error) {
	var d DBStats
	queries := []struct {
		dest  *int64
		query string
	}{
		{&d.Apps, "SELECT COUNT(*) FROM apps"},
		{&d.Users, "SELECT COUNT(*) FROM users"},
		{&d.Metrics, "SELECT COUNT(*) FROM metrics"},
		{&d.RequestStats, "SELECT COUNT(*) FROM request_stats"},
		{&d.AlertRules, "SELECT COUNT(*) FROM alert_rules"},
		{&d.BackupRuns, "SELECT COUNT(*) FROM backup_runs"},
	}
	for _, q := range queries {
		if err := s.db.QueryRow(q.query).Scan(q.dest); err != nil {
			return d, err
		}
	}
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&d.MigrationVer)
	if err == sql.ErrNoRows {
		err = nil
	}
	return d, err
}

// VacuumDB runs VACUUM on the SQLite database to reclaim unused space.
func (s *Store) VacuumDB() error {
	_, err := s.db.Exec("VACUUM")
	return err
}
