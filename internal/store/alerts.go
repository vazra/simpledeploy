package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

type Webhook struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	URL              string    `json:"url"`
	TemplateOverride string    `json:"template_override"`
	HeadersJSON      string    `json:"headers_json"`
	CreatedAt        time.Time `json:"created_at"`
}

type AlertRule struct {
	ID          int64    `json:"id"`
	AppID       *int64   `json:"app_id"`
	AppSlug     string   `json:"app_slug,omitempty"`
	Metric      string   `json:"metric"`
	Operator    string   `json:"operator"`
	Threshold   float64  `json:"threshold"`
	DurationSec int      `json:"duration_sec"`
	WebhookID   int64    `json:"webhook_id"`
	Enabled     bool     `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type AlertHistory struct {
	ID         int64      `json:"id"`
	RuleID     int64      `json:"rule_id"`
	FiredAt    time.Time  `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	Value      float64    `json:"value"`
	Metric     string     `json:"metric"`
	AppSlug    string     `json:"app_slug"`
	Operator   string     `json:"operator"`
	Threshold  float64    `json:"threshold"`
}

func (s *Store) CreateWebhook(w *Webhook) error {
	err := s.db.QueryRow(`
		INSERT INTO webhooks (name, type, url, template_override, headers_json)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id, created_at
	`, w.Name, w.Type, w.URL, nullString(w.TemplateOverride), nullString(w.HeadersJSON)).
		Scan(&w.ID, &w.CreatedAt)
	if err != nil {
		return fmt.Errorf("create webhook: %w", err)
	}
	s.fireHook(ScopeGlobal, "")
	return nil
}

func (s *Store) GetWebhook(id int64) (*Webhook, error) {
	var w Webhook
	var tmpl, hdrs sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, type, url, template_override, headers_json, created_at
		FROM webhooks WHERE id = ?
	`, id).Scan(&w.ID, &w.Name, &w.Type, &w.URL, &tmpl, &hdrs, &w.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("webhook %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook: %w", err)
	}
	if tmpl.Valid {
		w.TemplateOverride = tmpl.String
	}
	if hdrs.Valid {
		w.HeadersJSON = hdrs.String
	}
	return &w, nil
}

func (s *Store) ListWebhooks() ([]Webhook, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, url, template_override, headers_json, created_at
		FROM webhooks ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()

	var whs []Webhook
	for rows.Next() {
		var w Webhook
		var tmpl, hdrs sql.NullString
		if err := rows.Scan(&w.ID, &w.Name, &w.Type, &w.URL, &tmpl, &hdrs, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		if tmpl.Valid {
			w.TemplateOverride = tmpl.String
		}
		if hdrs.Valid {
			w.HeadersJSON = hdrs.String
		}
		whs = append(whs, w)
	}
	return whs, rows.Err()
}

func (s *Store) UpdateWebhook(w *Webhook) error {
	res, err := s.db.Exec(`
		UPDATE webhooks
		SET name=?, type=?, url=?, template_override=?, headers_json=?
		WHERE id=?
	`, w.Name, w.Type, w.URL, nullString(w.TemplateOverride), nullString(w.HeadersJSON), w.ID)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("webhook %d not found", w.ID)
	}
	s.fireHook(ScopeGlobal, "")
	return nil
}

func (s *Store) DeleteWebhook(id int64) error {
	res, err := s.db.Exec(`DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("webhook %d not found", id)
	}
	s.fireHook(ScopeGlobal, "")
	return nil
}

func (s *Store) CreateAlertRule(r *AlertRule) error {
	var appID interface{}
	if r.AppID != nil {
		appID = *r.AppID
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	err := s.db.QueryRow(`
		INSERT INTO alert_rules (app_id, metric, operator, threshold, duration_sec, webhook_id, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at
	`, appID, r.Metric, r.Operator, r.Threshold, r.DurationSec, r.WebhookID, enabled).
		Scan(&r.ID, &r.CreatedAt)
	if err != nil {
		return fmt.Errorf("create alert rule: %w", err)
	}
	if r.AppID != nil {
		s.fireAppHook(*r.AppID)
	} else {
		s.fireHook(ScopeGlobal, "")
	}
	return nil
}

func scanAlertRule(row interface {
	Scan(...any) error
}) (AlertRule, error) {
	var r AlertRule
	var appID sql.NullInt64
	var enabled int
	if err := row.Scan(
		&r.ID, &appID, &r.Metric, &r.Operator, &r.Threshold,
		&r.DurationSec, &r.WebhookID, &enabled, &r.CreatedAt,
	); err != nil {
		return AlertRule{}, err
	}
	if appID.Valid {
		id := appID.Int64
		r.AppID = &id
	}
	r.Enabled = enabled != 0
	return r, nil
}

func (s *Store) ListAlertRules(appID *int64) ([]AlertRule, error) {
	var rows *sql.Rows
	var err error
	if appID == nil {
		rows, err = s.db.Query(`
			SELECT id, app_id, metric, operator, threshold, duration_sec, webhook_id, enabled, created_at
			FROM alert_rules ORDER BY id
		`)
	} else {
		rows, err = s.db.Query(`
			SELECT id, app_id, metric, operator, threshold, duration_sec, webhook_id, enabled, created_at
			FROM alert_rules WHERE app_id = ? ORDER BY id
		`, *appID)
	}
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()

	var rules []AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan alert rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *Store) ListActiveAlertRules() ([]AlertRule, error) {
	rows, err := s.db.Query(`
		SELECT id, app_id, metric, operator, threshold, duration_sec, webhook_id, enabled, created_at
		FROM alert_rules WHERE enabled = 1 ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list active alert rules: %w", err)
	}
	defer rows.Close()

	var rules []AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan alert rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *Store) UpdateAlertRule(r *AlertRule) error {
	if !r.Enabled {
		_ = s.ResolveAlertsByRule(r.ID)
	}
	var appID interface{}
	if r.AppID != nil {
		appID = *r.AppID
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	res, err := s.db.Exec(`
		UPDATE alert_rules
		SET app_id=?, metric=?, operator=?, threshold=?, duration_sec=?, webhook_id=?, enabled=?
		WHERE id=?
	`, appID, r.Metric, r.Operator, r.Threshold, r.DurationSec, r.WebhookID, enabled, r.ID)
	if err != nil {
		return fmt.Errorf("update alert rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert rule %d not found", r.ID)
	}
	if _, err := s.db.Exec(`
		UPDATE alert_history SET metric=?, app_slug=?, operator=?, threshold=?
		WHERE rule_id=? AND resolved_at IS NULL
	`, r.Metric, r.AppSlug, r.Operator, r.Threshold, r.ID); err != nil {
		log.Printf("[store] update alert history snapshot: %v", err)
	}
	if r.AppID != nil {
		s.fireAppHook(*r.AppID)
	} else {
		s.fireHook(ScopeGlobal, "")
	}
	return nil
}

func (s *Store) DeleteAlertRule(id int64) error {
	// Fetch rule before delete to know scope for hook.
	row := s.db.QueryRow(`SELECT id, app_id, metric, operator, threshold, duration_sec, webhook_id, enabled, created_at FROM alert_rules WHERE id = ?`, id)
	rule, err := scanAlertRule(row)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("scan alert rule: %w", err)
	}

	_ = s.ResolveAlertsByRule(id)
	res, err := s.db.Exec(`DELETE FROM alert_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert rule %d not found", id)
	}
	if rule.AppID != nil {
		s.fireAppHook(*rule.AppID)
	} else {
		s.fireHook(ScopeGlobal, "")
	}
	return nil
}

func (s *Store) CreateAlertHistory(ruleID int64, value float64, rule *AlertRule) (*AlertHistory, error) {
	var h AlertHistory
	h.RuleID = ruleID
	h.Value = value
	var metric, appSlug, operator string
	var threshold float64
	if rule != nil {
		metric = rule.Metric
		appSlug = rule.AppSlug
		operator = rule.Operator
		threshold = rule.Threshold
	}
	err := s.db.QueryRow(`
		INSERT INTO alert_history (rule_id, value, metric, app_slug, operator, threshold)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, fired_at
	`, ruleID, value, metric, appSlug, operator, threshold).Scan(&h.ID, &h.FiredAt)
	if err != nil {
		return nil, fmt.Errorf("create alert history: %w", err)
	}
	h.Metric = metric
	h.AppSlug = appSlug
	h.Operator = operator
	h.Threshold = threshold
	return &h, nil
}

func (s *Store) ResolveAlertsByRule(ruleID int64) error {
	_, err := s.db.Exec(`
		UPDATE alert_history SET resolved_at = datetime('now')
		WHERE rule_id = ? AND resolved_at IS NULL
	`, ruleID)
	if err != nil {
		return fmt.Errorf("resolve alerts for rule %d: %w", ruleID, err)
	}
	return nil
}

func (s *Store) ResolveAlert(historyID int64) error {
	res, err := s.db.Exec(`
		UPDATE alert_history SET resolved_at = datetime('now') WHERE id = ?
	`, historyID)
	if err != nil {
		return fmt.Errorf("resolve alert: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert history %d not found", historyID)
	}
	return nil
}

func (s *Store) GetActiveAlert(ruleID int64) (*AlertHistory, error) {
	var h AlertHistory
	err := s.db.QueryRow(`
		SELECT id, rule_id, fired_at, resolved_at, value
		FROM alert_history
		WHERE rule_id = ? AND resolved_at IS NULL
		ORDER BY fired_at DESC LIMIT 1
	`, ruleID).Scan(&h.ID, &h.RuleID, &h.FiredAt, new(sql.NullTime), &h.Value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active alert: %w", err)
	}
	return &h, nil
}

func (s *Store) ListAlertHistory(ruleID *int64, limit int) ([]AlertHistory, error) {
	var rows *sql.Rows
	var err error
	const cols = `id, rule_id, fired_at, resolved_at, value, metric, app_slug, operator, threshold`
	if ruleID == nil {
		rows, err = s.db.Query(`
			SELECT `+cols+` FROM alert_history ORDER BY fired_at DESC LIMIT ?
		`, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT `+cols+` FROM alert_history WHERE rule_id = ? ORDER BY fired_at DESC LIMIT ?
		`, *ruleID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list alert history: %w", err)
	}
	defer rows.Close()

	var hist []AlertHistory
	for rows.Next() {
		var h AlertHistory
		var resolvedAt sql.NullTime
		if err := rows.Scan(&h.ID, &h.RuleID, &h.FiredAt, &resolvedAt, &h.Value, &h.Metric, &h.AppSlug, &h.Operator, &h.Threshold); err != nil {
			return nil, fmt.Errorf("scan alert history: %w", err)
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time
			h.ResolvedAt = &t
		}
		hist = append(hist, h)
	}
	return hist, rows.Err()
}

// UpsertWebhookByName inserts or updates a webhook by name.
// If a webhook with this name already exists it is updated in-place.
// Used by configsync ImportGlobal.
func (s *Store) UpsertWebhookByName(w *Webhook) error {
	var existing Webhook
	var tmpl, hdrs sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, type, url, template_override, headers_json, created_at
		FROM webhooks WHERE name = ?
	`, w.Name).Scan(&existing.ID, &existing.Name, &existing.Type, &existing.URL, &tmpl, &hdrs, &existing.CreatedAt)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("upsert webhook lookup %q: %w", w.Name, err)
	}
	if err == sql.ErrNoRows {
		// Insert new.
		w2 := &Webhook{Name: w.Name, Type: w.Type, URL: w.URL, TemplateOverride: w.TemplateOverride, HeadersJSON: w.HeadersJSON}
		if err := s.CreateWebhook(w2); err != nil {
			return fmt.Errorf("upsert webhook insert %q: %w", w.Name, err)
		}
		w.ID = w2.ID
		return nil
	}
	// Update existing.
	w.ID = existing.ID
	w2 := &Webhook{ID: existing.ID, Name: w.Name, Type: w.Type, URL: w.URL, TemplateOverride: w.TemplateOverride, HeadersJSON: w.HeadersJSON}
	if err := s.UpdateWebhook(w2); err != nil {
		return fmt.Errorf("upsert webhook update %q: %w", w.Name, err)
	}
	return nil
}

// UpsertWebhookFromRedacted updates type if webhook exists; inserts with empty URL if new.
// Preserves URL, headers_json, and template_override on update.
func (s *Store) UpsertWebhookFromRedacted(name, kind string) error {
	var existing Webhook
	var tmpl, hdrs sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, type, url, template_override, headers_json, created_at
		FROM webhooks WHERE name = ?
	`, name).Scan(&existing.ID, &existing.Name, &existing.Type, &existing.URL, &tmpl, &hdrs, &existing.CreatedAt)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("UpsertWebhookFromRedacted lookup %q: %w", name, err)
	}
	if err == sql.ErrNoRows {
		w := &Webhook{Name: name, Type: kind, URL: ""}
		if err := s.CreateWebhook(w); err != nil {
			return fmt.Errorf("UpsertWebhookFromRedacted insert %q: %w", name, err)
		}
		return nil
	}
	// Update type only; leave URL/headers/template untouched.
	_, err = s.db.Exec(`UPDATE webhooks SET type = ? WHERE id = ?`, kind, existing.ID)
	if err != nil {
		return fmt.Errorf("UpsertWebhookFromRedacted update %q: %w", name, err)
	}
	s.fireHook(ScopeGlobal, "")
	return nil
}

// DeleteAlertRulesForApp deletes all alert rules for the given app.
// Used by configsync ImportAppSidecar for full-replace semantics.
func (s *Store) DeleteAlertRulesForApp(appID int64) error {
	rules, err := s.ListAlertRules(&appID)
	if err != nil {
		return fmt.Errorf("list alert rules for app %d: %w", appID, err)
	}
	for _, r := range rules {
		_ = s.ResolveAlertsByRule(r.ID)
	}
	if _, err := s.db.Exec(`DELETE FROM alert_rules WHERE app_id = ?`, appID); err != nil {
		return fmt.Errorf("delete alert rules for app %d: %w", appID, err)
	}
	s.fireAppHook(appID)
	return nil
}

func (s *Store) ClearAlertHistory(resolvedOnly bool) error {
	var q string
	if resolvedOnly {
		q = `DELETE FROM alert_history WHERE resolved_at IS NOT NULL`
	} else {
		q = `DELETE FROM alert_history`
	}
	_, err := s.db.Exec(q)
	if err != nil {
		return fmt.Errorf("clear alert history: %w", err)
	}
	return nil
}
