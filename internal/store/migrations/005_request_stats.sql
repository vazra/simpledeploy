CREATE TABLE IF NOT EXISTS request_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER REFERENCES apps(id) ON DELETE CASCADE,
    timestamp DATETIME NOT NULL,
    status_code INTEGER NOT NULL,
    latency_ms REAL NOT NULL,
    method TEXT NOT NULL,
    path_pattern TEXT NOT NULL,
    tier TEXT NOT NULL DEFAULT 'raw' CHECK(tier IN ('raw', '1m', '5m', '1h'))
);

CREATE INDEX IF NOT EXISTS idx_reqstats_lookup ON request_stats(app_id, tier, timestamp);
