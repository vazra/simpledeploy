package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

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
}

func (s *Server) SetAppsDir(dir string) { s.appsDir = dir }

func (s *Server) SetReconciler(rec reconciler) { s.reconciler = rec }

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Compose string `json:"compose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.Compose == "" {
		http.Error(w, "name and compose are required", http.StatusBadRequest)
		return
	}

	composeData, err := base64.StdEncoding.DecodeString(body.Compose)
	if err != nil {
		http.Error(w, "invalid base64 compose data", http.StatusBadRequest)
		return
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

	if s.reconciler != nil {
		if err := s.reconciler.DeployOne(r.Context(), composePath, body.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"name":   body.Name,
		"status": "deployed",
	})
}

func (s *Server) handleRemoveApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	if s.reconciler != nil {
		if err := s.reconciler.RemoveOne(r.Context(), slug); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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
