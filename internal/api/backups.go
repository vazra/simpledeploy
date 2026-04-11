package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/vazra/simpledeploy/internal/store"
)

func (s *Server) handleListBackupConfigs(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	appID := app.ID
	cfgs, err := s.store.ListBackupConfigs(&appID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if cfgs == nil {
		cfgs = []store.BackupConfig{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfgs)
}

func (s *Server) handleCreateBackupConfig(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var cfg store.BackupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	cfg.AppID = app.ID

	if err := s.store.CreateBackupConfig(&cfg); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cfg)
}

func (s *Server) handleDeleteBackupConfig(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteBackupConfig(id); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListBackupRuns(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	// find the first backup config for this app
	appID := app.ID
	cfgs, err := s.store.ListBackupConfigs(&appID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	// collect runs for all configs belonging to this app
	var allRuns []store.BackupRun
	for _, cfg := range cfgs {
		runs, err := s.store.ListBackupRuns(cfg.ID)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		allRuns = append(allRuns, runs...)
	}

	if allRuns == nil {
		allRuns = []store.BackupRun{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allRuns)
}

func (s *Server) handleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupScheduler == nil {
		http.Error(w, "backup scheduler not configured", http.StatusServiceUnavailable)
		return
	}

	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	appID := app.ID
	cfgs, err := s.store.ListBackupConfigs(&appID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if len(cfgs) == 0 {
		http.Error(w, "no backup config for app", http.StatusNotFound)
		return
	}

	cfgID := cfgs[0].ID
	go func() {
		if err := s.backupScheduler.RunBackup(r.Context(), cfgID); err != nil {
			// log only; response already sent
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if s.backupScheduler == nil {
		http.Error(w, "backup scheduler not configured", http.StatusServiceUnavailable)
		return
	}

	idStr := r.PathValue("id")
	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	go func() {
		if err := s.backupScheduler.RunRestore(r.Context(), runID); err != nil {
			// log only; response already sent
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}
