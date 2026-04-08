CREATE TABLE IF NOT EXISTS metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER REFERENCES apps(id) ON DELETE CASCADE,
    container_id TEXT,
    cpu_pct REAL NOT NULL DEFAULT 0,
    mem_bytes INTEGER NOT NULL DEFAULT 0,
    mem_limit INTEGER NOT NULL DEFAULT 0,
    net_rx INTEGER NOT NULL DEFAULT 0,
    net_tx INTEGER NOT NULL DEFAULT 0,
    disk_read INTEGER NOT NULL DEFAULT 0,
    disk_write INTEGER NOT NULL DEFAULT 0,
    timestamp DATETIME NOT NULL,
    tier TEXT NOT NULL DEFAULT 'raw' CHECK(tier IN ('raw', '1m', '5m', '1h'))
);

CREATE INDEX IF NOT EXISTS idx_metrics_lookup ON metrics(app_id, tier, timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_tier_ts ON metrics(tier, timestamp);
