package api

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validDomain = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.*-]*$`)

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

	if domain == "" || !validDomain.MatchString(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
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

	if block, _ := pem.Decode([]byte(req.Cert)); block == nil || block.Type != "CERTIFICATE" {
		http.Error(w, "cert is not valid PEM", http.StatusBadRequest)
		return
	}
	if block, _ := pem.Decode([]byte(req.Key)); block == nil || !strings.Contains(block.Type, "KEY") {
		http.Error(w, "key is not valid PEM", http.StatusBadRequest)
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

	if domain == "" || !validDomain.MatchString(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
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
