package store

import (
	"testing"
)

func makeTestWebhook(t *testing.T, s *Store) *Webhook {
	t.Helper()
	w := &Webhook{
		Name: "test-hook",
		Type: "slack",
		URL:  "https://hooks.slack.com/test",
	}
	if err := s.CreateWebhook(w); err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	return w
}

func TestWebhookCRUD(t *testing.T) {
	s := newTestStore(t)

	w := &Webhook{
		Name:             "my-hook",
		Type:             "discord",
		URL:              "https://discord.com/api/webhooks/123",
		TemplateOverride: `{"content":"{{.Message}}"}`,
		HeadersJSON:      `{"X-Custom":"value"}`,
	}
	if err := s.CreateWebhook(w); err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if w.ID == 0 {
		t.Fatal("expected ID to be set after create")
	}
	if w.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set after create")
	}

	got, err := s.GetWebhook(w.ID)
	if err != nil {
		t.Fatalf("GetWebhook: %v", err)
	}
	if got.Name != w.Name {
		t.Errorf("Name = %q, want %q", got.Name, w.Name)
	}
	if got.Type != w.Type {
		t.Errorf("Type = %q, want %q", got.Type, w.Type)
	}
	if got.URL != w.URL {
		t.Errorf("URL = %q, want %q", got.URL, w.URL)
	}
	if got.TemplateOverride != w.TemplateOverride {
		t.Errorf("TemplateOverride = %q, want %q", got.TemplateOverride, w.TemplateOverride)
	}
	if got.HeadersJSON != w.HeadersJSON {
		t.Errorf("HeadersJSON = %q, want %q", got.HeadersJSON, w.HeadersJSON)
	}

	// add second webhook
	w2 := &Webhook{Name: "hook2", Type: "telegram", URL: "https://api.telegram.org/bot123/sendMessage"}
	if err := s.CreateWebhook(w2); err != nil {
		t.Fatalf("CreateWebhook w2: %v", err)
	}

	list, err := s.ListWebhooks()
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	if err := s.DeleteWebhook(w.ID); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	list, err = s.ListWebhooks()
	if err != nil {
		t.Fatalf("ListWebhooks after delete: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1 after delete", len(list))
	}

	if _, err := s.GetWebhook(w.ID); err == nil {
		t.Fatal("expected error getting deleted webhook, got nil")
	}
}

func TestAlertRuleCRUD(t *testing.T) {
	s := newTestStore(t)
	wh := makeTestWebhook(t, s)

	r := &AlertRule{
		Metric:      "cpu_pct",
		Operator:    ">",
		Threshold:   90.0,
		DurationSec: 300,
		WebhookID:   wh.ID,
		Enabled:     true,
	}
	if err := s.CreateAlertRule(r); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	if r.ID == 0 {
		t.Fatal("expected ID to be set after create")
	}
	if r.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set after create")
	}

	rules, err := s.ListAlertRules(nil)
	if err != nil {
		t.Fatalf("ListAlertRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(rules))
	}
	if rules[0].Metric != "cpu_pct" {
		t.Errorf("Metric = %q, want cpu_pct", rules[0].Metric)
	}
	if !rules[0].Enabled {
		t.Error("Enabled = false, want true")
	}
	if rules[0].AppID != nil {
		t.Error("AppID should be nil for system-level rule")
	}

	// disable the rule
	r.Enabled = false
	if err := s.UpdateAlertRule(r); err != nil {
		t.Fatalf("UpdateAlertRule: %v", err)
	}
	rules, err = s.ListAlertRules(nil)
	if err != nil {
		t.Fatalf("ListAlertRules after update: %v", err)
	}
	if rules[0].Enabled {
		t.Error("Enabled = true after disable, want false")
	}

	if err := s.DeleteAlertRule(r.ID); err != nil {
		t.Fatalf("DeleteAlertRule: %v", err)
	}
	rules, err = s.ListAlertRules(nil)
	if err != nil {
		t.Fatalf("ListAlertRules after delete: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("len(rules) = %d, want 0 after delete", len(rules))
	}
}

func TestAlertHistoryCreateAndResolve(t *testing.T) {
	s := newTestStore(t)
	wh := makeTestWebhook(t, s)

	rule := &AlertRule{
		Metric:      "mem_bytes",
		Operator:    ">",
		Threshold:   1000000,
		DurationSec: 60,
		WebhookID:   wh.ID,
		Enabled:     true,
	}
	if err := s.CreateAlertRule(rule); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	h, err := s.CreateAlertHistory(rule.ID, 1500000)
	if err != nil {
		t.Fatalf("CreateAlertHistory: %v", err)
	}
	if h.ID == 0 {
		t.Fatal("expected ID to be set")
	}
	if h.FiredAt.IsZero() {
		t.Fatal("expected FiredAt to be set")
	}
	if h.ResolvedAt != nil {
		t.Fatal("expected ResolvedAt to be nil after create")
	}
	if h.Value != 1500000 {
		t.Errorf("Value = %v, want 1500000", h.Value)
	}

	if err := s.ResolveAlert(h.ID); err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	hist, err := s.ListAlertHistory(&rule.ID, 10)
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("len(hist) = %d, want 1", len(hist))
	}
	if hist[0].ResolvedAt == nil {
		t.Fatal("expected ResolvedAt to be set after resolve")
	}
}

func TestGetActiveAlert(t *testing.T) {
	s := newTestStore(t)
	wh := makeTestWebhook(t, s)

	rule := &AlertRule{
		Metric:      "cpu_pct",
		Operator:    ">",
		Threshold:   80,
		DurationSec: 120,
		WebhookID:   wh.ID,
		Enabled:     true,
	}
	if err := s.CreateAlertRule(rule); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// no active alert yet
	active, err := s.GetActiveAlert(rule.ID)
	if err != nil {
		t.Fatalf("GetActiveAlert (before create): %v", err)
	}
	if active != nil {
		t.Fatal("expected nil active alert before any history")
	}

	h, err := s.CreateAlertHistory(rule.ID, 95.0)
	if err != nil {
		t.Fatalf("CreateAlertHistory: %v", err)
	}

	active, err = s.GetActiveAlert(rule.ID)
	if err != nil {
		t.Fatalf("GetActiveAlert (after create): %v", err)
	}
	if active == nil {
		t.Fatal("expected active alert, got nil")
	}
	if active.ID != h.ID {
		t.Errorf("active.ID = %d, want %d", active.ID, h.ID)
	}

	if err := s.ResolveAlert(h.ID); err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	active, err = s.GetActiveAlert(rule.ID)
	if err != nil {
		t.Fatalf("GetActiveAlert (after resolve): %v", err)
	}
	if active != nil {
		t.Fatal("expected nil active alert after resolve")
	}
}

func TestListActiveAlertRules(t *testing.T) {
	s := newTestStore(t)
	wh := makeTestWebhook(t, s)

	enabled := &AlertRule{
		Metric:      "cpu_pct",
		Operator:    ">",
		Threshold:   80,
		DurationSec: 60,
		WebhookID:   wh.ID,
		Enabled:     true,
	}
	disabled := &AlertRule{
		Metric:      "mem_bytes",
		Operator:    ">",
		Threshold:   500000,
		DurationSec: 60,
		WebhookID:   wh.ID,
		Enabled:     false,
	}

	if err := s.CreateAlertRule(enabled); err != nil {
		t.Fatalf("CreateAlertRule enabled: %v", err)
	}
	if err := s.CreateAlertRule(disabled); err != nil {
		t.Fatalf("CreateAlertRule disabled: %v", err)
	}

	active, err := s.ListActiveAlertRules()
	if err != nil {
		t.Fatalf("ListActiveAlertRules: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("len(active) = %d, want 1", len(active))
	}
	if active[0].ID != enabled.ID {
		t.Errorf("active[0].ID = %d, want %d", active[0].ID, enabled.ID)
	}
	if !active[0].Enabled {
		t.Error("Enabled = false, want true")
	}
}
