ALTER TABLE backup_configs ADD COLUMN uuid TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_backup_configs_uuid ON backup_configs(uuid) WHERE uuid IS NOT NULL;
