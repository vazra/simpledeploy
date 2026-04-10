package store

import (
	"database/sql"
	"fmt"
	"time"
)

type BackupConfig struct {
	ID               int64
	AppID            int64
	Strategy         string
	Target           string
	ScheduleCron     string
	TargetConfigJSON string
	RetentionCount   int
	CreatedAt        time.Time
}

type BackupRun struct {
	ID             int64
	BackupConfigID int64
	Status         string
	SizeBytes      *int64
	StartedAt      time.Time
	FinishedAt     *time.Time
	ErrorMsg       string
	FilePath       string
}

func (s *Store) CreateBackupConfig(cfg *BackupConfig) error {
	err := s.db.QueryRow(`
		INSERT INTO backup_configs (app_id, strategy, target, schedule_cron, target_config_json, retention_count)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, created_at
	`, cfg.AppID, cfg.Strategy, cfg.Target, cfg.ScheduleCron, cfg.TargetConfigJSON, cfg.RetentionCount).
		Scan(&cfg.ID, &cfg.CreatedAt)
	if err != nil {
		return fmt.Errorf("create backup config: %w", err)
	}
	return nil
}

func (s *Store) ListBackupConfigs(appID *int64) ([]BackupConfig, error) {
	var rows *sql.Rows
	var err error
	if appID == nil {
		rows, err = s.db.Query(`
			SELECT id, app_id, strategy, target, schedule_cron, target_config_json, retention_count, created_at
			FROM backup_configs ORDER BY id
		`)
	} else {
		rows, err = s.db.Query(`
			SELECT id, app_id, strategy, target, schedule_cron, target_config_json, retention_count, created_at
			FROM backup_configs WHERE app_id = ? ORDER BY id
		`, *appID)
	}
	if err != nil {
		return nil, fmt.Errorf("list backup configs: %w", err)
	}
	defer rows.Close()

	var cfgs []BackupConfig
	for rows.Next() {
		var c BackupConfig
		if err := rows.Scan(&c.ID, &c.AppID, &c.Strategy, &c.Target, &c.ScheduleCron, &c.TargetConfigJSON, &c.RetentionCount, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backup config: %w", err)
		}
		cfgs = append(cfgs, c)
	}
	return cfgs, rows.Err()
}

func (s *Store) GetBackupConfig(id int64) (*BackupConfig, error) {
	var c BackupConfig
	err := s.db.QueryRow(`
		SELECT id, app_id, strategy, target, schedule_cron, target_config_json, retention_count, created_at
		FROM backup_configs WHERE id = ?
	`, id).Scan(&c.ID, &c.AppID, &c.Strategy, &c.Target, &c.ScheduleCron, &c.TargetConfigJSON, &c.RetentionCount, &c.CreatedAt)
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

func (s *Store) UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath string) error {
	res, err := s.db.Exec(`
		UPDATE backup_runs
		SET status = 'success', size_bytes = ?, file_path = ?, finished_at = datetime('now')
		WHERE id = ?
	`, sizeBytes, filePath, id)
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

func (s *Store) ListBackupRuns(configID int64) ([]BackupRun, error) {
	rows, err := s.db.Query(`
		SELECT id, backup_config_id, status, size_bytes, started_at, finished_at, error_msg, file_path
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
		SELECT id, backup_config_id, status, size_bytes, started_at, finished_at, error_msg, file_path
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
		SELECT id, backup_config_id, status, size_bytes, started_at, finished_at, error_msg, file_path
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

type scanner interface {
	Scan(...any) error
}

func scanBackupRun(row scanner) (BackupRun, error) {
	var r BackupRun
	var sizeBytes sql.NullInt64
	var finishedAt sql.NullTime
	var errorMsg, filePath sql.NullString
	if err := row.Scan(&r.ID, &r.BackupConfigID, &r.Status, &sizeBytes, &r.StartedAt, &finishedAt, &errorMsg, &filePath); err != nil {
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
	return r, nil
}
