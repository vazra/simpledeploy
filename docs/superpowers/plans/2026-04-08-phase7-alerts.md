# Phase 7: Alerts & Webhooks - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Alert rules evaluate metrics every 30s. When thresholds are breached for a configured duration, fire webhooks (Slack, Telegram, Discord, custom). Track alert history. Default rules auto-created per app. Resolved alerts fire optional recovery webhook.

**Architecture:** An evaluator goroutine runs every 30s, loads active rules, queries recent metrics, checks conditions. When fired, dispatches webhook via HTTP POST using Go templates for body formatting. Alert state (fired/resolved) tracked in alert_history. Webhook configs stored in webhooks table with built-in templates per type.

**Tech Stack:** text/template (webhook bodies), net/http (webhook dispatch), existing store/metrics packages

---

## File Structure

```
internal/alerts/evaluator.go        - Alert evaluation loop
internal/alerts/evaluator_test.go
internal/alerts/webhook.go          - Webhook dispatch + templates
internal/alerts/webhook_test.go
internal/alerts/types.go            - AlertRule, AlertEvent types

internal/store/alerts.go            - Alert rules, history, webhooks CRUD
internal/store/alerts_test.go
internal/store/migrations/006_alerts.sql

internal/api/alerts.go              - Alert/webhook management endpoints
internal/api/alerts_test.go

cmd/simpledeploy/main.go            - Wire evaluator into serve
```

---

### Task 1: Alerts Store

**Files:**
- Create: `internal/store/migrations/006_alerts.sql`
- Create: `internal/store/alerts.go`
- Create: `internal/store/alerts_test.go`

#### Migration:
```sql
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
```

#### Types:
```go
type Webhook struct {
    ID               int64
    Name             string
    Type             string // slack, telegram, discord, custom
    URL              string
    TemplateOverride string
    HeadersJSON      string
    CreatedAt        time.Time
}

type AlertRule struct {
    ID          int64
    AppID       *int64 // nil for system rules
    Metric      string // "cpu_pct", "mem_bytes", etc.
    Operator    string // ">", "<", ">=", "<="
    Threshold   float64
    DurationSec int
    WebhookID   int64
    Enabled     bool
    CreatedAt   time.Time
}

type AlertHistory struct {
    ID         int64
    RuleID     int64
    FiredAt    time.Time
    ResolvedAt *time.Time
    Value      float64
}
```

#### Methods:
- `CreateWebhook(w *Webhook) error`
- `GetWebhook(id int64) (*Webhook, error)`
- `ListWebhooks() ([]Webhook, error)`
- `DeleteWebhook(id int64) error`
- `CreateAlertRule(r *AlertRule) error`
- `ListAlertRules(appID *int64) ([]AlertRule, error)` - nil = all rules
- `ListActiveAlertRules() ([]AlertRule, error)` - enabled only
- `UpdateAlertRule(r *AlertRule) error`
- `DeleteAlertRule(id int64) error`
- `CreateAlertHistory(ruleID int64, value float64) (*AlertHistory, error)`
- `ResolveAlert(historyID int64) error` - set resolved_at
- `GetActiveAlert(ruleID int64) (*AlertHistory, error)` - get unresolved alert for rule
- `ListAlertHistory(ruleID *int64, limit int) ([]AlertHistory, error)`

#### Tests:
- TestWebhookCRUD
- TestAlertRuleCRUD
- TestAlertHistoryCreateAndResolve
- TestGetActiveAlert
- TestListActiveAlertRules

- [ ] Commit: `git commit -m "add alerts store: webhooks, rules, history"`

---

### Task 2: Webhook Dispatch + Templates

**Files:**
- Create: `internal/alerts/types.go`
- Create: `internal/alerts/webhook.go`
- Create: `internal/alerts/webhook_test.go`

#### Types:
```go
type AlertEvent struct {
    AppName   string
    AppSlug   string
    Metric    string
    Value     float64
    Threshold float64
    Operator  string
    Status    string // "firing", "resolved"
    FiredAt   time.Time
}
```

#### Webhook dispatch:
```go
type WebhookDispatcher struct {
    client *http.Client
}

func NewWebhookDispatcher() *WebhookDispatcher

func (d *WebhookDispatcher) Send(webhook store.Webhook, event AlertEvent) error
```

Send logic:
1. Select template based on webhook.Type (slack/telegram/discord/custom)
2. If webhook.TemplateOverride is set, use that instead
3. Execute Go text/template with AlertEvent as data
4. POST to webhook.URL with rendered body
5. Parse webhook.HeadersJSON for custom headers

#### Built-in templates:

```go
var templates = map[string]string{
    "slack": `{"text":"{{if eq .Status \"firing\"}}:red_circle:{{else}}:green_circle:{{end}} *{{.AppName}}* - {{.Metric}} {{.Operator}} {{.Threshold}} (current: {{printf \"%.1f\" .Value}}) - {{.Status}}"}`,

    "telegram": `{"chat_id":"{{.ChatID}}","text":"{{if eq .Status \"firing\"}}🔴{{else}}🟢{{end}} <b>{{.AppName}}</b>\n{{.Metric}} {{.Operator}} {{.Threshold}} (current: {{printf \"%.1f\" .Value}})\nStatus: {{.Status}}","parse_mode":"HTML"}`,

    "discord": `{"content":"{{if eq .Status \"firing\"}}🔴{{else}}🟢{{end}} **{{.AppName}}** - {{.Metric}} {{.Operator}} {{.Threshold}} (current: {{printf \"%.1f\" .Value}}) - {{.Status}}"}`,

    "custom": `{"app":"{{.AppName}}","metric":"{{.Metric}}","value":{{printf \"%.2f\" .Value}},"threshold":{{printf \"%.2f\" .Threshold}},"status":"{{.Status}}"}`,
}
```

#### Tests:
- TestRenderSlackTemplate
- TestRenderCustomTemplate
- TestRenderWithOverride
- TestWebhookSend (use httptest.NewServer to verify POST body)

- [ ] Commit: `git commit -m "add webhook dispatch with built-in templates"`

---

### Task 3: Alert Evaluator

**Files:**
- Create: `internal/alerts/evaluator.go`
- Create: `internal/alerts/evaluator_test.go`

#### Evaluator:
```go
type MetricQuerier interface {
    QueryMetrics(appID *int64, tier string, from, to time.Time) ([]metrics.MetricPoint, error)
}

type AlertStore interface {
    ListActiveAlertRules() ([]store.AlertRule, error)
    GetActiveAlert(ruleID int64) (*store.AlertHistory, error)
    CreateAlertHistory(ruleID int64, value float64) (*store.AlertHistory, error)
    ResolveAlert(historyID int64) error
    GetWebhook(id int64) (*store.Webhook, error)
    GetAppByID(id int64) (*store.App, error)
}

type Evaluator struct {
    store      AlertStore
    metrics    MetricQuerier
    dispatcher *WebhookDispatcher
}

func NewEvaluator(store AlertStore, metrics MetricQuerier, dispatcher *WebhookDispatcher) *Evaluator

func (e *Evaluator) Run(ctx context.Context, interval time.Duration)
func (e *Evaluator) EvaluateOnce(ctx context.Context) error
```

EvaluateOnce logic:
1. Load active rules
2. For each rule:
   a. Query metrics for last `duration_sec` seconds (use "raw" tier)
   b. Extract the metric field (cpu_pct, mem_bytes, etc.) from each point
   c. Check if ALL points satisfy the condition (operator + threshold)
   d. If condition met and no active alert: fire webhook, create alert_history
   e. If condition NOT met and active alert exists: resolve alert, fire resolved webhook

#### Condition checker:
```go
func checkCondition(value float64, operator string, threshold float64) bool {
    switch operator {
    case ">": return value > threshold
    case "<": return value < threshold
    case ">=": return value >= threshold
    case "<=": return value <= threshold
    }
    return false
}

func extractMetricValue(point metrics.MetricPoint, metric string) float64 {
    switch metric {
    case "cpu_pct": return point.CPUPct
    case "mem_bytes": return float64(point.MemBytes)
    case "mem_pct": return float64(point.MemBytes) / float64(point.MemLimit) * 100
    case "disk_read": return float64(point.DiskRead)
    case "disk_write": return float64(point.DiskWrite)
    }
    return 0
}
```

#### Tests (use mocks):
- TestCheckCondition - all operators
- TestExtractMetricValue - cpu_pct, mem_bytes, mem_pct
- TestEvaluateOnce_FiresAlert - metrics exceed threshold for full duration
- TestEvaluateOnce_NoFire - metrics below threshold
- TestEvaluateOnce_ResolvesAlert - previously fired, now below threshold

- [ ] Commit: `git commit -m "add alert evaluator with condition checking"`

---

### Task 4: Alert API Endpoints + Wire

**Files:**
- Create: `internal/api/alerts.go`
- Create: `internal/api/alerts_test.go`
- Modify: `internal/api/server.go`
- Modify: `cmd/simpledeploy/main.go`

#### Endpoints (all require auth):

```
GET    /api/webhooks              - list webhooks
POST   /api/webhooks              - create webhook
DELETE /api/webhooks/{id}         - delete webhook

GET    /api/alerts/rules          - list alert rules (?app_id=N optional)
POST   /api/alerts/rules          - create alert rule
PUT    /api/alerts/rules/{id}     - update alert rule
DELETE /api/alerts/rules/{id}     - delete alert rule

GET    /api/alerts/history        - list alert history (?rule_id=N, limit=50)
```

#### Wiring in main.go:

```go
dispatcher := alerts.NewWebhookDispatcher()
evaluator := alerts.NewEvaluator(db, db, dispatcher)
go evaluator.Run(ctx, 30*time.Second)
```

#### Tests:
- TestCreateWebhook
- TestListWebhooks
- TestCreateAlertRule
- TestListAlertRules
- TestAlertHistory

- [ ] Run full test suite, tidy, build
- [ ] Commit: `git commit -m "add alert API endpoints and wire evaluator"`

---

## Verification Checklist

- [ ] Webhooks CRUD (slack/telegram/discord/custom)
- [ ] Alert rules CRUD with enable/disable
- [ ] Alert evaluator runs every 30s
- [ ] Conditions: >, <, >=, <= on cpu_pct, mem_bytes, mem_pct
- [ ] Fires webhook when threshold breached for full duration
- [ ] Resolves alert when condition clears
- [ ] Built-in templates for Slack, Telegram, Discord
- [ ] Custom templates with Go text/template
- [ ] Alert history tracked
- [ ] API endpoints for webhooks, rules, history
- [ ] All tests pass
