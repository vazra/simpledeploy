CREATE TABLE IF NOT EXISTS backup_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    strategy TEXT NOT NULL CHECK(strategy IN ('postgres', 'volume')),
    target TEXT NOT NULL CHECK(target IN ('s3', 'local')),
    schedule_cron TEXT NOT NULL,
    target_config_json TEXT NOT NULL DEFAULT '{}',
    retention_count INTEGER NOT NULL DEFAULT 7,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS backup_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_config_id INTEGER NOT NULL REFERENCES backup_configs(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running', 'success', 'failed')),
    size_bytes INTEGER,
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME,
    error_msg TEXT,
    file_path TEXT
);

CREATE INDEX IF NOT EXISTS idx_backup_configs_app ON backup_configs(app_id);
CREATE INDEX IF NOT EXISTS idx_backup_runs_config ON backup_runs(backup_config_id);
