-- Allow gitsync conflict alerts that are not tied to an alert_rule.

PRAGMA foreign_keys=OFF;

ALTER TABLE alert_history RENAME TO alert_history_old;

CREATE TABLE alert_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id     INTEGER REFERENCES alert_rules(id) ON DELETE CASCADE,
    fired_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME,
    value       REAL NOT NULL,
    metric      TEXT NOT NULL DEFAULT '',
    app_slug    TEXT NOT NULL DEFAULT '',
    operator    TEXT NOT NULL DEFAULT '',
    threshold   REAL NOT NULL DEFAULT 0
);

INSERT INTO alert_history SELECT * FROM alert_history_old;

DROP TABLE alert_history_old;

CREATE INDEX IF NOT EXISTS idx_alert_history_rule     ON alert_history(rule_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_active   ON alert_history(rule_id, resolved_at);
CREATE INDEX IF NOT EXISTS idx_alert_history_fired_at ON alert_history(fired_at DESC);

PRAGMA foreign_keys=ON;
