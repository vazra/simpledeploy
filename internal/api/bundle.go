package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/vazra/simpledeploy/internal/appbundle"
	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/fsutil"
	"github.com/vazra/simpledeploy/internal/mirror"
	"github.com/vazra/simpledeploy/internal/store"
)

// handleImportAppPreview parses an uploaded bundle and returns a diff summary
// without applying any changes. Used by the UI to confirm overwrite imports.
func (s *Server) handleImportAppPreview(w http.ResponseWriter, r *http.Request) {
	const maxBundle = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBundle)
	if err := r.ParseMultipartForm(maxBundle); err != nil {
		http.Error(w, "invalid multipart upload", http.StatusBadRequest)
		return
	}
	mode := r.FormValue("mode")
	slug := r.FormValue("slug")
	if mode != "new" && mode != "overwrite" {
		http.Error(w, "mode must be \"new\" or \"overwrite\"", http.StatusBadRequest)
		return
	}
	if !validAppName.MatchString(slug) {
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var zipBuf bytes.Buffer
	if _, err := zipBuf.ReadFrom(file); err != nil {
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		return
	}
	bundle, err := appbundle.Parse(zipBuf.Bytes())
	if err != nil {
		http.Error(w, "invalid bundle: "+err.Error(), http.StatusBadRequest)
		return
	}

	incoming := map[string]any{
		"compose":  string(bundle.Compose),
		"sidecar":  string(bundle.Sidecar),
		"manifest": bundle.Manifest,
	}

	resp := map[string]any{
		"mode":     mode,
		"slug":     slug,
		"incoming": incoming,
	}

	if mode == "new" {
		resp["current"] = nil
		resp["changes"] = map[string]any{
			"compose_changed":           true,
			"sidecar_changed":           len(bundle.Sidecar) > 0,
			"alert_rule_count_delta":    countAlertRules(bundle.Sidecar),
			"backup_config_count_delta": countBackupConfigs(bundle.Sidecar),
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// overwrite
	existing, err := s.store.GetAppBySlug(slug)
	if err != nil || existing == nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	appDir := filepath.Join(s.appsDir, slug)
	curCompose, _ := fsutil.ReadRegularFile(filepath.Join(appDir, "docker-compose.yml"))
	curSidecar, _ := fsutil.ReadRegularFile(filepath.Join(appDir, "simpledeploy.yml"))

	resp["current"] = map[string]any{
		"compose": string(curCompose),
		"sidecar": string(curSidecar),
	}

	curAlerts := countAlertRules(curSidecar)
	curBackups := countBackupConfigs(curSidecar)
	inAlerts := countAlertRules(bundle.Sidecar)
	inBackups := countBackupConfigs(bundle.Sidecar)

	resp["changes"] = map[string]any{
		"compose_changed":              !bytes.Equal(curCompose, bundle.Compose),
		"sidecar_changed":              !bytes.Equal(curSidecar, bundle.Sidecar),
		"alert_rule_count_current":     curAlerts,
		"alert_rule_count_incoming":    inAlerts,
		"alert_rule_count_delta":       inAlerts - curAlerts,
		"backup_config_count_current":  curBackups,
		"backup_config_count_incoming": inBackups,
		"backup_config_count_delta":    inBackups - curBackups,
	}
	writeJSON(w, http.StatusOK, resp)
}

func countAlertRules(sidecarYAML []byte) int {
	if len(sidecarYAML) == 0 {
		return 0
	}
	var sc configsync.AppSidecar
	if err := yaml.Unmarshal(sidecarYAML, &sc); err != nil {
		return 0
	}
	return len(sc.AlertRules)
}

func countBackupConfigs(sidecarYAML []byte) int {
	if len(sidecarYAML) == 0 {
		return 0
	}
	var sc configsync.AppSidecar
	if err := yaml.Unmarshal(sidecarYAML, &sc); err != nil {
		return 0
	}
	return len(sc.BackupConfigs)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleExportApp returns a ZIP bundle of the app's on-disk config files.
func (s *Server) handleExportApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !validAppName.MatchString(slug) {
		http.Error(w, "invalid app name", http.StatusBadRequest)
		return
	}
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	// appbundle.Build reads with fsutil.ReadRegularFile, so a linked file in
	// the app folder is refused instead of pulling a host file into the zip.
	appDir := filepath.Join(s.appsDir, slug)
	zipBytes, err := appbundle.Build(appDir, app.Slug, app.Name, s.buildVersion)
	if err != nil {
		log.Printf("[export] build %s: %v", slug, err)
		if errors.Is(err, fsutil.ErrNotRegular) {
			http.Error(w, "the app folder has a linked file (docker-compose.yml, simpledeploy.yml or .env); replace it with a regular file", http.StatusConflict)
			return
		}
		http.Error(w, "failed to build bundle", http.StatusInternalServerError)
		return
	}

	exportedAfter, _ := json.Marshal(map[string]any{"name": app.Slug})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		AppID:    &app.ID,
		AppSlug:  app.Slug,
		Category: "lifecycle",
		Action:   "exported",
		After:    exportedAfter,
	})

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.simpledeploy.zip"`, slug))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBytes)
}

// handleImportApp accepts a multipart upload of a bundle ZIP and either
// creates a new app (mode=new) or replaces an existing app's compose
// (mode=overwrite).
func (s *Server) handleImportApp(w http.ResponseWriter, r *http.Request) {
	const maxBundle = 10 << 20 // 10 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxBundle)
	if err := r.ParseMultipartForm(maxBundle); err != nil {
		http.Error(w, "invalid multipart upload", http.StatusBadRequest)
		return
	}
	mode := r.FormValue("mode")
	slug := r.FormValue("slug")
	if mode != "new" && mode != "overwrite" {
		http.Error(w, "mode must be \"new\" or \"overwrite\"", http.StatusBadRequest)
		return
	}
	if !validAppName.MatchString(slug) {
		http.Error(w, "invalid slug: must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,62}", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	zipBytes := make([]byte, 0, 64<<10)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := file.Read(buf)
		if n > 0 {
			zipBytes = append(zipBytes, buf[:n]...)
		}
		if rerr != nil {
			break
		}
	}

	bundle, err := appbundle.Parse(zipBytes)
	if err != nil {
		http.Error(w, "invalid bundle: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Same port pinning as handleDeploy: published ports listen on
	// 127.0.0.1 so traffic has to pass Caddy's allowlist and rate limits.
	composeData := bundle.Compose
	if os.Getenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK") != "true" {
		composeData = mirror.RewritePortsLoopback(composeData)
	}
	if injected, _, ierr := compose.InjectSharedNetwork(composeData, "simpledeploy-public"); ierr == nil {
		composeData = injected
	}

	appDir := filepath.Join(s.appsDir, slug)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	// Validate exactly what will run: the final compose bytes in the real
	// app folder, with the .env that will sit next to them (the bundle's
	// env.example for a new app, otherwise the app's current .env).
	dotEnv, err := compose.ReadDotEnv(appDir)
	if err != nil {
		http.Error(w, dotEnvProblem, http.StatusBadRequest)
		return
	}
	if mode == "new" && len(bundle.EnvExample) > 0 {
		dotEnv = bundle.EnvExample
	}
	parsed, err := compose.ParseContent(composeData, composePath, slug, dotEnv)
	if errors.Is(err, compose.ErrDotEnv) {
		http.Error(w, dotEnvProblem, http.StatusBadRequest)
		return
	}
	if v, ok := violationsOf(err); ok {
		writeViolations(w, http.StatusBadRequest, "compose file contains disallowed directives", v)
		return
	}
	if err != nil {
		http.Error(w, "invalid compose file in bundle", http.StatusBadRequest)
		return
	}
	if violations := compose.ValidateComposeForDeploy(parsed); len(violations) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":      "compose file contains disallowed directives",
			"violations": violations,
		})
		return
	}

	existing, getErr := s.store.GetAppBySlug(slug)
	switch mode {
	case "new":
		if getErr == nil && existing != nil {
			http.Error(w, fmt.Sprintf("app %q already exists", slug), http.StatusConflict)
			return
		}
	case "overwrite":
		if getErr != nil || existing == nil {
			http.Error(w, "app not found", http.StatusNotFound)
			return
		}
	}

	if err := os.MkdirAll(appDir, 0o700); err != nil {
		http.Error(w, "failed to create app directory", http.StatusInternalServerError)
		return
	}
	// .env first: it was part of the validation above, so it must be in
	// place before the compose file the watcher deploys.
	// Only written for mode=new (do not clobber existing env on overwrite).
	if mode == "new" && len(bundle.EnvExample) > 0 {
		envPath := filepath.Join(appDir, ".env")
		if err := fsutil.WriteFileAtomic(envPath, bundle.EnvExample, 0o600); err != nil {
			log.Printf("[import] write .env %s: %v", slug, err)
			http.Error(w, "failed to write .env file", http.StatusInternalServerError)
			return
		}
	}
	if err := fsutil.WriteFileAtomic(composePath, composeData, 0o600); err != nil {
		http.Error(w, "failed to write compose file", http.StatusInternalServerError)
		return
	}

	// Sidecar: always overwrite if present in bundle.
	if len(bundle.Sidecar) > 0 {
		if err := fsutil.WriteFileAtomic(filepath.Join(appDir, "simpledeploy.yml"), bundle.Sidecar, 0o600); err != nil {
			log.Printf("[import] write sidecar %s: %v", slug, err)
		}
	}

	s.EnqueueGitCommit([]string{composePath}, "import:"+slug)

	// For new apps, ensure a store row exists before configsync apply (which
	// requires the app row).
	if mode == "new" {
		displayName := slug
		if bundle.Manifest.App.DisplayName != "" {
			displayName = bundle.Manifest.App.DisplayName
		}
		if err := s.store.UpsertApp(&store.App{
			Name:        displayName,
			Slug:        slug,
			ComposePath: composePath,
			Status:      "stopped",
		}, nil); err != nil {
			log.Printf("[import] upsert app %s: %v", slug, err)
		}
	}

	// Apply sidecar via configsync if available.
	if s.cs != nil && len(bundle.Sidecar) > 0 {
		loaded, lerr := s.cs.LoadAppFromFS(slug)
		if lerr != nil {
			log.Printf("[import] load sidecar %s: %v", slug, lerr)
		} else if loaded != nil {
			if aerr := s.cs.ApplyAppSidecar(slug, loaded); aerr != nil {
				log.Printf("[import] apply sidecar %s: %v", slug, aerr)
			}
		}
	}

	if s.reconciler != nil {
		go func() {
			if err := s.reconciler.DeployOne(context.Background(), composePath, slug); err != nil {
				fmt.Fprintf(os.Stderr, "import deploy %s: %v\n", slug, err)
			}
		}()
	}

	var appID *int64
	if mode == "overwrite" {
		appID = &existing.ID
	} else if app, err := s.store.GetAppBySlug(slug); err == nil {
		appID = &app.ID
	}
	importedAfter, _ := json.Marshal(map[string]any{"name": slug, "mode": mode})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		AppID:    appID,
		AppSlug:  slug,
		Category: "lifecycle",
		Action:   "imported",
		After:    importedAfter,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"slug": slug,
		"mode": mode,
	})
}
