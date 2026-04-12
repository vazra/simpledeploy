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
	if resp["ID"] == nil {
		t.Error("response missing ID")
	}
	if resp["Name"] != "slack-alerts" {
		t.Errorf("Name = %q, want slack-alerts", resp["Name"])
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
	id := int64(created["ID"].(float64))

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
		if int64(wh["ID"].(float64)) == id {
			t.Errorf("webhook %d still present after delete", id)
		}
	}
}

func TestCreateAlertRule(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	wh := createTestWebhook(t, srv, cookie, "mywh")
	whID := int64(wh["ID"].(float64))

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
	if resp["ID"] == nil {
		t.Error("response missing ID")
	}
}

func TestListAlertRules(t *testing.T) {
	srv, _, cookie := setupAlertTestServer(t)

	wh := createTestWebhook(t, srv, cookie, "mywh2")
	whID := int64(wh["ID"].(float64))

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
	id := int64(created["ID"].(float64))

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
	if resp["Name"] != "updated" {
		t.Errorf("Name = %q, want updated", resp["Name"])
	}
	if resp["Type"] != "discord" {
		t.Errorf("Type = %q, want discord", resp["Type"])
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
	id := int64(created["ID"].(float64))

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
	if hist[0]["RuleID"] == nil {
		t.Error("history entry missing RuleID")
	}
}
