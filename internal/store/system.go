package store

import (
	"database/sql"
	"fmt"
)

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

// TierStat holds the row count for a single tier in a time-series table.
type TierStat struct {
	Tier  string `json:"tier"`
	Count int64  `json:"count"`
}

// GetMetricsTierStats returns row counts grouped by tier for the metrics table.
func (s *Store) GetMetricsTierStats() ([]TierStat, error) {
	return queryTierStats(s.db, "metrics")
}

// GetRequestStatsTierStats returns row counts grouped by tier for the request_stats table.
func (s *Store) GetRequestStatsTierStats() ([]TierStat, error) {
	return queryTierStats(s.db, "request_stats")
}

func queryTierStats(db *sql.DB, table string) ([]TierStat, error) {
	// Whitelist table names to prevent SQL injection
	switch table {
	case "metrics", "request_stats":
		// allowed
	default:
		return nil, fmt.Errorf("invalid table name: %s", table)
	}
	rows, err := db.Query(`SELECT tier, COUNT(*) FROM ` + table + ` GROUP BY tier ORDER BY tier`)
	if err != nil {
		return []TierStat{}, err
	}
	defer rows.Close()
	stats := []TierStat{}
	for rows.Next() {
		var s TierStat
		if err := rows.Scan(&s.Tier, &s.Count); err != nil {
			return []TierStat{}, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
