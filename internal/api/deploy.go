package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/mirror"
	"github.com/vazra/simpledeploy/internal/store"
)

var validAppName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

type reconciler interface {
	DeployOne(ctx context.Context, composePath, appName string) error
	RemoveOne(ctx context.Context, appName string) error
	RestartOne(ctx context.Context, slug string) error
	StopOne(ctx context.Context, slug string) error
	StartOne(ctx context.Context, slug string) error
	PullOne(ctx context.Context, slug string) error
	ScaleOne(ctx context.Context, slug string, scales map[string]int) error
	AppServices(ctx context.Context, slug string) ([]deployer.ServiceStatus, error)
	RollbackOne(ctx context.Context, slug string, versionID int64) error
	ListVersions(ctx context.Context, slug string) ([]store.ComposeVersion, error)
	ListDeployEvents(ctx context.Context, slug string) ([]store.DeployEvent, error)
	Reconcile(ctx context.Context) error
	CancelOne(ctx context.Context, slug string) error
	IsDeploying(slug string) bool
	SubscribeDeployLog(slug string) (<-chan deployer.OutputLine, func(), bool)
}

func (s *Server) SetAppsDir(dir string) { s.appsDir = dir }

func (s *Server) SetReconciler(rec reconciler) { s.reconciler = rec }

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Compose string `json:"compose"`
		Source  string `json:"source"`
		Force   bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.Compose == "" {
		http.Error(w, "name and compose are required", http.StatusBadRequest)
		return
	}
	if !validAppName.MatchString(body.Name) {
		http.Error(w, "invalid app name: must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,62}", http.StatusBadRequest)
		return
	}

	// Collision handling: never clobber an existing app.
	// Manual flow: reject and ask user to delete first.
	// Template flow: suggest a free candidate name (foo-2..foo-50).
	if s.store != nil && !body.Force {
		if _, err := s.store.GetAppBySlug(body.Name); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			if body.Source == "template" {
				suggested := ""
				for i := 2; i <= 50; i++ {
					candidate := fmt.Sprintf("%s-%d", body.Name, i)
					if _, err := s.store.GetAppBySlug(candidate); err == nil {
						continue
					}
					if _, statErr := os.Stat(filepath.Join(s.appsDir, candidate)); !os.IsNotExist(statErr) {
						continue
					}
					suggested = candidate
					break
				}
				resp := map[string]string{
					"error": fmt.Sprintf("app %q already exists", body.Name),
				}
				if suggested != "" {
					resp["suggested_name"] = suggested
				}
				json.NewEncoder(w).Encode(resp)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("app %q already exists. Delete it before reusing the name.", body.Name),
			})
			return
		}
	}

	composeData, err := base64.StdEncoding.DecodeString(body.Compose)
	if err != nil {
		http.Error(w, "invalid base64 compose data", http.StatusBadRequest)
		return
	}

	// Optional image mirror: when SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX is set
	// (E2E/CI or dev), rewrite docker.io-bound image refs to the mirror
	// before the compose file is persisted.
	if prefix := os.Getenv("SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX"); prefix != "" {
		composeData = mirror.RewriteCompose(composeData, prefix)
	}

	// Attach the shared public network so endpoint services are reachable
	// from the host-native Caddy without requiring published ports. On failure,
	// log and fall through with the original bytes so the validator below can
	// still produce a useful error.
	if injected, changed, err := compose.InjectSharedNetwork(composeData, "simpledeploy-public"); err != nil {
		log.Printf("[deploy] inject shared network for %s: %v (continuing)", body.Name, err)
	} else {
		composeData = injected
		_ = changed
	}

	appDir := filepath.Join(s.appsDir, body.Name)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		http.Error(w, "failed to create app directory", http.StatusInternalServerError)
		return
	}

	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, composeData, 0644); err != nil {
		http.Error(w, "failed to write compose file", http.StatusInternalServerError)
		return
	}

	// Validate compose file for dangerous directives
	parsed, err := compose.ParseFile(composePath, body.Name)
	if err != nil {
		os.Remove(composePath)
		http.Error(w, "invalid compose file", http.StatusBadRequest)
		return
	}
	if violations := compose.ValidateComposeSecurity(parsed); len(violations) > 0 {
		os.Remove(composePath)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error":      "compose file contains disallowed directives",
			"violations": violations,
		})
		return
	}

	s.EnqueueGitCommit([]string{composePath}, "deploy:"+body.Name)

	if s.reconciler != nil {
		go func() {
			if err := s.reconciler.DeployOne(context.Background(), composePath, body.Name); err != nil {
				fmt.Fprintf(os.Stderr, "deploy %s: %v\n", body.Name, err)
			}
		}()
	}

	if s.audit != nil {
		caller := GetAuthUser(r)
		name := ""
		if caller != nil {
			name = caller.Username
		}
		s.audit.Log(audit.Event{Type: "deploy", Username: name, Detail: body.Name, Success: true})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"name":   body.Name,
		"status": "started",
	})
}

func (s *Server) handleRemoveApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !validAppName.MatchString(slug) {
		http.Error(w, "invalid app name", http.StatusBadRequest)
		return
	}

	if s.reconciler != nil {
		if err := s.reconciler.RemoveOne(r.Context(), slug); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}

	if s.cs != nil {
		if err := s.cs.DeleteAppSidecar(slug); err != nil {
			log.Printf("[configsync] DeleteAppSidecar %s: %v", slug, err)
		}
	}

	appDir := filepath.Join(s.appsDir, slug)
	if err := os.RemoveAll(appDir); err != nil {
		http.Error(w, "failed to remove app directory", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleValidateCompose(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Compose string `json:"compose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Compose == "" {
		http.Error(w, "compose is required", http.StatusBadRequest)
		return
	}

	composeData, err := base64.StdEncoding.DecodeString(body.Compose)
	if err != nil {
		http.Error(w, "invalid base64 compose data", http.StatusBadRequest)
		return
	}

	tmpFile, err := os.CreateTemp("", "validate-compose-*.yml")
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(composeData); err != nil {
		tmpFile.Close()
		http.Error(w, "failed to write temp file", http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	type validateResponse struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors,omitempty"`
	}

	_, parseErr := compose.ParseFile(tmpFile.Name(), "validate")
	if parseErr != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(validateResponse{
			Valid:  false,
			Errors: []string{parseErr.Error()},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(validateResponse{Valid: true})
}

func (s *Server) handleGetCompose(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	composePath := filepath.Join(s.appsDir, slug, "docker-compose.yml")

	data, err := os.ReadFile(composePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "compose file not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to read compose file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/yaml")
	w.Write(data)
}
