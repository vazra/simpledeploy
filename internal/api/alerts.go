package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/vazra/simpledeploy/internal/alerts"
	"github.com/vazra/simpledeploy/internal/audit"
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

// webhookAuditJSON builds the webhookView JSON shape for audit records.
func webhookAuditJSON(name, url, whType string) []byte {
	b, _ := json.Marshal(map[string]any{
		"name":   name,
		"url":    url,
		"events": []string{whType},
	})
	return b
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
	if u, err := url.Parse(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		http.Error(w, "url must use http or https scheme", http.StatusBadRequest)
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
	afterJSON := webhookAuditJSON(wh.Name, wh.URL, wh.Type)
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "webhook",
		Action:   "added",
		After:    afterJSON,
	})
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
	existing, _ := s.store.GetWebhook(id)
	if err := s.store.DeleteWebhook(id); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	if existing != nil {
		beforeJSON := webhookAuditJSON(existing.Name, existing.URL, existing.Type)
		_, _ = s.audit.Record(r.Context(), audit.RecordReq{
			Category: "webhook",
			Action:   "removed",
			Before:   beforeJSON,
		})
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
	existing, _ := s.store.GetWebhook(id)
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
	if u, err := url.Parse(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		http.Error(w, "url must use http or https scheme", http.StatusBadRequest)
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
	var beforeJSON []byte
	if existing != nil {
		beforeJSON = webhookAuditJSON(existing.Name, existing.URL, existing.Type)
	}
	afterJSON := webhookAuditJSON(wh.Name, wh.URL, wh.Type)
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "webhook",
		Action:   "changed",
		Before:   beforeJSON,
		After:    afterJSON,
	})
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
	alerts.EnrichEvent(&event)

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
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var appID *int64
	if v := r.URL.Query().Get("app_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			http.Error(w, "invalid app_id", http.StatusBadRequest)
			return
		}
		appID = &id
	}
	if appID != nil && user.Role != "super_admin" {
		ok, _ := s.store.HasAppAccessByID(user.ID, *appID)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	rules, err := s.store.ListAlertRules(appID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []store.AlertRule{}
	}
	if user.Role != "super_admin" {
		filtered := rules[:0]
		for _, rule := range rules {
			if rule.AppID == nil {
				continue
			}
			ok, _ := s.store.HasAppAccessByID(user.ID, *rule.AppID)
			if ok {
				filtered = append(filtered, rule)
			}
		}
		rules = filtered
	}
	for i, r := range rules {
		if r.AppID != nil {
			if app, err := s.store.GetAppByID(*r.AppID); err == nil {
				rules[i].AppSlug = app.Slug
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

// alertRuleAuditJSON builds the alertView JSON shape for audit records.
// The alertView.Name field is synthesised from metric+operator+threshold.
func alertRuleAuditJSON(r *store.AlertRule) []byte {
	name := fmt.Sprintf("%s %s %.4g", r.Metric, r.Operator, r.Threshold)
	b, _ := json.Marshal(map[string]any{
		"name":      name,
		"metric":    r.Metric,
		"threshold": r.Threshold,
	})
	return b
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
	if !s.canMutateForApp(w, r, req.AppID) {
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
	var appID *int64
	if req.AppID != nil {
		appID = req.AppID
	}
	afterJSON := alertRuleAuditJSON(rule)
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "alert",
		Action:   "added",
		AppID:    appID,
		AppSlug:  rule.AppSlug,
		After:    afterJSON,
	})
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
	// Load existing rule for before-snapshot.
	var existing *store.AlertRule
	if rules, err := s.store.ListAlertRules(nil); err == nil {
		for i := range rules {
			if rules[i].ID == id {
				r2 := rules[i]
				existing = &r2
				break
			}
		}
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
	// Authorise against both the existing rule's app (so a manage user
	// cannot reach across apps) and the requested target app.
	if existing != nil && !s.canMutateForApp(w, r, existing.AppID) {
		return
	}
	if !s.canMutateForApp(w, r, req.AppID) {
		return
	}
	var appSlug string
	if req.AppID != nil {
		if app, err := s.store.GetAppByID(*req.AppID); err == nil {
			appSlug = app.Slug
		}
	}
	rule := &store.AlertRule{
		ID:          id,
		AppID:       req.AppID,
		AppSlug:     appSlug,
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
	var beforeJSON []byte
	if existing != nil {
		beforeJSON = alertRuleAuditJSON(existing)
	}
	afterJSON := alertRuleAuditJSON(rule)
	var appID *int64
	if req.AppID != nil {
		appID = req.AppID
	}
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "alert",
		Action:   "changed",
		AppID:    appID,
		AppSlug:  appSlug,
		Before:   beforeJSON,
		After:    afterJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

func (s *Server) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	// Load existing rule for before-snapshot.
	var existing *store.AlertRule
	if rules, err := s.store.ListAlertRules(nil); err == nil {
		for i := range rules {
			if rules[i].ID == id {
				r2 := rules[i]
				existing = &r2
				break
			}
		}
	}
	if existing != nil && !s.canMutateForApp(w, r, existing.AppID) {
		return
	}
	if existing == nil {
		// Fall through to DeleteAlertRule which will 404; still require auth role for safety.
		user := GetAuthUser(r)
		if user == nil || (user.Role != "super_admin" && user.Role != "manage") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	if err := s.store.DeleteAlertRule(id); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	if existing != nil {
		beforeJSON := alertRuleAuditJSON(existing)
		var appID *int64
		if existing.AppID != nil {
			appID = existing.AppID
		}
		_, _ = s.audit.Record(r.Context(), audit.RecordReq{
			Category: "alert",
			Action:   "removed",
			AppID:    appID,
			AppSlug:  existing.AppSlug,
			Before:   beforeJSON,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Alert History ---

func (s *Server) handleListAlertHistory(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
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
	// Filter to apps the caller has access to (super_admin sees everything).
	if user.Role != "super_admin" {
		filtered := hist[:0]
		accessCache := map[string]bool{}
		for _, h := range hist {
			if h.AppSlug == "" {
				// global rows visible only to super_admin
				continue
			}
			ok, present := accessCache[h.AppSlug]
			if !present {
				granted, _ := s.store.HasAppAccess(user.ID, h.AppSlug)
				accessCache[h.AppSlug] = granted
				ok = granted
			}
			if ok {
				filtered = append(filtered, h)
			}
		}
		hist = filtered
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hist)
}

func (s *Server) handleClearAlertHistory(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	resolvedOnly := mode != "all"
	if err := s.store.ClearAlertHistory(resolvedOnly); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
