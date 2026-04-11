package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
)

// InsertMetrics batch-inserts metric points in a single transaction.
func (s *Store) InsertMetrics(points []metrics.MetricPoint) error {
	if len(points) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO metrics
			(app_id, container_id, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write, timestamp, tier)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		var appID interface{}
		if p.AppID != nil {
			appID = *p.AppID
		}
		if _, err := stmt.Exec(
			appID, p.ContainerID,
			p.CPUPct, p.MemBytes, p.MemLimit,
			p.NetRx, p.NetTx,
			p.DiskRead, p.DiskWrite,
			p.Timestamp.UTC().Format("2006-01-02 15:04:05"),
			tier,
		); err != nil {
			return fmt.Errorf("insert metric: %w", err)
		}
	}

	return tx.Commit()
}

// QueryMetrics returns metric points for the given app (nil = system/no-app), tier, and time range.
func (s *Store) QueryMetrics(appID *int64, tier string, from, to time.Time) ([]metrics.MetricPoint, error) {
	var rows *sql.Rows
	var err error

	if appID == nil {
		rows, err = s.db.Query(`
			SELECT app_id, container_id, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write, timestamp
			FROM metrics
			WHERE app_id IS NULL AND tier = ? AND timestamp >= ? AND timestamp <= ?
			ORDER BY timestamp
		`, tier,
			from.UTC().Format("2006-01-02 15:04:05"),
			to.UTC().Format("2006-01-02 15:04:05"),
		)
	} else {
		rows, err = s.db.Query(`
			SELECT app_id, container_id, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write, timestamp
			FROM metrics
			WHERE app_id = ? AND tier = ? AND timestamp >= ? AND timestamp <= ?
			ORDER BY timestamp
		`, *appID, tier,
			from.UTC().Format("2006-01-02 15:04:05"),
			to.UTC().Format("2006-01-02 15:04:05"),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var pts []metrics.MetricPoint
	for rows.Next() {
		var p metrics.MetricPoint
		var dbAppID sql.NullInt64
		var ts string
		if err := rows.Scan(
			&dbAppID, &p.ContainerID,
			&p.CPUPct, &p.MemBytes, &p.MemLimit,
			&p.NetRx, &p.NetTx,
			&p.DiskRead, &p.DiskWrite,
			&ts,
		); err != nil {
			return nil, fmt.Errorf("scan metric: %w", err)
		}
		if dbAppID.Valid {
			id := dbAppID.Int64
			p.AppID = &id
		}
		p.Tier = tier
		t, err := time.Parse("2006-01-02 15:04:05", ts)
		if err != nil {
			// try RFC3339 fallback
			t, err = time.Parse(time.RFC3339, ts)
			if err != nil {
				return nil, fmt.Errorf("parse timestamp %q: %w", ts, err)
			}
		}
		p.Timestamp = t.UTC()
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

// AggregateMetrics reads raw points from sourceTier older than olderThan,
// groups them by app_id, container_id, and a time bucket, then inserts
// averages/sums as destTier rows.
func (s *Store) AggregateMetrics(sourceTier, destTier string, olderThan time.Time) error {
	if err := validateMetricsTier(destTier); err != nil {
		return err
	}
	if err := validateMetricsTier(sourceTier); err != nil {
		return err
	}
	bucket := timeBucket(destTier)

	query := fmt.Sprintf(`
		INSERT INTO metrics (app_id, container_id, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write, timestamp, tier)
		SELECT
			app_id,
			container_id,
			avg(cpu_pct),
			max(mem_bytes),
			max(mem_limit),
			sum(net_rx),
			sum(net_tx),
			sum(disk_read),
			sum(disk_write),
			%s AS bucket,
			?
		FROM metrics
		WHERE tier = ? AND timestamp < ?
		GROUP BY app_id, container_id, bucket
	`, bucket)

	_, err := s.db.Exec(query,
		destTier,
		sourceTier,
		olderThan.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("aggregate metrics: %w", err)
	}
	return nil
}

// PruneMetrics deletes metric points with the given tier older than before.
// Returns the number of rows deleted.
func (s *Store) PruneMetrics(tier string, before time.Time) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM metrics WHERE tier = ? AND timestamp < ?`,
		tier,
		before.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, fmt.Errorf("prune metrics: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

// SelectTier picks the appropriate metrics tier for a query duration.
// <= 1h: raw, <= 24h: 1m, <= 7d: 5m, else: 1h
func SelectTier(duration time.Duration) string {
	switch {
	case duration <= time.Hour:
		return metrics.TierRaw
	case duration <= 24*time.Hour:
		return metrics.Tier1m
	case duration <= 7*24*time.Hour:
		return metrics.Tier5m
	default:
		return metrics.Tier1h
	}
}

func validateMetricsTier(tier string) error {
	switch tier {
	case metrics.TierRaw, metrics.Tier1m, metrics.Tier5m, metrics.Tier1h:
		return nil
	default:
		return fmt.Errorf("invalid metrics tier: %s", tier)
	}
}

// timeBucket returns the SQLite strftime expression for bucketing by destTier.
func timeBucket(destTier string) string {
	switch destTier {
	case metrics.Tier1m:
		return `strftime('%Y-%m-%d %H:%M:00', timestamp)`
	case metrics.Tier5m:
		return `strftime('%Y-%m-%d %H:', timestamp) || printf('%02d:00', (CAST(strftime('%M', timestamp) AS INTEGER) / 5) * 5)`
	case metrics.Tier1h:
		return `strftime('%Y-%m-%d %H:00:00', timestamp)`
	default:
		return `strftime('%Y-%m-%d %H:%M:%S', timestamp)`
	}
}
