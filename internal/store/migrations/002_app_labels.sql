CREATE TABLE IF NOT EXISTS app_labels (
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (app_id, key)
);

CREATE INDEX IF NOT EXISTS idx_app_labels_app ON app_labels(app_id);
