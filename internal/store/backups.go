package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type BackupConfig struct {
	ID               int64     `json:"id"`
	AppID            int64     `json:"app_id"`
	Strategy         string    `json:"strategy"`
	Target           string    `json:"target"`
	ScheduleCron     string    `json:"schedule_cron"`
	TargetConfigJSON string    `json:"target_config_json"`
	RetentionMode    string    `json:"retention_mode"`
	RetentionCount   int       `json:"retention_count"`
	RetentionDays    *int      `json:"retention_days"`
	VerifyUpload     bool      `json:"verify_upload"`
	PreHooks         string    `json:"pre_hooks"`
	PostHooks        string    `json:"post_hooks"`
	Paths            string    `json:"paths"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type BackupRun struct {
	ID               int64      `json:"id"`
	BackupConfigID   int64      `json:"backup_config_id"`
	Status           string     `json:"status"`
	SizeBytes        *int64     `json:"size_bytes"`
	Checksum         string     `json:"checksum"`
	FilePath         string     `json:"file_path"`
	ComposeVersionID *int64     `json:"compose_version_id"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	ErrorMsg         string     `json:"error_msg"`
}

// BackupSummaryApp holds aggregated backup health for one app.
type BackupSummaryApp struct {
	AppSlug            string   `json:"app_slug"`
	AppName            string   `json:"app_name"`
	ConfigCount        int      `json:"config_count"`
	Strategies         []string `json:"strategies"`
	LastRunStatus      string   `json:"last_run_status"`
	LastRunFinishedAt  *string  `json:"last_run_finished_at"`
	LastRunSizeBytes   *int64   `json:"last_run_size_bytes"`
	TotalSizeBytes     int64    `json:"total_size_bytes"`
	RecentSuccessCount int      `json:"recent_success_count"`
	RecentFailCount    int      `json:"recent_fail_count"`
	NextCron           string   `json:"next_cron"`
}

// BackupRunWithApp extends BackupRun with app and strategy info for cross-app views.
type BackupRunWithApp struct {
	BackupRun
	AppName  string `json:"app_name"`
	AppSlug  string `json:"app_slug"`
	Strategy string `json:"strategy"`
}

func (s *Store) GetBackupSummary() ([]BackupSummaryApp, error) {
	rows, err := s.db.Query(`
		SELECT
			a.slug,
			a.name,
			COUNT(DISTINCT bc.id)                                          AS config_count,
			GROUP_CONCAT(DISTINCT bc.strategy)                            AS strategies,
			(SELECT br.status FROM backup_runs br
			 JOIN backup_configs bc2 ON br.backup_config_id = bc2.id
			 WHERE bc2.app_id = a.id
			 ORDER BY br.started_at DESC LIMIT 1)                         AS last_run_status,
			(SELECT br.finished_at FROM backup_runs br
			 JOIN backup_configs bc2 ON br.backup_config_id = bc2.id
			 WHERE bc2.app_id = a.id
			 ORDER BY br.started_at DESC LIMIT 1)                         AS last_run_finished_at,
			(SELECT br.size_bytes FROM backup_runs br
			 JOIN backup_configs bc2 ON br.backup_config_id = bc2.id
			 WHERE bc2.app_id = a.id
			 ORDER BY br.started_at DESC LIMIT 1)                         AS last_run_size_bytes,
			COALESCE(SUM(CASE WHEN br.status = 'success' THEN br.size_bytes ELSE 0 END), 0) AS total_size_bytes,
			COUNT(CASE WHEN br.status = 'success' AND br.started_at >= datetime('now', '-24 hours') THEN 1 END) AS recent_success_count,
			COUNT(CASE WHEN br.status = 'failed'  AND br.started_at >= datetime('now', '-24 hours') THEN 1 END) AS recent_fail_count,
			MIN(bc.schedule_cron)                                         AS next_cron
		FROM apps a
		JOIN backup_configs bc ON bc.app_id = a.id
		LEFT JOIN backup_runs br ON br.backup_config_id = bc.id
		GROUP BY a.id, a.slug, a.name
		ORDER BY a.name
	`)
	if err != nil {
		return nil, fmt.Errorf("get backup summary: %w", err)
	}
	defer rows.Close()

	var summaries []BackupSummaryApp
	for rows.Next() {
		var sum BackupSummaryApp
		var strategiesCSV string
		var lastRunStatus, lastRunFinishedAt sql.NullString
		var lastRunSizeBytes sql.NullInt64
		if err := rows.Scan(
			&sum.AppSlug,
			&sum.AppName,
			&sum.ConfigCount,
			&strategiesCSV,
			&lastRunStatus,
			&lastRunFinishedAt,
			&lastRunSizeBytes,
			&sum.TotalSizeBytes,
			&sum.RecentSuccessCount,
			&sum.RecentFailCount,
			&sum.NextCron,
		); err != nil {
			return nil, fmt.Errorf("scan backup summary: %w", err)
		}
		sum.Strategies = splitCSV(strategiesCSV)
		if lastRunStatus.Valid {
			sum.LastRunStatus = lastRunStatus.String
		}
		if lastRunFinishedAt.Valid {
			v := lastRunFinishedAt.String
			sum.LastRunFinishedAt = &v
		}
		if lastRunSizeBytes.Valid {
			v := lastRunSizeBytes.Int64
			sum.LastRunSizeBytes = &v
		}
		summaries = append(summaries, sum)
	}
	return summaries, rows.Err()
}

func (s *Store) ListRecentBackupRuns(limit int) ([]BackupRunWithApp, error) {
	rows, err := s.db.Query(`
		SELECT
			br.id, br.backup_config_id, br.status, br.size_bytes,
			br.checksum, br.file_path, br.compose_version_id,
			br.started_at, br.finished_at, br.error_msg,
			a.name, a.slug, bc.strategy
		FROM backup_runs br
		JOIN backup_configs bc ON bc.id = br.backup_config_id
		JOIN apps a ON a.id = bc.app_id
		ORDER BY br.started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent backup runs: %w", err)
	}
	defer rows.Close()

	var runs []BackupRunWithApp
	for rows.Next() {
		var r BackupRunWithApp
		var sizeBytes sql.NullInt64
		var finishedAt sql.NullTime
		var errorMsg, filePath, checksum sql.NullString
		var composeVersionID sql.NullInt64
		if err := rows.Scan(
			&r.ID, &r.BackupConfigID, &r.Status, &sizeBytes,
			&checksum, &filePath, &composeVersionID,
			&r.StartedAt, &finishedAt, &errorMsg,
			&r.AppName, &r.AppSlug, &r.Strategy,
		); err != nil {
			return nil, fmt.Errorf("scan recent backup run: %w", err)
		}
		if sizeBytes.Valid {
			v := sizeBytes.Int64
			r.SizeBytes = &v
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			r.FinishedAt = &t
		}
		if errorMsg.Valid {
			r.ErrorMsg = errorMsg.String
		}
		if filePath.Valid {
			r.FilePath = filePath.String
		}
		if checksum.Valid {
			r.Checksum = checksum.String
		}
		if composeVersionID.Valid {
			v := composeVersionID.Int64
			r.ComposeVersionID = &v
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (s *Store) CreateBackupConfig(cfg *BackupConfig) error {
	err := s.db.QueryRow(`
		INSERT INTO backup_configs (app_id, strategy, target, schedule_cron, target_config_json,
			retention_mode, retention_count, retention_days, verify_upload, pre_hooks, post_hooks, paths)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at, updated_at
	`, cfg.AppID, cfg.Strategy, cfg.Target, cfg.ScheduleCron, cfg.TargetConfigJSON,
		cfg.RetentionMode, cfg.RetentionCount, cfg.RetentionDays, cfg.VerifyUpload,
		cfg.PreHooks, cfg.PostHooks, cfg.Paths).
		Scan(&cfg.ID, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create backup config: %w", err)
	}
	return nil
}

func (s *Store) UpdateBackupConfig(cfg *BackupConfig) error {
	res, err := s.db.Exec(`
		UPDATE backup_configs SET
			strategy = ?, target = ?, schedule_cron = ?, target_config_json = ?,
			retention_mode = ?, retention_count = ?, retention_days = ?,
			verify_upload = ?, pre_hooks = ?, post_hooks = ?, paths = ?,
			updated_at = datetime('now')
		WHERE id = ?
	`, cfg.Strategy, cfg.Target, cfg.ScheduleCron, cfg.TargetConfigJSON,
		cfg.RetentionMode, cfg.RetentionCount, cfg.RetentionDays,
		cfg.VerifyUpload, cfg.PreHooks, cfg.PostHooks, cfg.Paths,
		cfg.ID)
	if err != nil {
		return fmt.Errorf("update backup config: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("backup config %d not found", cfg.ID)
	}
	return nil
}

const backupConfigCols = `id, app_id, strategy, target, schedule_cron, target_config_json,
	retention_mode, retention_count, retention_days, verify_upload,
	pre_hooks, post_hooks, paths, created_at, updated_at`

func scanBackupConfig(row scanner) (BackupConfig, error) {
	var c BackupConfig
	var retentionDays sql.NullInt64
	var verifyUpload int
	var preHooks, postHooks, paths sql.NullString
	if err := row.Scan(&c.ID, &c.AppID, &c.Strategy, &c.Target, &c.ScheduleCron, &c.TargetConfigJSON,
		&c.RetentionMode, &c.RetentionCount, &retentionDays, &verifyUpload,
		&preHooks, &postHooks, &paths, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return BackupConfig{}, err
	}
	if retentionDays.Valid {
		v := int(retentionDays.Int64)
		c.RetentionDays = &v
	}
	c.VerifyUpload = verifyUpload != 0
	if preHooks.Valid {
		c.PreHooks = preHooks.String
	}
	if postHooks.Valid {
		c.PostHooks = postHooks.String
	}
	if paths.Valid {
		c.Paths = paths.String
	}
	return c, nil
}

func (s *Store) ListBackupConfigs(appID *int64) ([]BackupConfig, error) {
	var rows *sql.Rows
	var err error
	if appID == nil {
		rows, err = s.db.Query(`SELECT ` + backupConfigCols + ` FROM backup_configs ORDER BY id`)
	} else {
		rows, err = s.db.Query(`SELECT `+backupConfigCols+` FROM backup_configs WHERE app_id = ? ORDER BY id`, *appID)
	}
	if err != nil {
		return nil, fmt.Errorf("list backup configs: %w", err)
	}
	defer rows.Close()

	var cfgs []BackupConfig
	for rows.Next() {
		c, err := scanBackupConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("scan backup config: %w", err)
		}
		cfgs = append(cfgs, c)
	}
	return cfgs, rows.Err()
}

func (s *Store) GetBackupConfig(id int64) (*BackupConfig, error) {
	c, err := scanBackupConfig(s.db.QueryRow(`SELECT `+backupConfigCols+` FROM backup_configs WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backup config %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get backup config: %w", err)
	}
	return &c, nil
}

func (s *Store) DeleteBackupConfig(id int64) error {
	res, err := s.db.Exec(`DELETE FROM backup_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup config: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("backup config %d not found", id)
	}
	return nil
}

func (s *Store) CreateBackupRun(configID int64) (*BackupRun, error) {
	var r BackupRun
	r.BackupConfigID = configID
	err := s.db.QueryRow(`
		INSERT INTO backup_runs (backup_config_id)
		VALUES (?)
		RETURNING id, status, started_at
	`, configID).Scan(&r.ID, &r.Status, &r.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("create backup run: %w", err)
	}
	return &r, nil
}

func (s *Store) UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath, checksum string) error {
	res, err := s.db.Exec(`
		UPDATE backup_runs
		SET status = 'success', size_bytes = ?, file_path = ?, checksum = ?, finished_at = datetime('now')
		WHERE id = ?
	`, sizeBytes, filePath, checksum, id)
	if err != nil {
		return fmt.Errorf("update backup run success: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("backup run %d not found", id)
	}
	return nil
}

func (s *Store) UpdateBackupRunFailed(id int64, errMsg string) error {
	res, err := s.db.Exec(`
		UPDATE backup_runs
		SET status = 'failed', error_msg = ?, finished_at = datetime('now')
		WHERE id = ?
	`, errMsg, id)
	if err != nil {
		return fmt.Errorf("update backup run failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("backup run %d not found", id)
	}
	return nil
}

const backupRunCols = `id, backup_config_id, status, size_bytes, checksum, file_path, compose_version_id, started_at, finished_at, error_msg`

func (s *Store) ListBackupRuns(configID int64) ([]BackupRun, error) {
	rows, err := s.db.Query(`
		SELECT `+backupRunCols+`
		FROM backup_runs WHERE backup_config_id = ? ORDER BY started_at DESC
	`, configID)
	if err != nil {
		return nil, fmt.Errorf("list backup runs: %w", err)
	}
	defer rows.Close()

	var runs []BackupRun
	for rows.Next() {
		r, err := scanBackupRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan backup run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) GetBackupRun(id int64) (*BackupRun, error) {
	r, err := scanBackupRun(s.db.QueryRow(`
		SELECT `+backupRunCols+`
		FROM backup_runs WHERE id = ?
	`, id))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backup run %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get backup run: %w", err)
	}
	return &r, nil
}

func (s *Store) ListOldBackupRuns(configID int64, keepCount int) ([]BackupRun, error) {
	rows, err := s.db.Query(`
		SELECT `+backupRunCols+`
		FROM backup_runs
		WHERE backup_config_id = ? AND status = 'success'
		ORDER BY started_at DESC
		LIMIT -1 OFFSET ?
	`, configID, keepCount)
	if err != nil {
		return nil, fmt.Errorf("list old backup runs: %w", err)
	}
	defer rows.Close()

	var runs []BackupRun
	for rows.Next() {
		r, err := scanBackupRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan backup run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) ListOldBackupRunsByTime(configID int64, maxAgeDays int) ([]BackupRun, error) {
	rows, err := s.db.Query(`
		SELECT `+backupRunCols+`
		FROM backup_runs
		WHERE backup_config_id = ? AND status = 'success'
		  AND started_at < datetime('now', ? || ' days')
		ORDER BY started_at DESC
	`, configID, fmt.Sprintf("-%d", maxAgeDays))
	if err != nil {
		return nil, fmt.Errorf("list old backup runs by time: %w", err)
	}
	defer rows.Close()

	var runs []BackupRun
	for rows.Next() {
		r, err := scanBackupRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan backup run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

type scanner interface {
	Scan(...any) error
}

func scanBackupRun(row scanner) (BackupRun, error) {
	var r BackupRun
	var sizeBytes sql.NullInt64
	var finishedAt sql.NullTime
	var errorMsg, filePath, checksum sql.NullString
	var composeVersionID sql.NullInt64
	if err := row.Scan(&r.ID, &r.BackupConfigID, &r.Status, &sizeBytes,
		&checksum, &filePath, &composeVersionID,
		&r.StartedAt, &finishedAt, &errorMsg); err != nil {
		return BackupRun{}, err
	}
	if sizeBytes.Valid {
		v := sizeBytes.Int64
		r.SizeBytes = &v
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		r.FinishedAt = &t
	}
	if errorMsg.Valid {
		r.ErrorMsg = errorMsg.String
	}
	if filePath.Valid {
		r.FilePath = filePath.String
	}
	if checksum.Valid {
		r.Checksum = checksum.String
	}
	if composeVersionID.Valid {
		v := composeVersionID.Int64
		r.ComposeVersionID = &v
	}
	return r, nil
}
