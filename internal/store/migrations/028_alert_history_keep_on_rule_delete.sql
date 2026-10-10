-- Keep alert history when its rule is deleted: rows already carry a snapshot
-- of the rule (metric, app_slug, operator, threshold), so rule_id becomes NULL
-- instead of cascading. Migrations run with foreign keys off.

CREATE TABLE alert_history_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id     INTEGER REFERENCES alert_rules(id) ON DELETE SET NULL,
    fired_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME,
    value       REAL NOT NULL,
    metric      TEXT NOT NULL DEFAULT '',
    app_slug    TEXT NOT NULL DEFAULT '',
    operator    TEXT NOT NULL DEFAULT '',
    threshold   REAL NOT NULL DEFAULT 0
);

INSERT INTO alert_history_new (id, rule_id, fired_at, resolved_at, value, metric, app_slug, operator, threshold)
SELECT id,
       CASE WHEN rule_id IN (SELECT id FROM alert_rules) THEN rule_id ELSE NULL END,
       fired_at, resolved_at, value, metric, app_slug, operator, threshold
FROM alert_history;

DROP TABLE alert_history;
ALTER TABLE alert_history_new RENAME TO alert_history;

CREATE INDEX IF NOT EXISTS idx_alert_history_rule     ON alert_history(rule_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_active   ON alert_history(rule_id, resolved_at);
CREATE INDEX IF NOT EXISTS idx_alert_history_fired_at ON alert_history(fired_at DESC);
