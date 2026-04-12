package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/vazra/simpledeploy/internal/alerts"
	"github.com/vazra/simpledeploy/internal/store"
)

// --- Webhooks ---

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	whs, err := s.store.ListWebhooks()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if whs == nil {
		whs = []store.Webhook{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(whs)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		Type             string `json:"type"`
		URL              string `json:"url"`
		TemplateOverride string `json:"template_override"`
		HeadersJSON      string `json:"headers_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Type == "" || req.URL == "" {
		http.Error(w, "name, type and url required", http.StatusBadRequest)
		return
	}
	wh := &store.Webhook{
		Name:             req.Name,
		Type:             req.Type,
		URL:              req.URL,
		TemplateOverride: req.TemplateOverride,
		HeadersJSON:      req.HeadersJSON,
	}
	if err := s.store.CreateWebhook(wh); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wh)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteWebhook(id); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Name             string `json:"name"`
		Type             string `json:"type"`
		URL              string `json:"url"`
		TemplateOverride string `json:"template_override"`
		HeadersJSON      string `json:"headers_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Type == "" || req.URL == "" {
		http.Error(w, "name, type and url required", http.StatusBadRequest)
		return
	}
	wh := &store.Webhook{
		ID:               id,
		Name:             req.Name,
		Type:             req.Type,
		URL:              req.URL,
		TemplateOverride: req.TemplateOverride,
		HeadersJSON:      req.HeadersJSON,
	}
	if err := s.store.UpdateWebhook(wh); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wh)
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WebhookID        *int64 `json:"webhook_id"`
		Type             string `json:"type"`
		URL              string `json:"url"`
		TemplateOverride string `json:"template_override"`
		HeadersJSON      string `json:"headers_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	var wh store.Webhook
	if req.WebhookID != nil {
		loaded, err := s.store.GetWebhook(*req.WebhookID)
		if err != nil {
			httpError(w, err, http.StatusNotFound)
			return
		}
		wh = *loaded
	} else {
		if req.Type == "" || req.URL == "" {
			http.Error(w, "webhook_id or type+url required", http.StatusBadRequest)
			return
		}
		wh = store.Webhook{
			Type:             req.Type,
			URL:              req.URL,
			TemplateOverride: req.TemplateOverride,
			HeadersJSON:      req.HeadersJSON,
		}
	}

	event := alerts.AlertEvent{
		AppName:   "my-app",
		AppSlug:   "my-app",
		Metric:    "cpu_pct",
		Value:     92.5,
		Threshold: 80,
		Operator:  ">",
		Status:    "firing",
		FiredAt:   time.Now(),
	}

	if s.webhookDispatcher == nil {
		http.Error(w, "webhook dispatcher not configured", http.StatusInternalServerError)
		return
	}
	if err := s.webhookDispatcher.Send(wh, event); err != nil {
		http.Error(w, fmt.Sprintf("test failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Alert Rules ---

func (s *Server) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	var appID *int64
	if v := r.URL.Query().Get("app_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			http.Error(w, "invalid app_id", http.StatusBadRequest)
			return
		}
		appID = &id
	}
	rules, err := s.store.ListAlertRules(appID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []store.AlertRule{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

func (s *Server) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppID       *int64  `json:"app_id"`
		Metric      string  `json:"metric"`
		Operator    string  `json:"operator"`
		Threshold   float64 `json:"threshold"`
		DurationSec int     `json:"duration_sec"`
		WebhookID   int64   `json:"webhook_id"`
		Enabled     bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Metric == "" || req.Operator == "" || req.WebhookID == 0 {
		http.Error(w, "metric, operator and webhook_id required", http.StatusBadRequest)
		return
	}
	// verify webhook exists
	if _, err := s.store.GetWebhook(req.WebhookID); err != nil {
		http.Error(w, "webhook not found", http.StatusBadRequest)
		return
	}
	rule := &store.AlertRule{
		AppID:       req.AppID,
		Metric:      req.Metric,
		Operator:    req.Operator,
		Threshold:   req.Threshold,
		DurationSec: req.DurationSec,
		WebhookID:   req.WebhookID,
		Enabled:     req.Enabled,
	}
	if err := s.store.CreateAlertRule(rule); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

func (s *Server) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		AppID       *int64  `json:"app_id"`
		Metric      string  `json:"metric"`
		Operator    string  `json:"operator"`
		Threshold   float64 `json:"threshold"`
		DurationSec int     `json:"duration_sec"`
		WebhookID   int64   `json:"webhook_id"`
		Enabled     bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	rule := &store.AlertRule{
		ID:          id,
		AppID:       req.AppID,
		Metric:      req.Metric,
		Operator:    req.Operator,
		Threshold:   req.Threshold,
		DurationSec: req.DurationSec,
		WebhookID:   req.WebhookID,
		Enabled:     req.Enabled,
	}
	if err := s.store.UpdateAlertRule(rule); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

func (s *Server) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteAlertRule(id); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Alert History ---

func (s *Server) handleListAlertHistory(w http.ResponseWriter, r *http.Request) {
	var ruleID *int64
	if v := r.URL.Query().Get("rule_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			http.Error(w, "invalid rule_id", http.StatusBadRequest)
			return
		}
		ruleID = &id
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			limit = n
		}
	}
	hist, err := s.store.ListAlertHistory(ruleID, limit)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if hist == nil {
		hist = []store.AlertHistory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hist)
}
