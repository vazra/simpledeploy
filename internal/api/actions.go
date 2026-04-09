package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/deployer"
)

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	if err := s.reconciler.RestartOne(r.Context(), slug); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	if err := s.reconciler.StopOne(r.Context(), slug); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	if err := s.reconciler.StartOne(r.Context(), slug); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	if err := s.reconciler.PullOne(r.Context(), slug); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	var body struct {
		Scales map[string]int `json:"scales"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body.Scales) == 0 {
		http.Error(w, "scales is required", http.StatusBadRequest)
		return
	}
	if err := s.reconciler.ScaleOne(r.Context(), slug, body.Scales); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGetServices(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	services, err := s.reconciler.AppServices(r.Context(), slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if services == nil {
		services = []deployer.ServiceStatus{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}
