package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/compose"
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

func (s *Server) handleBackupSummary(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.GetBackupSummary()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if apps == nil {
		apps = []store.BackupSummaryApp{}
	}

	runs, err := s.store.ListRecentBackupRuns(20)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []store.BackupRunWithApp{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"apps":        apps,
		"recent_runs": runs,
	})
}

type strategyInfo struct {
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Available   bool     `json:"available"`
	Containers  []string `json:"containers,omitempty"`
	Volumes     []string `json:"volumes,omitempty"`
	Description string   `json:"description"`
}

func (s *Server) handleDetectStrategies(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	cfg, parseErr := compose.ParseFile(app.ComposePath, app.Name)

	var postgresStrategy, volumeStrategy strategyInfo

	if parseErr != nil {
		postgresStrategy = strategyInfo{
			Type:        "postgres",
			Label:       "Database (PostgreSQL)",
			Available:   false,
			Containers:  []string{},
			Description: "No compose file available",
		}
		volumeStrategy = strategyInfo{
			Type:        "volume",
			Label:       "Files & Volumes",
			Available:   false,
			Volumes:     []string{},
			Description: "No compose file available",
		}
	} else {
		var pgContainers []string
		var volumeMounts []string

		for _, svc := range cfg.Services {
			if strings.Contains(strings.ToLower(svc.Image), "postgres") {
				pgContainers = append(pgContainers, fmt.Sprintf("%s-%s-1", app.Name, svc.Name))
			}
			for _, vol := range svc.Volumes {
				if vol.Target != "/var/run/docker.sock" {
					volumeMounts = append(volumeMounts, vol.Target)
				}
			}
		}
		if pgContainers == nil {
			pgContainers = []string{}
		}
		if volumeMounts == nil {
			volumeMounts = []string{}
		}

		pgDesc := "Backs up PostgreSQL databases using pg_dump."
		if len(pgContainers) == 0 {
			pgDesc = "No PostgreSQL services detected in compose file."
		}
		volDesc := "Backs up named volumes and bind-mounted directories."
		if len(volumeMounts) == 0 {
			volDesc = "No volume mounts detected in compose file."
		}

		postgresStrategy = strategyInfo{
			Type:        "postgres",
			Label:       "Database (PostgreSQL)",
			Available:   len(pgContainers) > 0,
			Containers:  pgContainers,
			Description: pgDesc,
		}
		volumeStrategy = strategyInfo{
			Type:        "volume",
			Label:       "Files & Volumes",
			Available:   len(volumeMounts) > 0,
			Volumes:     volumeMounts,
			Description: volDesc,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"strategies": []strategyInfo{postgresStrategy, volumeStrategy},
	})
}

func (s *Server) handleTriggerBackupConfig(w http.ResponseWriter, r *http.Request) {
	if s.backupScheduler == nil {
		http.Error(w, "backup scheduler not configured", http.StatusServiceUnavailable)
		return
	}

	idStr := r.PathValue("id")
	cfgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if _, err := s.store.GetBackupConfig(cfgID); err != nil {
		http.Error(w, "backup config not found", http.StatusNotFound)
		return
	}

	go func() {
		if err := s.backupScheduler.RunBackup(context.Background(), cfgID); err != nil {
			// log only; response already sent
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleTestS3(w http.ResponseWriter, r *http.Request) {
	var cfg backup.S3Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	target, err := backup.NewS3Target(cfg)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	testKey := ".simpledeploy-s3-test"
	testData := []byte("simpledeploy s3 connectivity test")

	_, _, uploadErr := target.Upload(r.Context(), testKey, bytes.NewReader(testData))
	if uploadErr != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": uploadErr.Error()})
		return
	}

	_ = target.Delete(r.Context(), testKey)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}
