-- Backup System v2: drop and recreate backup tables with new schema
-- Safe because pre-production, no existing data to preserve

DROP TABLE IF EXISTS backup_runs;
DROP TABLE IF EXISTS backup_configs;

CREATE TABLE backup_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    strategy TEXT NOT NULL CHECK(strategy IN ('postgres','mysql','mongo','redis','sqlite','volume')),
    target TEXT NOT NULL CHECK(target IN ('s3','local')),
    schedule_cron TEXT NOT NULL,
    target_config_json TEXT NOT NULL DEFAULT '{}',
    retention_mode TEXT NOT NULL DEFAULT 'count' CHECK(retention_mode IN ('count','time')),
    retention_count INTEGER NOT NULL DEFAULT 7,
    retention_days INTEGER,
    verify_upload INTEGER NOT NULL DEFAULT 0,
    pre_hooks TEXT,
    post_hooks TEXT,
    paths TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE backup_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_config_id INTEGER NOT NULL REFERENCES backup_configs(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running','success','failed')),
    size_bytes INTEGER,
    checksum TEXT,
    file_path TEXT,
    compose_version_id INTEGER REFERENCES compose_versions(id),
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME,
    error_msg TEXT
);

CREATE INDEX idx_backup_configs_app ON backup_configs(app_id);
CREATE INDEX idx_backup_runs_config ON backup_runs(backup_config_id);
CREATE INDEX idx_backup_runs_status ON backup_runs(status);

-- Extend compose_versions with name, notes, env snapshot
ALTER TABLE compose_versions ADD COLUMN name TEXT;
ALTER TABLE compose_versions ADD COLUMN notes TEXT;
ALTER TABLE compose_versions ADD COLUMN env_snapshot TEXT;
