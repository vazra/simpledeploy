ALTER TABLE apps ADD COLUMN archived_at TIMESTAMP NULL;
CREATE INDEX idx_apps_archived_at ON apps(archived_at) WHERE archived_at IS NOT NULL;
