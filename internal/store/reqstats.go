package store

import (
	"fmt"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
)

// RequestStat is an alias for metrics.RequestStat for backward compatibility.
type RequestStat = metrics.RequestStat

// InsertRequestStats batch-inserts request stats in a single transaction.
func (s *Store) InsertRequestStats(stats []metrics.RequestStat) error {
	if len(stats) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO request_stats
			(app_id, timestamp, status_code, latency_ms, method, path_pattern, tier)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, st := range stats {
		tier := st.Tier
		if tier == "" {
			tier = "raw"
		}
		if _, err := stmt.Exec(
			st.AppID,
			st.Timestamp.UTC().Format("2006-01-02 15:04:05"),
			st.StatusCode,
			st.LatencyMs,
			st.Method,
			st.PathPattern,
			tier,
		); err != nil {
			return fmt.Errorf("insert request stat: %w", err)
		}
	}

	return tx.Commit()
}

// QueryRequestStats returns request stats for the given app, tier, and time range.
func (s *Store) QueryRequestStats(appID int64, tier string, from, to time.Time) ([]metrics.RequestStat, error) {
	rows, err := s.db.Query(`
		SELECT app_id, timestamp, status_code, latency_ms, method, path_pattern
		FROM request_stats
		WHERE app_id = ? AND tier = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp
	`, appID, tier,
		from.UTC().Format("2006-01-02 15:04:05"),
		to.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return nil, fmt.Errorf("query request stats: %w", err)
	}
	defer rows.Close()

	var result []metrics.RequestStat
	for rows.Next() {
		var st metrics.RequestStat
		var ts string
		if err := rows.Scan(
			&st.AppID, &ts, &st.StatusCode, &st.LatencyMs, &st.Method, &st.PathPattern,
		); err != nil {
			return nil, fmt.Errorf("scan request stat: %w", err)
		}
		t, err := time.Parse("2006-01-02 15:04:05", ts)
		if err != nil {
			t, err = time.Parse(time.RFC3339, ts)
			if err != nil {
				return nil, fmt.Errorf("parse timestamp %q: %w", ts, err)
			}
		}
		st.Timestamp = t.UTC()
		st.Tier = tier
		result = append(result, st)
	}
	return result, rows.Err()
}

// AggregateRequestStats reads rows from sourceTier older than olderThan,
// groups them by app_id, method, path_pattern, status group, and time bucket,
// then inserts averaged rows as destTier.
func (s *Store) AggregateRequestStats(sourceTier, destTier string, olderThan time.Time) error {
	bucket := reqStatsBucket(destTier)

	query := fmt.Sprintf(`
		INSERT INTO request_stats (app_id, timestamp, status_code, latency_ms, method, path_pattern, tier)
		SELECT
			app_id,
			%s AS bucket,
			(status_code / 100) * 100,
			avg(latency_ms),
			method,
			path_pattern,
			?
		FROM request_stats
		WHERE tier = ? AND timestamp < ?
		GROUP BY app_id, method, path_pattern, (status_code / 100) * 100, bucket
	`, bucket)

	_, err := s.db.Exec(query,
		destTier,
		sourceTier,
		olderThan.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("aggregate request stats: %w", err)
	}
	return nil
}

// PruneRequestStats deletes request stats with the given tier older than before.
// Returns the number of rows deleted.
func (s *Store) PruneRequestStats(tier string, before time.Time) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM request_stats WHERE tier = ? AND timestamp < ?`,
		tier,
		before.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, fmt.Errorf("prune request stats: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

// reqStatsBucket returns the SQLite strftime expression for bucketing by destTier.
func reqStatsBucket(destTier string) string {
	switch destTier {
	case "1m":
		return `strftime('%Y-%m-%d %H:%M:00', timestamp)`
	case "5m":
		return `strftime('%Y-%m-%d %H:', timestamp) || printf('%02d:00', (CAST(strftime('%M', timestamp) AS INTEGER) / 5) * 5)`
	case "1h":
		return `strftime('%Y-%m-%d %H:00:00', timestamp)`
	default:
		return `strftime('%Y-%m-%d %H:%M:%S', timestamp)`
	}
}
