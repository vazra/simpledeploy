CREATE TABLE IF NOT EXISTS webhooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('slack', 'telegram', 'discord', 'custom')),
    url TEXT NOT NULL,
    template_override TEXT,
    headers_json TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id INTEGER REFERENCES apps(id) ON DELETE CASCADE,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL CHECK(operator IN ('>', '<', '>=', '<=')),
    threshold REAL NOT NULL,
    duration_sec INTEGER NOT NULL DEFAULT 300,
    webhook_id INTEGER NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS alert_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id INTEGER NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    fired_at DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME,
    value REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_app ON alert_rules(app_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_rule ON alert_history(rule_id);
