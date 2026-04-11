package store

import (
	"fmt"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
)

// InsertRequestMetrics batch-inserts request metric points in a single transaction.
func (s *Store) InsertRequestMetrics(points []metrics.RequestMetricPoint) error {
	if len(points) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO request_metrics
			(app_id, ts, tier, count, error_count, avg_latency, max_latency)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range points {
		tier := p.Tier
		if tier == "" {
			tier = metrics.TierRaw
		}
		if _, err := stmt.Exec(
			p.AppID, p.Ts, tier,
			p.Count, p.ErrorCount,
			p.AvgLatency, p.MaxLatency,
		); err != nil {
			return fmt.Errorf("insert request metric: %w", err)
		}
	}

	return tx.Commit()
}

// QueryRequestMetrics returns request metric points for the given app and range.
// Returns the points, interval in seconds, and any error.
func (s *Store) QueryRequestMetrics(appID int64, rangeStr string) ([]metrics.RequestMetricPoint, int, error) {
	tier, intervalSec := SelectTier(rangeStr)

	now := time.Now().Unix()
	dur := rangeToDuration(rangeStr)
	from := now - int64(dur.Seconds())
	to := now

	rows, err := s.db.Query(`
		SELECT app_id, ts, tier, count, error_count, avg_latency, max_latency
		FROM request_metrics
		WHERE app_id = ? AND tier = ? AND ts >= ? AND ts <= ?
		ORDER BY ts
	`, appID, tier, from, to)
	if err != nil {
		return nil, 0, fmt.Errorf("query request metrics: %w", err)
	}
	defer rows.Close()

	var pts []metrics.RequestMetricPoint
	for rows.Next() {
		var p metrics.RequestMetricPoint
		if err := rows.Scan(
			&p.AppID, &p.Ts, &p.Tier,
			&p.Count, &p.ErrorCount,
			&p.AvgLatency, &p.MaxLatency,
		); err != nil {
			return nil, 0, fmt.Errorf("scan request metric: %w", err)
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return pts, intervalSec, nil
}

// AggregateRequestMetrics reads points from sourceTier older than olderThan,
// groups by app_id and time bucket, inserts as destTier.
func (s *Store) AggregateRequestMetrics(sourceTier, destTier string, olderThan time.Time) error {
	if err := validateMetricsTier(destTier); err != nil {
		return err
	}
	if err := validateMetricsTier(sourceTier); err != nil {
		return err
	}
	bucket := timeBucket(destTier)
	cutoff := olderThan.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("aggregate request metrics: begin tx: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`
		INSERT INTO request_metrics (app_id, ts, tier, count, error_count, avg_latency, max_latency)
		SELECT
			app_id,
			%s AS bucket_ts,
			?,
			sum(count),
			sum(error_count),
			CASE WHEN sum(count) > 0 THEN sum(avg_latency * count) / sum(count) ELSE 0 END,
			max(max_latency)
		FROM request_metrics
		WHERE tier = ? AND ts < ?
		GROUP BY app_id, bucket_ts
	`, bucket)

	if _, err := tx.Exec(query, destTier, sourceTier, cutoff); err != nil {
		return fmt.Errorf("aggregate request metrics: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM request_metrics WHERE tier = ? AND ts < ?`,
		sourceTier, cutoff,
	); err != nil {
		return fmt.Errorf("aggregate request metrics: delete source: %w", err)
	}

	return tx.Commit()
}

// PruneRequestMetrics deletes request metric points with the given tier older than before.
func (s *Store) PruneRequestMetrics(tier string, before time.Time) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM request_metrics WHERE tier = ? AND ts < ?`,
		tier, before.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("prune request metrics: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}
