-- Expand apps.status CHECK to include 'unstable' (added by post-deploy
-- stabilization classifier) and 'failed' (terminal deploy failure).
-- SQLite CHECKs are table-bound, so recreate via rename/copy/swap.

CREATE TABLE apps_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    compose_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped' CHECK(status IN ('running', 'stopped', 'error', 'unstable', 'failed')),
    domain TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    compose_hash TEXT NOT NULL DEFAULT ''
);

INSERT INTO apps_new (id, name, slug, compose_path, status, domain, created_at, updated_at, compose_hash)
SELECT id, name, slug, compose_path, status, domain, created_at, updated_at, compose_hash FROM apps;

DROP TABLE apps;
ALTER TABLE apps_new RENAME TO apps;

CREATE INDEX IF NOT EXISTS idx_apps_slug ON apps(slug);
CREATE INDEX IF NOT EXISTS idx_apps_status ON apps(status);
