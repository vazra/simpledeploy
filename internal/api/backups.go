package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/fsutil"
	"github.com/vazra/simpledeploy/internal/mirror"
	"github.com/vazra/simpledeploy/internal/store"
)

// backupCfgAuditJSON builds the backupView JSON shape for audit records.
func backupCfgAuditJSON(cfg *store.BackupConfig) []byte {
	name := cfg.Strategy + "/" + cfg.Target
	b, _ := json.Marshal(map[string]any{
		"name":     name,
		"schedule": cfg.ScheduleCron,
		"target":   cfg.Target,
		"strategy": cfg.Strategy,
	})
	return b
}

// validateBackupConfig checks user-supplied backup settings before they are
// stored: the cron schedule, backup paths, and any custom S3 endpoint.
// Errors are human-readable and meant to be returned as 400.
func validateBackupConfig(ctx context.Context, cfg *store.BackupConfig) error {
	cfg.ScheduleCron = strings.TrimSpace(cfg.ScheduleCron)
	if err := backup.ValidateCron(cfg.ScheduleCron); err != nil {
		return err
	}
	if err := backup.ValidatePathsConfig(cfg.Strategy, cfg.Paths); err != nil {
		return err
	}
	if cfg.Target == "s3" && cfg.TargetConfigJSON != "" {
		var s3cfg backup.S3Config
		if err := json.Unmarshal([]byte(cfg.TargetConfigJSON), &s3cfg); err != nil {
			return fmt.Errorf("invalid S3 settings: %v", err)
		}
		if err := backup.ValidateS3Endpoint(ctx, s3cfg.Endpoint); err != nil {
			return err
		}
	}
	return nil
}

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
	if err := validateBackupConfig(r.Context(), &cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

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

	appID := app.ID
	afterJSON := backupCfgAuditJSON(&cfg)
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "backup",
		Action:   "added",
		AppID:    &appID,
		AppSlug:  app.Slug,
		After:    afterJSON,
	})

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
	if !s.canMutateForApp(w, r, &existing.AppID) {
		return
	}

	var cfg store.BackupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	cfg.ID = existing.ID
	cfg.AppID = existing.AppID
	if err := validateBackupConfig(r.Context(), &cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Encrypt S3 target config if present
	if cfg.Target == "s3" && cfg.TargetConfigJSON != "" && s.masterSecret != "" {
		encrypted, err := auth.Encrypt(cfg.TargetConfigJSON, s.masterSecret)
		if err != nil {
			httpError(w, fmt.Errorf("encrypt s3 config: %w", err), http.StatusInternalServerError)
			return
		}
		cfg.TargetConfigJSON = encrypted
	}

	beforeJSON := backupCfgAuditJSON(existing)
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

	appID := existing.AppID
	var appSlug string
	if app, err := s.store.GetAppByID(existing.AppID); err == nil {
		appSlug = app.Slug
	}
	afterJSON := backupCfgAuditJSON(&cfg)
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "backup",
		Action:   "changed",
		AppID:    &appID,
		AppSlug:  appSlug,
		Before:   beforeJSON,
		After:    afterJSON,
	})

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

	existing, err := s.store.GetBackupConfig(id)
	if err != nil {
		http.Error(w, "backup config not found", http.StatusNotFound)
		return
	}
	if !s.canMutateForApp(w, r, &existing.AppID) {
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

	appID := existing.AppID
	var appSlug string
	if app, err := s.store.GetAppByID(existing.AppID); err == nil {
		appSlug = app.Slug
	}
	beforeJSON := backupCfgAuditJSON(existing)
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "backup",
		Action:   "removed",
		AppID:    &appID,
		AppSlug:  appSlug,
		Before:   beforeJSON,
	})

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

	run, err := s.store.GetBackupRun(runID)
	if err != nil {
		http.Error(w, "backup run not found", http.StatusNotFound)
		return
	}
	cfg, err := s.store.GetBackupConfig(run.BackupConfigID)
	if err != nil {
		http.Error(w, "backup config not found", http.StatusNotFound)
		return
	}
	if !s.canMutateForApp(w, r, &cfg.AppID) {
		return
	}

	// Shares the upload-restore slots: each restore stages the backup on
	// disk and drives docker, so cap how many run at once.
	select {
	case s.restoreSem <- struct{}{}:
	default:
		http.Error(w, "too many restores in progress, try again later", http.StatusTooManyRequests)
		return
	}

	go func() {
		defer func() { <-s.restoreSem }()
		if err := s.backupScheduler.RunRestore(context.Background(), runID); err != nil {
			log.Printf("[api] restore run %d: %v", runID, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleBackupSummary(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	apps, err := s.store.GetBackupSummary()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	runs, err := s.store.ListRecentBackupRuns(20)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	// Filter to apps the caller can see (super_admin sees all).
	if user.Role != "super_admin" {
		slugs, err := s.store.GetUserAppSlugs(user.ID)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		allowed := make(map[string]struct{}, len(slugs))
		for _, sl := range slugs {
			allowed[sl] = struct{}{}
		}
		filteredApps := make([]store.BackupSummaryApp, 0, len(apps))
		for _, a := range apps {
			if _, ok := allowed[a.AppSlug]; ok {
				filteredApps = append(filteredApps, a)
			}
		}
		apps = filteredApps
		filteredRuns := make([]store.BackupRunWithApp, 0, len(runs))
		for _, rn := range runs {
			if _, ok := allowed[rn.AppSlug]; ok {
				filteredRuns = append(filteredRuns, rn)
			}
		}
		runs = filteredRuns
	}

	if apps == nil {
		apps = []store.BackupSummaryApp{}
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

	cfg, parseErr := compose.ParseFile(app.ComposePath, "simpledeploy-"+app.Slug)
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

	cfg, err := s.store.GetBackupConfig(cfgID)
	if err != nil {
		http.Error(w, "backup config not found", http.StatusNotFound)
		return
	}
	if !s.canMutateForApp(w, r, &cfg.AppID) {
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
	if err := backup.ValidateS3Endpoint(r.Context(), cfg.Endpoint); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	if run.Status != "success" {
		http.Error(w, "backup run not successful", http.StatusBadRequest)
		return
	}

	cfg, err := s.store.GetBackupConfig(run.BackupConfigID)
	if err != nil {
		http.Error(w, "backup config not found", http.StatusNotFound)
		return
	}
	// Backup files contain hashed passwords, encrypted creds, full DB
	// dumps. Restrict to manage+grant or super_admin (mirror of the
	// restore endpoint's authorization). Closes audit M-15.
	if !s.canMutateForApp(w, r, &cfg.AppID) {
		return
	}

	filename := filepath.Base(run.FilePath)

	switch cfg.Target {
	case "local":
		// run.FilePath is stored as just the filename (relative to the local
		// target's backup dir). Resolve against dataDir/backups.
		absPath := run.FilePath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(s.dataDir, "backups", run.FilePath)
		}
		absPath = filepath.Clean(absPath)
		backupsRoot := filepath.Clean(filepath.Join(s.dataDir, "backups"))
		if !strings.HasPrefix(absPath, backupsRoot+string(filepath.Separator)) && absPath != backupsRoot {
			http.Error(w, "invalid backup path", http.StatusForbidden)
			return
		}
		f, err := os.Open(absPath)
		if err != nil {
			httpError(w, fmt.Errorf("open backup file: %w", err), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := io.Copy(w, f); err != nil {
			log.Printf("[api] download backup run %d: %v", runID, err)
		}

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
	requestedContainer := r.FormValue("container")
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

	// Only containers of this app's compose project may be restored into.
	containerName, status, err := s.resolveRestoreContainer(r.Context(), app, requestedContainer)
	if err != nil {
		if status == http.StatusInternalServerError {
			httpError(w, err, status)
		} else {
			http.Error(w, err.Error(), status)
		}
		return
	}

	// Reject when too many restores are already running. The semaphore is
	// shared across apps; holding a slot for the duration of a slow restore
	// would otherwise let unlimited concurrent uploads exhaust docker
	// daemon resources. Acquire before touching disk so rejected requests
	// never write the upload.
	select {
	case s.restoreSem <- struct{}{}:
	default:
		http.Error(w, "too many restores in progress, try again later", http.StatusTooManyRequests)
		return
	}
	release := func() { <-s.restoreSem }

	// Save to temp file. The dir holds raw backups (DB dumps), so owner-only.
	tmpDir := filepath.Join(s.dataDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		release()
		httpError(w, fmt.Errorf("create temp dir: %w", err), http.StatusInternalServerError)
		return
	}
	// MkdirAll keeps the mode of an existing dir; older versions made it 0755.
	if err := os.Chmod(tmpDir, 0o700); err != nil {
		release()
		httpError(w, fmt.Errorf("restrict temp dir: %w", err), http.StatusInternalServerError)
		return
	}
	tmpFile, err := os.CreateTemp(tmpDir, "restore-*"+ext)
	if err != nil {
		release()
		httpError(w, fmt.Errorf("create temp: %w", err), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		release()
		httpError(w, fmt.Errorf("save upload: %w", err), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()
	tmpPath := tmpFile.Name()

	// Async restore
	go func() {
		defer release()
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
			ContainerName:        containerName,
			Reader:               f,
			MaxDecompressedBytes: backup.UploadRestoreMaxBytes(),
		}
		if err := st.Restore(context.Background(), opts); err != nil {
			log.Printf("[api] upload restore %s/%s: %v", slug, strategy, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

// resolveRestoreContainer maps the upload-restore "container" field onto a
// container of app's compose project (simpledeploy-<app.Slug>), so a
// restore can never target another app's container. requested may be a
// container name (with or without leading "/"), a container ID or ID
// prefix of at least 12 characters, or a compose service name (resolved to
// that service's container). Empty requested falls back to app.Slug (or
// to the lone container of a single-container app), checked the same way;
// without Docker it falls back to app.Name unchecked. On error the
// returned status is the HTTP code to send.
func (s *Server) resolveRestoreContainer(ctx context.Context, app *store.App, requested string) (string, int, error) {
	requested = strings.TrimSpace(requested)
	if s.docker == nil {
		if requested != "" {
			return "", http.StatusBadRequest, fmt.Errorf("choosing a container is unavailable because SimpleDeploy is not connected to Docker; leave the container field empty")
		}
		return app.Name, 0, nil
	}

	// Compose projects are named after the slug; app.Name can be a display name.
	project := "simpledeploy-" + app.Slug
	list, err := s.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+project)),
	})
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("list app containers: %w", err)
	}
	// Re-check the project label rather than trusting the daemon filter alone.
	var own []container.Summary
	for _, c := range list {
		if c.Labels["com.docker.compose.project"] == project {
			own = append(own, c)
		}
	}
	sort.Slice(own, func(i, j int) bool { return restoreContainerName(own[i]) < restoreContainerName(own[j]) })

	candidate := requested
	if candidate == "" {
		candidate = app.Slug
	}
	bare := strings.TrimPrefix(candidate, "/")

	// Exact container name or ID (prefix) first.
	for _, c := range own {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == bare {
				return restoreContainerName(c), 0, nil
			}
		}
		if c.ID == candidate || (len(candidate) >= 12 && strings.HasPrefix(c.ID, candidate)) {
			return restoreContainerName(c), 0, nil
		}
	}
	// Then compose service name, preferring a running replica.
	var match *container.Summary
	for i := range own {
		c := &own[i]
		if c.Labels["com.docker.compose.service"] != candidate {
			continue
		}
		if match == nil || (c.State == "running" && match.State != "running") {
			match = c
		}
	}
	if match != nil {
		return restoreContainerName(*match), 0, nil
	}

	if len(own) == 0 {
		return "", http.StatusBadRequest, fmt.Errorf("no containers found for app %q. Deploy the app first, then try the restore again", app.Name)
	}
	// Single-container apps need no choice.
	if requested == "" && len(own) == 1 {
		return restoreContainerName(own[0]), 0, nil
	}
	seen := map[string]bool{}
	var options []string
	for _, c := range own {
		opt := c.Labels["com.docker.compose.service"]
		if opt == "" {
			opt = restoreContainerName(c)
		}
		if !seen[opt] {
			seen[opt] = true
			options = append(options, opt)
		}
	}
	if requested == "" {
		return "", http.StatusBadRequest, fmt.Errorf("choose which service to restore into. Services in this app: %s", strings.Join(options, ", "))
	}
	return "", http.StatusBadRequest, fmt.Errorf("container %q is not part of app %q. Choose one of this app's services: %s", requested, app.Name, strings.Join(options, ", "))
}

// restoreContainerName is the name docker exec should use for c.
func restoreContainerName(c container.Summary) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return c.ID
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

const versionRefused = "this version contains settings that are no longer allowed"

// checkVersionForRestore prepares a stored compose version as it will be
// written to composePath (image mirror, loopback ports) and checks it with
// the app's current .env against the security rules and other apps'
// domains, so a refused version leaves the running app untouched. Returns
// the content to write. On failure it writes a 400 ({error, violations}
// for parse and rule problems) or a 409 for a domain conflict.
func (s *Server) checkVersionForRestore(w http.ResponseWriter, r *http.Request, slug, composePath string, ver *store.ComposeVersion) ([]byte, bool) {
	composeData := []byte(ver.Content)
	if prefix := os.Getenv("SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX"); prefix != "" {
		composeData = mirror.RewriteCompose(composeData, prefix)
	}
	if os.Getenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK") != "true" {
		composeData = mirror.RewritePortsLoopback(composeData)
	}

	dotEnv, err := compose.ReadDotEnv(filepath.Dir(composePath))
	if err != nil {
		http.Error(w, dotEnvProblem, http.StatusBadRequest)
		return nil, false
	}
	parsed, err := compose.ParseContent(composeData, composePath, slug, dotEnv)
	if err != nil {
		violations, isViolation := violationsOf(err)
		switch {
		case isViolation:
			writeViolations(w, http.StatusBadRequest, versionRefused, violations)
		case errors.Is(err, compose.ErrDotEnv):
			http.Error(w, dotEnvProblem, http.StatusBadRequest)
		default:
			writeViolations(w, http.StatusBadRequest, "stored version is not a valid compose file", []string{err.Error()})
		}
		return nil, false
	}
	if violations := compose.ValidateComposeSecurity(parsed); len(violations) > 0 {
		writeViolations(w, http.StatusBadRequest, versionRefused, violations)
		return nil, false
	}
	// The version's domains may have been taken by another app since.
	var current []compose.EndpointConfig
	if cfg, err := parseComposeForDisplay(composePath, slug); err == nil {
		current = cfg.Endpoints
	}
	if code, msg := s.checkEndpointDomains(r, slug, parsed.Endpoints, current); code != 0 {
		http.Error(w, msg, code)
		return nil, false
	}
	return composeData, true
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

	// Capture before-state by reading the current compose file on disk.
	var oldYAML string
	if raw, err := fsutil.ReadRegularFile(app.ComposePath); err == nil {
		oldYAML = string(raw)
	}

	composeData, ok := s.checkVersionForRestore(w, r, slug, app.ComposePath, ver)
	if !ok {
		return
	}

	if err := fsutil.WriteFileAtomic(app.ComposePath, composeData, 0o600); err != nil {
		httpError(w, fmt.Errorf("write compose: %w", err), http.StatusInternalServerError)
		return
	}

	appID := app.ID
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category:         "compose",
		Action:           "changed",
		AppID:            &appID,
		AppSlug:          slug,
		Before:           composeAuditView(oldYAML),
		After:            composeAuditView(ver.Content),
		ComposeVersionID: &id,
	})

	// Redeploy
	if s.reconciler != nil {
		go func() {
			if err := s.reconciler.DeployOne(context.Background(), app.ComposePath, slug); err != nil {
				log.Printf("[api] restore version redeploy %s: %v", slug, err)
			}
		}()
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "restoring"})
}
