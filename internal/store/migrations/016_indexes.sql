CREATE INDEX IF NOT EXISTS idx_alert_history_active ON alert_history(rule_id, resolved_at);
CREATE INDEX IF NOT EXISTS idx_alert_history_fired_at ON alert_history(fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_runs_started_at ON backup_runs(started_at DESC);
