package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	go func() {
		if err := s.reconciler.RestartOne(context.Background(), slug); err != nil {
			fmt.Fprintf(os.Stderr, "restart %s: %v\n", slug, err)
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
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
	go func() {
		if err := s.reconciler.PullOne(context.Background(), slug); err != nil {
			fmt.Fprintf(os.Stderr, "pull %s: %v\n", slug, err)
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
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

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	var body struct {
		VersionID int64 `json:"version_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.VersionID == 0 {
		http.Error(w, "version_id is required", http.StatusBadRequest)
		return
	}
	if err := s.reconciler.RollbackOne(r.Context(), slug, body.VersionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	versions, err := s.reconciler.ListVersions(r.Context(), slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if versions == nil {
		versions = []store.ComposeVersion{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

func (s *Server) handleListDeployEvents(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	events, err := s.reconciler.ListDeployEvents(r.Context(), slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []store.DeployEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
