package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleGitSyncNow(w http.ResponseWriter, r *http.Request) {
	if s.gs == nil {
		http.Error(w, "git sync not enabled", http.StatusServiceUnavailable)
		return
	}
	if err := s.gs.SyncNow(r.Context()); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

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

func (s *Server) handleGitApplyPending(w http.ResponseWriter, r *http.Request) {
	if s.gs == nil {
		http.Error(w, "git sync not enabled", http.StatusServiceUnavailable)
		return
	}
	if err := s.gs.ApplyPending(r.Context()); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	status := s.gs.Status()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleGitApplyPendingSafe(w http.ResponseWriter, r *http.Request) {
	gsMu.RLock()
	defer gsMu.RUnlock()
	s.handleGitApplyPending(w, r)
}
