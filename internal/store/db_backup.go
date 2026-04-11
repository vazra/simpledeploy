package store

import "fmt"

type DBBackupRun struct {
	ID        int64  `json:"id"`
	FilePath  string `json:"file_path"`
	SizeBytes int64  `json:"size_bytes"`
	Status    string `json:"status"`
	Compact   bool   `json:"compact"`
	ErrorMsg  string `json:"error_msg,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) GetDBBackupConfig() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM db_backup_config")
	if err != nil {
		return nil, fmt.Errorf("query db_backup_config: %w", err)
	}
	defer rows.Close()

	cfg := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan db_backup_config: %w", err)
		}
		cfg[k] = v
	}
	return cfg, rows.Err()
}

func (s *Store) SetDBBackupConfig(key, value string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO db_backup_config (key, value) VALUES (?, ?)", key, value)
	if err != nil {
		return fmt.Errorf("upsert db_backup_config: %w", err)
	}
	return nil
}

func (s *Store) CreateDBBackupRun(filePath string, sizeBytes int64, compact bool, status string, errMsg string) error {
	compactInt := 0
	if compact {
		compactInt = 1
	}
	_, err := s.db.Exec(
		"INSERT INTO db_backup_runs (file_path, size_bytes, status, compact, error_msg) VALUES (?, ?, ?, ?, ?)",
		filePath, sizeBytes, status, compactInt, errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert db_backup_run: %w", err)
	}
	return nil
}

func (s *Store) ListDBBackupRuns(limit int) ([]DBBackupRun, error) {
	rows, err := s.db.Query(
		"SELECT id, file_path, size_bytes, status, compact, COALESCE(error_msg, ''), created_at FROM db_backup_runs ORDER BY created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query db_backup_runs: %w", err)
	}
	defer rows.Close()

	var runs []DBBackupRun
	for rows.Next() {
		var r DBBackupRun
		var compactInt int
		if err := rows.Scan(&r.ID, &r.FilePath, &r.SizeBytes, &r.Status, &compactInt, &r.ErrorMsg, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan db_backup_run: %w", err)
		}
		r.Compact = compactInt != 0
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) PruneDBBackupRuns(keepCount int) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT id, file_path FROM db_backup_runs ORDER BY created_at DESC LIMIT -1 OFFSET ?",
		keepCount,
	)
	if err != nil {
		return nil, fmt.Errorf("query prunable db_backup_runs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	var paths []string
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, fmt.Errorf("scan prunable db_backup_run: %w", err)
		}
		ids = append(ids, id)
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, id := range ids {
		if _, err := s.db.Exec("DELETE FROM db_backup_runs WHERE id = ?", id); err != nil {
			return nil, fmt.Errorf("delete db_backup_run %d: %w", id, err)
		}
	}

	return paths, nil
}

func (s *Store) VacuumInto(destPath string) error {
	_, err := s.db.Exec("VACUUM INTO ?", destPath)
	if err != nil {
		return fmt.Errorf("vacuum into %s: %w", destPath, err)
	}
	return nil
}
