DROP TABLE IF EXISTS metrics;
DROP TABLE IF EXISTS request_stats;

CREATE TABLE metrics (
    app_id       INTEGER REFERENCES apps(id) ON DELETE CASCADE,
    container_id TEXT NOT NULL DEFAULT '',
    ts           INTEGER NOT NULL,
    tier         TEXT NOT NULL DEFAULT 'raw' CHECK(tier IN ('raw','1m','5m','1h','1d')),
    cpu_pct      REAL NOT NULL DEFAULT 0,
    mem_bytes    INTEGER NOT NULL DEFAULT 0,
    mem_limit    INTEGER NOT NULL DEFAULT 0,
    net_rx       REAL NOT NULL DEFAULT 0,
    net_tx       REAL NOT NULL DEFAULT 0,
    disk_read    REAL NOT NULL DEFAULT 0,
    disk_write   REAL NOT NULL DEFAULT 0
);
CREATE INDEX idx_metrics_app_tier_ts ON metrics(app_id, tier, ts);
CREATE INDEX idx_metrics_tier_ts ON metrics(tier, ts);

CREATE TABLE request_metrics (
    app_id      INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    ts          INTEGER NOT NULL,
    tier        TEXT NOT NULL DEFAULT 'raw' CHECK(tier IN ('raw','1m','5m','1h','1d')),
    count       INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    avg_latency REAL NOT NULL DEFAULT 0,
    max_latency REAL NOT NULL DEFAULT 0
);
CREATE INDEX idx_reqmetrics_app_tier_ts ON request_metrics(app_id, tier, ts);
