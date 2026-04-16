package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
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

	// Encrypt S3 target config if present
	if cfg.Target == "s3" && cfg.TargetConfigJSON != "" && s.masterSecret != "" {
		encrypted, err := auth.Encrypt(cfg.TargetConfigJSON, s.masterSecret)
		if err != nil {
			httpError(w, fmt.Errorf("encrypt s3 config: %w", err), http.StatusInternalServerError)
			return
		}
		cfg.TargetConfigJSON = encrypted
	}

	if err := s.store.CreateBackupConfig(&cfg); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	// Hot-reload schedule
	if s.backupScheduler != nil && cfg.ScheduleCron != "" {
		if err := s.backupScheduler.ScheduleConfig(cfg.ID, cfg.ScheduleCron); err != nil {
			log.Printf("[api] schedule backup config %d: %v", cfg.ID, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cfg)
}

func (s *Server) handleUpdateBackupConfig(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	existing, err := s.store.GetBackupConfig(id)
	if err != nil {
		http.Error(w, "backup config not found", http.StatusNotFound)
		return
	}

	var cfg store.BackupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	cfg.ID = existing.ID
	cfg.AppID = existing.AppID

	// Encrypt S3 target config if present
	if cfg.Target == "s3" && cfg.TargetConfigJSON != "" && s.masterSecret != "" {
		encrypted, err := auth.Encrypt(cfg.TargetConfigJSON, s.masterSecret)
		if err != nil {
			httpError(w, fmt.Errorf("encrypt s3 config: %w", err), http.StatusInternalServerError)
			return
		}
		cfg.TargetConfigJSON = encrypted
	}

	if err := s.store.UpdateBackupConfig(&cfg); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	// Reschedule
	if s.backupScheduler != nil {
		s.backupScheduler.UnscheduleConfig(cfg.ID)
		if cfg.ScheduleCron != "" {
			if err := s.backupScheduler.ScheduleConfig(cfg.ID, cfg.ScheduleCron); err != nil {
				log.Printf("[api] reschedule backup config %d: %v", cfg.ID, err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func (s *Server) handleDeleteBackupConfig(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Unschedule before deleting
	if s.backupScheduler != nil {
		s.backupScheduler.UnscheduleConfig(id)
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

	// Trigger ALL configs for the app
	for _, cfg := range cfgs {
		cfgID := cfg.ID
		go func() {
			if err := s.backupScheduler.RunBackup(context.Background(), cfgID); err != nil {
				log.Printf("[api] backup config %d: %v", cfgID, err)
			}
		}()
	}

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
		if err := s.backupScheduler.RunRestore(context.Background(), runID); err != nil {
			log.Printf("[api] restore run %d: %v", runID, err)
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

func (s *Server) handleDetectStrategies(w http.ResponseWriter, r *http.Request) {
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

	cfg, parseErr := compose.ParseFile(app.ComposePath, app.Name)
	if parseErr != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"strategies": []backup.DetectionResult{},
			"error":      "could not parse compose file",
		})
		return
	}

	detector := s.backupScheduler.GetDetector()
	results := detector.DetectAll(cfg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"strategies": results,
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
			log.Printf("[api] backup config %d: %v", cfgID, err)
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

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	run, err := s.store.GetBackupRun(runID)
	if err != nil {
		http.Error(w, "backup run not found", http.StatusNotFound)
		return
	}
	if run.Status != "ok" {
		http.Error(w, "backup run not successful", http.StatusBadRequest)
		return
	}

	cfg, err := s.store.GetBackupConfig(run.BackupConfigID)
	if err != nil {
		http.Error(w, "backup config not found", http.StatusNotFound)
		return
	}

	filename := filepath.Base(run.FilePath)

	switch cfg.Target {
	case "local":
		// Stream file directly
		f, err := os.Open(run.FilePath)
		if err != nil {
			httpError(w, fmt.Errorf("open backup file: %w", err), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Type", "application/octet-stream")
		io.Copy(w, f)

	case "s3":
		// Decrypt S3 config and generate pre-signed URL
		targetJSON := cfg.TargetConfigJSON
		if s.masterSecret != "" {
			if decrypted, err := auth.Decrypt(targetJSON, s.masterSecret); err == nil {
				targetJSON = decrypted
			}
		}
		var s3cfg backup.S3Config
		if err := json.Unmarshal([]byte(targetJSON), &s3cfg); err != nil {
			httpError(w, fmt.Errorf("parse s3 config: %w", err), http.StatusInternalServerError)
			return
		}
		target, err := backup.NewS3Target(s3cfg)
		if err != nil {
			httpError(w, fmt.Errorf("create s3 target: %w", err), http.StatusInternalServerError)
			return
		}
		url, err := target.PresignedURL(r.Context(), run.FilePath, 15*time.Minute)
		if err != nil {
			httpError(w, fmt.Errorf("presign url: %w", err), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)

	default:
		http.Error(w, "unsupported target type for download", http.StatusBadRequest)
	}
}

func (s *Server) handleUploadRestore(w http.ResponseWriter, r *http.Request) {
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

	// 32MB max upload
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	strategy := r.FormValue("strategy")
	container := r.FormValue("container")
	if strategy == "" {
		http.Error(w, "strategy required", http.StatusBadRequest)
		return
	}

	// Validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	validExts := map[string]bool{".sql": true, ".gz": true, ".tar": true, ".rdb": true, ".db": true, ".bak": true, ".dump": true}
	if !validExts[ext] {
		http.Error(w, "unsupported file extension", http.StatusBadRequest)
		return
	}

	// Save to temp file
	tmpDir := filepath.Join(s.dataDir, "tmp")
	os.MkdirAll(tmpDir, 0755)
	tmpFile, err := os.CreateTemp(tmpDir, "restore-*"+ext)
	if err != nil {
		httpError(w, fmt.Errorf("create temp: %w", err), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		httpError(w, fmt.Errorf("save upload: %w", err), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()
	tmpPath := tmpFile.Name()

	containerName := container
	if containerName == "" {
		containerName = app.Name
	}

	// Async restore
	go func() {
		defer os.Remove(tmpPath)

		st, ok := s.backupScheduler.GetStrategy(strategy)
		if !ok {
			log.Printf("[api] upload restore: unknown strategy %s", strategy)
			return
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			log.Printf("[api] upload restore: open temp: %v", err)
			return
		}
		defer f.Close()

		opts := backup.RestoreOpts{
			ContainerName: containerName,
			Reader:        f,
		}
		if err := st.Restore(context.Background(), opts); err != nil {
			log.Printf("[api] upload restore %s/%s: %v", slug, strategy, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

// Compose version handlers

func (s *Server) handleUpdateComposeVersion(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	// Verify version belongs to app
	ver, err := s.store.GetComposeVersion(id)
	if err != nil {
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}
	if ver.AppID != app.ID {
		http.Error(w, "version does not belong to app", http.StatusForbidden)
		return
	}

	var body struct {
		Name        string `json:"name"`
		Notes       string `json:"notes"`
		EnvSnapshot string `json:"env_snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateComposeVersion(id, body.Name, body.Notes, body.EnvSnapshot); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	updated, _ := s.store.GetComposeVersion(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleDownloadComposeVersion(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	ver, err := s.store.GetComposeVersion(id)
	if err != nil {
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}
	if ver.AppID != app.ID {
		http.Error(w, "version does not belong to app", http.StatusForbidden)
		return
	}

	filename := fmt.Sprintf("%s-v%d-docker-compose.yml", slug, ver.Version)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Write([]byte(ver.Content))
}

func (s *Server) handleRestoreComposeVersion(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	ver, err := s.store.GetComposeVersion(id)
	if err != nil {
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}
	if ver.AppID != app.ID {
		http.Error(w, "version does not belong to app", http.StatusForbidden)
		return
	}

	// Write compose content to file
	if err := os.WriteFile(app.ComposePath, []byte(ver.Content), 0644); err != nil {
		httpError(w, fmt.Errorf("write compose: %w", err), http.StatusInternalServerError)
		return
	}

	// Redeploy
	if s.reconciler != nil {
		go func() {
			if err := s.reconciler.DeployOne(context.Background(), app.ComposePath, app.Name); err != nil {
				log.Printf("[api] restore version redeploy %s: %v", slug, err)
			}
		}()
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "restoring"})
}
