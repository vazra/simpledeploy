CREATE TABLE compose_versions (
    id INTEGER PRIMARY KEY,
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    hash TEXT NOT NULL,
    created_at DATETIME DEFAULT (datetime('now')),
    UNIQUE(app_id, version)
);

CREATE TABLE deploy_events (
    id INTEGER PRIMARY KEY,
    app_slug TEXT NOT NULL,
    action TEXT NOT NULL,
    user_id INTEGER,
    detail TEXT,
    created_at DATETIME DEFAULT (datetime('now'))
);
