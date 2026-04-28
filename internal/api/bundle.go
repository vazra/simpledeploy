package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vazra/simpledeploy/internal/appbundle"
	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

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
	appDir := filepath.Join(s.appsDir, slug)
	zipBytes, err := appbundle.Build(appDir, app.Slug, app.Name, s.buildVersion)
	if err != nil {
		log.Printf("[export] build %s: %v", slug, err)
		http.Error(w, "failed to build bundle", http.StatusInternalServerError)
		return
	}

	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		AppID:    &app.ID,
		AppSlug:  app.Slug,
		Category: "lifecycle",
		Action:   "exported",
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

	// Validate compose security.
	tmp, err := os.CreateTemp("", "import-compose-*.yml")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bundle.Compose); err != nil {
		tmp.Close()
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tmp.Close()
	parsed, err := compose.ParseFile(tmp.Name(), slug)
	if err != nil {
		http.Error(w, "invalid compose file in bundle", http.StatusBadRequest)
		return
	}
	if violations := compose.ValidateComposeSecurity(parsed); len(violations) > 0 {
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

	composeData := bundle.Compose
	if injected, _, ierr := compose.InjectSharedNetwork(composeData, "simpledeploy-public"); ierr == nil {
		composeData = injected
	}

	appDir := filepath.Join(s.appsDir, slug)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		http.Error(w, "failed to create app directory", http.StatusInternalServerError)
		return
	}
	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, composeData, 0o644); err != nil {
		http.Error(w, "failed to write compose file", http.StatusInternalServerError)
		return
	}

	// Sidecar: always overwrite if present in bundle.
	if len(bundle.Sidecar) > 0 {
		if err := os.WriteFile(filepath.Join(appDir, "simpledeploy.yml"), bundle.Sidecar, 0o644); err != nil {
			log.Printf("[import] write sidecar %s: %v", slug, err)
		}
	}
	// .env: only write for mode=new (do not clobber existing env on overwrite).
	if mode == "new" && len(bundle.EnvExample) > 0 {
		envPath := filepath.Join(appDir, ".env")
		if err := os.WriteFile(envPath, bundle.EnvExample, 0o644); err != nil {
			log.Printf("[import] write .env %s: %v", slug, err)
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
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		AppID:    appID,
		AppSlug:  slug,
		Category: "lifecycle",
		Action:   "imported",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"slug": slug,
		"mode": mode,
	})
}
