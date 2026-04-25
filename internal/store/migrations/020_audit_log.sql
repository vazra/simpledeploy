CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY,
    app_id INTEGER REFERENCES apps(id) ON DELETE SET NULL,
    app_slug TEXT,
    actor_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    actor_name TEXT,
    actor_source TEXT NOT NULL,
    ip TEXT,
    category TEXT NOT NULL,
    action TEXT NOT NULL,
    target TEXT,
    summary TEXT NOT NULL,
    before_json TEXT,
    after_json TEXT,
    error TEXT,
    compose_version_id INTEGER REFERENCES compose_versions(id) ON DELETE SET NULL,
    sync_status TEXT,
    sync_commit_sha TEXT,
    sync_error TEXT,
    created_at DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX idx_audit_app_created ON audit_log(app_id, created_at DESC);
CREATE INDEX idx_audit_created ON audit_log(created_at DESC);
CREATE INDEX idx_audit_sync_pending ON audit_log(sync_status) WHERE sync_status = 'pending';
