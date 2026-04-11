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
			(app_id, container_id, ts, tier, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write)
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
			p.Ts, tier,
			p.CPUPct, p.MemBytes, p.MemLimit,
			p.NetRx, p.NetTx,
			p.DiskRead, p.DiskWrite,
		); err != nil {
			return fmt.Errorf("insert metric: %w", err)
		}
	}

	return tx.Commit()
}

// SelectTiers returns the tiers to query (primary + feeder tiers with unrolled
// data) and the display interval for a range string.
func SelectTiers(rangeStr string) ([]string, int) {
	switch rangeStr {
	case "1h":
		return []string{metrics.TierRaw}, 10
	case "6h":
		return []string{metrics.Tier1m, metrics.TierRaw}, 60
	case "24h":
		return []string{metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 300
	case "1w":
		return []string{metrics.Tier1h, metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 3600
	case "1m":
		return []string{metrics.Tier1h, metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 3600
	case "1yr":
		return []string{metrics.Tier1d, metrics.Tier1h, metrics.Tier5m, metrics.Tier1m, metrics.TierRaw}, 86400
	default:
		return []string{metrics.TierRaw}, 10
	}
}

// rangeToDuration converts a range string to a time.Duration.
func rangeToDuration(rangeStr string) time.Duration {
	switch rangeStr {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "1w":
		return 7 * 24 * time.Hour
	case "1m":
		return 30 * 24 * time.Hour
	case "1yr":
		return 365 * 24 * time.Hour
	default:
		return time.Hour
	}
}

// QueryMetrics returns metric points for the given app and range string.
// Returns the points, the interval in seconds, and any error.
func (s *Store) QueryMetrics(appID *int64, rangeStr string) ([]metrics.MetricPoint, int, error) {
	tiers, intervalSec := SelectTiers(rangeStr)

	now := time.Now().Unix()
	dur := rangeToDuration(rangeStr)
	from := now - int64(dur.Seconds())
	to := now

	var appFilter string
	args := make([]interface{}, 0, 5+len(tiers))
	if appID == nil {
		appFilter = "app_id IS NULL"
	} else {
		appFilter = "app_id = ?"
		args = append(args, *appID)
	}
	// Build tier IN clause
	tierPlaceholders := ""
	for i, t := range tiers {
		if i > 0 {
			tierPlaceholders += ", "
		}
		tierPlaceholders += "?"
		args = append(args, t)
	}
	args = append(args, from, to)

	query := fmt.Sprintf(`
		SELECT app_id, container_id, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write, ts, tier
		FROM metrics
		WHERE %s AND tier IN (%s) AND ts >= ? AND ts <= ?
		ORDER BY container_id, ts
	`, appFilter, tierPlaceholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var pts []metrics.MetricPoint
	for rows.Next() {
		var p metrics.MetricPoint
		var dbAppID sql.NullInt64
		// avg() in SQLite returns float64, so scan mem_bytes/mem_limit as float then cast
		var memBytes, memLimit float64
		if err := rows.Scan(
			&dbAppID, &p.ContainerID,
			&p.CPUPct, &memBytes, &memLimit,
			&p.NetRx, &p.NetTx,
			&p.DiskRead, &p.DiskWrite,
			&p.Ts, &p.Tier,
		); err != nil {
			return nil, 0, fmt.Errorf("scan metric: %w", err)
		}
		p.MemBytes = int64(memBytes)
		p.MemLimit = int64(memLimit)
		if dbAppID.Valid {
			id := dbAppID.Int64
			p.AppID = &id
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return pts, intervalSec, nil
}

// AggregateMetrics reads points from sourceTier older than olderThan,
// groups by app_id, container_id, and time bucket, inserts as destTier.
func (s *Store) AggregateMetrics(sourceTier, destTier string, olderThan time.Time) error {
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
		return fmt.Errorf("aggregate metrics: begin tx: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`
		INSERT INTO metrics (app_id, container_id, ts, tier, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, disk_read, disk_write)
		SELECT
			app_id,
			container_id,
			%s AS bucket_ts,
			?,
			avg(cpu_pct),
			avg(mem_bytes),
			max(mem_limit),
			avg(net_rx),
			avg(net_tx),
			avg(disk_read),
			avg(disk_write)
		FROM metrics
		WHERE tier = ? AND ts < ?
		GROUP BY app_id, container_id, bucket_ts
	`, bucket)

	if _, err := tx.Exec(query, destTier, sourceTier, cutoff); err != nil {
		return fmt.Errorf("aggregate metrics: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM metrics WHERE tier = ? AND ts < ?`,
		sourceTier, cutoff,
	); err != nil {
		return fmt.Errorf("aggregate metrics: delete source: %w", err)
	}

	return tx.Commit()
}

// PruneMetrics deletes metric points with the given tier older than before.
func (s *Store) PruneMetrics(tier string, before time.Time) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM metrics WHERE tier = ? AND ts < ?`,
		tier, before.Unix(),
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

func validateMetricsTier(tier string) error {
	switch tier {
	case metrics.TierRaw, metrics.Tier1m, metrics.Tier5m, metrics.Tier1h, metrics.Tier1d:
		return nil
	default:
		return fmt.Errorf("invalid metrics tier: %s", tier)
	}
}

// timeBucket returns integer arithmetic expression for bucketing by destTier.
func timeBucket(destTier string) string {
	switch destTier {
	case metrics.Tier1m:
		return "(ts / 60) * 60"
	case metrics.Tier5m:
		return "(ts / 300) * 300"
	case metrics.Tier1h:
		return "(ts / 3600) * 3600"
	case metrics.Tier1d:
		return "(ts / 86400) * 86400"
	default:
		return "ts"
	}
}
