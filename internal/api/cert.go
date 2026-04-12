package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type certRequest struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func (s *Server) handleUploadCert(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	domain := r.PathValue("domain")

	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	if domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	var req certRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Cert == "" || req.Key == "" {
		http.Error(w, "cert and key are required", http.StatusBadRequest)
		return
	}

	// Validate PEM-ish content
	if !strings.Contains(req.Cert, "BEGIN CERTIFICATE") {
		http.Error(w, "cert does not appear to be PEM encoded", http.StatusBadRequest)
		return
	}
	if !strings.Contains(req.Key, "BEGIN") {
		http.Error(w, "key does not appear to be PEM encoded", http.StatusBadRequest)
		return
	}

	certDir := filepath.Join(filepath.Dir(app.ComposePath), "certs")
	if err := os.MkdirAll(certDir, 0755); err != nil {
		httpError(w, fmt.Errorf("create certs dir: %w", err), http.StatusInternalServerError)
		return
	}

	certPath := filepath.Join(certDir, domain+".crt")
	keyPath := filepath.Join(certDir, domain+".key")

	if err := os.WriteFile(certPath, []byte(req.Cert), 0644); err != nil {
		httpError(w, fmt.Errorf("write cert: %w", err), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(keyPath, []byte(req.Key), 0600); err != nil {
		httpError(w, fmt.Errorf("write key: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteCert(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	domain := r.PathValue("domain")

	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	if domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	certDir := filepath.Join(filepath.Dir(app.ComposePath), "certs")
	certPath := filepath.Join(certDir, domain+".crt")
	keyPath := filepath.Join(certDir, domain+".key")

	os.Remove(certPath)
	os.Remove(keyPath)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
