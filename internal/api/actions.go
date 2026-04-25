package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	go func() {
		if err := s.reconciler.RestartOne(context.Background(), slug); err != nil {
			fmt.Fprintf(os.Stderr, "restart %s: %v\n", slug, err)
		}
	}()
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "status": "restarting"})
	_, _ = s.audit.Record(ctx, audit.RecordReq{
		Category: "lifecycle",
		Action:   "restarted",
		AppSlug:  slug,
		After:    afterJSON,
	})
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
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "status": "stopped"})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "stopped",
		AppSlug:  slug,
		After:    afterJSON,
	})
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
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "status": "running"})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "started",
		AppSlug:  slug,
		After:    afterJSON,
	})
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
	afterJSON, _ := json.Marshal(map[string]any{"name": slug})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "image_pulled",
		AppSlug:  slug,
		After:    afterJSON,
	})
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

	// Capture before/after replica counts for audit.
	// beforeReplicas: sum of current values in the incoming scale map (not live state,
	// since ServiceStatus has no Replicas field). We use 0 as unknown before-state.
	var afterReplicas int
	for _, v := range body.Scales {
		afterReplicas += v
	}

	if err := s.reconciler.ScaleOne(r.Context(), slug, body.Scales); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	beforeJSON, _ := json.Marshal(map[string]any{"name": slug, "replicas": 0})
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "replicas": afterReplicas})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "scaled",
		AppSlug:  slug,
		Before:   beforeJSON,
		After:    afterJSON,
	})

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
		httpError(w, err, http.StatusInternalServerError)
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
		httpError(w, err, http.StatusInternalServerError)
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
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if versions == nil {
		versions = []store.ComposeVersion{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid version id", http.StatusBadRequest)
		return
	}
	// Capture version number + app for audit before deletion.
	var beforeJSON []byte
	var appID *int64
	var appSlug string
	if v, err := s.store.GetComposeVersion(id); err == nil && v != nil {
		beforeJSON, _ = json.Marshal(map[string]any{"version": v.Version})
		if a, err := s.store.GetAppByID(v.AppID); err == nil && a != nil {
			aid := a.ID
			appID = &aid
			appSlug = a.Slug
		}
	}
	if err := s.store.DeleteComposeVersion(id); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "compose",
		Action:   "version_removed",
		AppID:    appID,
		AppSlug:  appSlug,
		Before:   beforeJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListDeployEvents(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	events, err := s.reconciler.ListDeployEvents(r.Context(), slug)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []store.DeployEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
