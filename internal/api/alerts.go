package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/vazra/simpledeploy/internal/store"
)

// --- Webhooks ---

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	whs, err := s.store.ListWebhooks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusNotFound)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusNotFound)
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
		http.Error(w, err.Error(), http.StatusNotFound)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hist == nil {
		hist = []store.AlertHistory{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hist)
}
