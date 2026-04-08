package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

type reconciler interface {
	DeployOne(ctx context.Context, composePath, appName string) error
	RemoveOne(ctx context.Context, appName string) error
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
