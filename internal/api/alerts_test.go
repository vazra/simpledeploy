package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func setupAlertTestServer(t *testing.T) (*Server, *store.Store, *http.Cookie) {
	t.Helper()
	srv, st := setupAuthTestServer(t)
	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": "admin",
		"password": "password123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d", w.Code)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	return srv, st, cookie
}

func createTestWebhook(t *testing.T, srv *Server, cookie *http.Cookie, name string) map[string]any {
	t.Helper()
	req := authedRequest(t, http.MethodPost, "/api/webhooks", map[string]string{
		"name": name,
		"type": "slack",
		"url":  "https://hooks.slack.com/test",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create webhook status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func createTestRule(t *testing.T, srv *Server, cookie *http.Cookie, whID int64) map[string]any {
	t.Helper()
	req := authedRequest(t, http.MethodPost, "/api/alerts/rules", map[string]any{
		"metric":       "cpu_pct",
		"operator":     ">",
		"threshold":    80.0,
		"duration_sec": 300,
		"webhook_id":   whID,
		"enabled":      true,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rule status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestCreateWebhook(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	req := authedRequest(t, http.MethodPost, "/api/webhooks", map[string]string{
		"name": "slack-alerts",
		"type": "slack",
		"url":  "https://hooks.slack.com/services/test",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == nil {
		t.Error("response missing id")
	}
	if resp["name"] != "slack-alerts" {
		t.Errorf("name = %q, want slack-alerts", resp["name"])
	}
}

func TestListWebhooks(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	createTestWebhook(t, srv, cookie, "wh1")
	createTestWebhook(t, srv, cookie, "wh2")

	req := authedRequest(t, http.MethodGet, "/api/webhooks", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var whs []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&whs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(whs) < 2 {
		t.Errorf("got %d webhooks, want >= 2", len(whs))
	}
}

func TestDeleteWebhook(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	created := createTestWebhook(t, srv, cookie, "to-delete")
	id := int64(created["id"].(float64))

	req := authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/webhooks/%d", id), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// verify gone
	req = authedRequest(t, http.MethodGet, "/api/webhooks", nil, cookie)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var whs []map[string]any
	json.NewDecoder(w.Body).Decode(&whs)
	for _, wh := range whs {
		if int64(wh["id"].(float64)) == id {
			t.Errorf("webhook %d still present after delete", id)
		}
	}
}

func TestCreateAlertRule(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	wh := createTestWebhook(t, srv, cookie, "mywh")
	whID := int64(wh["id"].(float64))

	req := authedRequest(t, http.MethodPost, "/api/alerts/rules", map[string]any{
		"metric":       "cpu_pct",
		"operator":     ">",
		"threshold":    80.0,
		"duration_sec": 300,
		"webhook_id":   whID,
		"enabled":      true,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == nil {
		t.Error("response missing id")
	}
}

func TestListAlertRules(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	wh := createTestWebhook(t, srv, cookie, "mywh2")
	whID := int64(wh["id"].(float64))

	for i := 0; i < 2; i++ {
		req := authedRequest(t, http.MethodPost, "/api/alerts/rules", map[string]any{
			"metric":       "cpu_pct",
			"operator":     ">",
			"threshold":    float64(70 + i),
			"duration_sec": 60,
			"webhook_id":   whID,
			"enabled":      true,
		}, cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create rule %d status = %d", i, w.Code)
		}
	}

	req := authedRequest(t, http.MethodGet, "/api/alerts/rules", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var rules []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&rules); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rules) < 2 {
		t.Errorf("got %d rules, want >= 2", len(rules))
	}
}

func TestUpdateWebhookAPI(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	created := createTestWebhook(t, srv, cookie, "original")
	id := int64(created["id"].(float64))

	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/webhooks/%d", id), map[string]string{
		"name": "updated",
		"type": "discord",
		"url":  "https://discord.com/api/webhooks/456",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["name"] != "updated" {
		t.Errorf("name = %q, want updated", resp["name"])
	}
	if resp["type"] != "discord" {
		t.Errorf("type = %q, want discord", resp["type"])
	}
}

func TestUpdateWebhookNotFound(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	req := authedRequest(t, http.MethodPut, "/api/webhooks/9999", map[string]string{
		"name": "x",
		"type": "slack",
		"url":  "https://example.com",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestTestWebhookByID(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	// need dispatcher set
	srv.webhookDispatcher = nil // no dispatcher - should fail gracefully

	created := createTestWebhook(t, srv, cookie, "test-wh")
	id := int64(created["id"].(float64))

	req := authedRequest(t, http.MethodPost, "/api/webhooks/test", map[string]any{
		"webhook_id": id,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// without dispatcher, should get 500
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (no dispatcher); body: %s", w.Code, w.Body.String())
	}
}

func TestTestWebhookValidation(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	// no webhook_id and no type+url
	req := authedRequest(t, http.MethodPost, "/api/webhooks/test", map[string]any{}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestListAlertHistory(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)

	// create webhook + rule directly via store
	wh := &store.Webhook{Name: "hw", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rule := &store.AlertRule{
		Metric:      "cpu_pct",
		Operator:    ">",
		Threshold:   90,
		DurationSec: 60,
		WebhookID:   wh.ID,
		Enabled:     true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := st.CreateAlertHistory(rule.ID, 95.5, rule); err != nil {
		t.Fatalf("create history: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/alerts/history?rule_id=%d", rule.ID), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var hist []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&hist); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(hist) != 1 {
		t.Errorf("got %d history entries, want 1", len(hist))
	}
	if hist[0]["rule_id"] == nil {
		t.Error("history entry missing rule_id")
	}
}

// --- Edge case tests ---

func TestCreateRuleInvalidWebhook(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	req := authedRequest(t, http.MethodPost, "/api/alerts/rules", map[string]any{
		"metric":       "cpu_pct",
		"operator":     ">",
		"threshold":    80.0,
		"duration_sec": 300,
		"webhook_id":   99999,
		"enabled":      true,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteWebhookReferencedByRule(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	wh := createTestWebhook(t, srv, cookie, "ref-wh")
	whID := int64(wh["id"].(float64))
	createTestRule(t, srv, cookie, whID)

	// attempt delete webhook that's referenced by a rule
	req := authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/webhooks/%d", whID), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// current behavior: SQLite foreign key or just deletes. Record what happens.
	// If it succeeds (200), webhook is deleted despite rule reference.
	// If it fails, it should be an error.
	t.Logf("delete referenced webhook: status=%d body=%s", w.Code, w.Body.String())

	// At minimum, verify the response is a valid HTTP status
	if w.Code != http.StatusOK && w.Code != http.StatusConflict && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status = %d", w.Code)
	}
}

func TestCreateRuleMissingFields(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing metric", map[string]any{"operator": ">", "webhook_id": 1}},
		{"missing operator", map[string]any{"metric": "cpu_pct", "webhook_id": 1}},
		{"missing webhook_id", map[string]any{"metric": "cpu_pct", "operator": ">"}},
		{"empty body", map[string]any{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := authedRequest(t, http.MethodPost, "/api/alerts/rules", tc.body, cookie)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestUpdateAlertRuleAPI(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	wh := createTestWebhook(t, srv, cookie, "upd-wh")
	whID := int64(wh["id"].(float64))
	rule := createTestRule(t, srv, cookie, whID)
	ruleID := int64(rule["id"].(float64))

	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/alerts/rules/%d", ruleID), map[string]any{
		"metric":       "mem_pct",
		"operator":     ">=",
		"threshold":    95.0,
		"duration_sec": 600,
		"webhook_id":   whID,
		"enabled":      true,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["metric"] != "mem_pct" {
		t.Errorf("metric = %q, want mem_pct", resp["metric"])
	}
	if resp["operator"] != ">=" {
		t.Errorf("operator = %q, want >=", resp["operator"])
	}
	if resp["threshold"].(float64) != 95.0 {
		t.Errorf("threshold = %v, want 95", resp["threshold"])
	}
}

func TestUpdateAlertRuleNotFound(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	wh := createTestWebhook(t, srv, cookie, "nf-wh")
	whID := int64(wh["id"].(float64))

	req := authedRequest(t, http.MethodPut, "/api/alerts/rules/99999", map[string]any{
		"metric":       "cpu_pct",
		"operator":     ">",
		"threshold":    80.0,
		"duration_sec": 300,
		"webhook_id":   whID,
		"enabled":      true,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteRuleResolvesActiveAlerts(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)

	wh := &store.Webhook{Name: "dr-wh", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rule := &store.AlertRule{
		Metric: "cpu_pct", Operator: ">", Threshold: 90,
		DurationSec: 60, WebhookID: wh.ID, Enabled: true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	// fire an alert
	if _, err := st.CreateAlertHistory(rule.ID, 95.0, rule); err != nil {
		t.Fatalf("create history: %v", err)
	}
	// verify active
	active, err := st.GetActiveAlert(rule.ID)
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active == nil {
		t.Fatal("expected active alert before delete")
	}

	// delete rule via API
	req := authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/alerts/rules/%d", rule.ID), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// verify alert resolved
	hist, err := st.ListAlertHistory(&rule.ID, 10)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	for _, h := range hist {
		if h.ResolvedAt == nil {
			t.Error("alert should be resolved after rule delete")
		}
	}
}

func TestDisableRuleResolvesActiveAlerts(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)

	wh := &store.Webhook{Name: "dis-wh", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rule := &store.AlertRule{
		Metric: "cpu_pct", Operator: ">", Threshold: 90,
		DurationSec: 60, WebhookID: wh.ID, Enabled: true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := st.CreateAlertHistory(rule.ID, 95.0, rule); err != nil {
		t.Fatalf("create history: %v", err)
	}

	// disable via API update
	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/alerts/rules/%d", rule.ID), map[string]any{
		"metric":       rule.Metric,
		"operator":     rule.Operator,
		"threshold":    rule.Threshold,
		"duration_sec": rule.DurationSec,
		"webhook_id":   rule.WebhookID,
		"enabled":      false,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// verify alert resolved
	active, err := st.GetActiveAlert(rule.ID)
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active != nil {
		t.Error("expected no active alert after disabling rule")
	}
}

func TestAlertHistorySnapshotFields(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)

	wh := &store.Webhook{Name: "snap-wh", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rule := &store.AlertRule{
		Metric: "mem_pct", Operator: ">=", Threshold: 85,
		DurationSec: 120, WebhookID: wh.ID, Enabled: true,
		AppSlug: "myapp",
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := st.CreateAlertHistory(rule.ID, 90.0, rule); err != nil {
		t.Fatalf("create history: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/alerts/history?rule_id=%d", rule.ID), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	var hist []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&hist); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("got %d entries, want 1", len(hist))
	}
	h := hist[0]
	if h["metric"] != "mem_pct" {
		t.Errorf("metric = %v, want mem_pct", h["metric"])
	}
	if h["app_slug"] != "myapp" {
		t.Errorf("app_slug = %v, want myapp", h["app_slug"])
	}
	if h["operator"] != ">=" {
		t.Errorf("operator = %v, want >=", h["operator"])
	}
	if h["threshold"].(float64) != 85.0 {
		t.Errorf("threshold = %v, want 85", h["threshold"])
	}
	if h["value"].(float64) != 90.0 {
		t.Errorf("value = %v, want 90", h["value"])
	}
}

func TestClearAlertHistoryResolved(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)

	wh := &store.Webhook{Name: "clr-wh", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rule := &store.AlertRule{
		Metric: "cpu_pct", Operator: ">", Threshold: 90,
		DurationSec: 60, WebhookID: wh.ID, Enabled: true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	// create resolved alert
	h1, err := st.CreateAlertHistory(rule.ID, 95.0, rule)
	if err != nil {
		t.Fatalf("create history: %v", err)
	}
	if err := st.ResolveAlert(h1.ID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// create active alert
	if _, err := st.CreateAlertHistory(rule.ID, 92.0, rule); err != nil {
		t.Fatalf("create history 2: %v", err)
	}

	// clear resolved only (default)
	req := authedRequest(t, http.MethodDelete, "/api/alerts/history", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	// active alert should remain
	hist, err := st.ListAlertHistory(&rule.ID, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(hist) != 1 {
		t.Errorf("got %d entries, want 1 (active only)", len(hist))
	}
	if hist[0].ResolvedAt != nil {
		t.Error("remaining entry should be unresolved")
	}
}

func TestClearAlertHistoryAll(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)

	wh := &store.Webhook{Name: "clra-wh", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rule := &store.AlertRule{
		Metric: "cpu_pct", Operator: ">", Threshold: 90,
		DurationSec: 60, WebhookID: wh.ID, Enabled: true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	h1, err := st.CreateAlertHistory(rule.ID, 95.0, rule)
	if err != nil {
		t.Fatalf("create history: %v", err)
	}
	if err := st.ResolveAlert(h1.ID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := st.CreateAlertHistory(rule.ID, 92.0, rule); err != nil {
		t.Fatalf("create history 2: %v", err)
	}

	// clear all
	req := authedRequest(t, http.MethodDelete, "/api/alerts/history?mode=all", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	hist, err := st.ListAlertHistory(nil, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("got %d entries, want 0", len(hist))
	}
}

func TestEditRuleAfterFired(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)

	wh := &store.Webhook{Name: "edit-wh", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	rule := &store.AlertRule{
		Metric: "cpu_pct", Operator: ">", Threshold: 80,
		DurationSec: 60, WebhookID: wh.ID, Enabled: true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	// fire alert with old values
	if _, err := st.CreateAlertHistory(rule.ID, 85.0, rule); err != nil {
		t.Fatalf("create history: %v", err)
	}

	// update rule via API (change threshold)
	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/alerts/rules/%d", rule.ID), map[string]any{
		"metric":       "cpu_pct",
		"operator":     ">",
		"threshold":    95.0,
		"duration_sec": 120,
		"webhook_id":   wh.ID,
		"enabled":      true,
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d; body: %s", w.Code, w.Body.String())
	}

	// old alert history should still have old threshold (snapshot)
	hist, err := st.ListAlertHistory(&rule.ID, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(hist) < 1 {
		t.Fatal("expected at least 1 history entry")
	}
	if hist[0].Threshold != 80.0 {
		t.Errorf("old alert threshold = %v, want 80 (snapshot preserved)", hist[0].Threshold)
	}

	// fire new alert with new rule values
	updatedRule := &store.AlertRule{
		ID: rule.ID, Metric: "cpu_pct", Operator: ">", Threshold: 95,
		DurationSec: 120, WebhookID: wh.ID, Enabled: true,
	}
	if _, err := st.CreateAlertHistory(rule.ID, 97.0, updatedRule); err != nil {
		t.Fatalf("create history 2: %v", err)
	}

	hist, err = st.ListAlertHistory(&rule.ID, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(hist) < 2 {
		t.Fatal("expected at least 2 history entries")
	}
	// find entries by value to avoid ordering ambiguity
	for _, h := range hist {
		if h.Value == 85.0 && h.Threshold != 80.0 {
			t.Errorf("old alert (value=85) threshold = %v, want 80 (snapshot)", h.Threshold)
		}
		if h.Value == 97.0 && h.Threshold != 95.0 {
			t.Errorf("new alert (value=97) threshold = %v, want 95", h.Threshold)
		}
	}
}
