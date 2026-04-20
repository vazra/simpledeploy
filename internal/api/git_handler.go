package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	if s.gs == nil {
		http.Error(w, "git sync not enabled", http.StatusServiceUnavailable)
		return
	}
	status := s.gs.Status()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleGitWebhook(w http.ResponseWriter, r *http.Request) {
	if s.gs == nil {
		http.Error(w, "git sync not enabled", http.StatusServiceUnavailable)
		return
	}
	h := s.gs.WebhookHandler()
	if h == nil {
		http.Error(w, "git sync webhook not configured", http.StatusServiceUnavailable)
		return
	}
	h.ServeHTTP(w, r)
}
