package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vazra/simpledeploy/internal/auth"
)

type registryRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type registryResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Server) handleListRegistries(w http.ResponseWriter, r *http.Request) {
	regs, err := s.store.ListRegistries()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	resp := make([]registryResponse, len(regs))
	for i, reg := range regs {
		username := ""
		if s.masterSecret != "" {
			username, _ = auth.Decrypt(reg.UsernameEnc, s.masterSecret)
		}
		resp[i] = registryResponse{
			ID:        reg.ID,
			Name:      reg.Name,
			URL:       reg.URL,
			Username:  username,
			CreatedAt: reg.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: reg.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateRegistry(w http.ResponseWriter, r *http.Request) {
	var req registryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.URL == "" || req.Username == "" || req.Password == "" {
		http.Error(w, "name, url, username, password required", http.StatusBadRequest)
		return
	}
	if s.masterSecret == "" {
		http.Error(w, "master_secret not configured", http.StatusInternalServerError)
		return
	}
	usernameEnc, err := auth.Encrypt(req.Username, s.masterSecret)
	if err != nil {
		httpError(w, fmt.Errorf("encrypt username: %w", err), http.StatusInternalServerError)
		return
	}
	passwordEnc, err := auth.Encrypt(req.Password, s.masterSecret)
	if err != nil {
		httpError(w, fmt.Errorf("encrypt password: %w", err), http.StatusInternalServerError)
		return
	}
	reg, err := s.store.CreateRegistry(req.Name, req.URL, usernameEnc, passwordEnc)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	s.recordAudit(r, nil, "registry", "added", nil, map[string]any{"name": reg.Name, "url": reg.URL})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(registryResponse{
		ID:        reg.ID,
		Name:      reg.Name,
		URL:       reg.URL,
		Username:  req.Username,
		CreatedAt: reg.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: reg.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (s *Server) handleUpdateRegistry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Load existing for before-snapshot.
	existing, _ := s.store.GetRegistry(id)
	var req registryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.URL == "" || req.Username == "" || req.Password == "" {
		http.Error(w, "name, url, username, password required", http.StatusBadRequest)
		return
	}
	if s.masterSecret == "" {
		http.Error(w, "master_secret not configured", http.StatusInternalServerError)
		return
	}
	usernameEnc, err := auth.Encrypt(req.Username, s.masterSecret)
	if err != nil {
		httpError(w, fmt.Errorf("encrypt username: %w", err), http.StatusInternalServerError)
		return
	}
	passwordEnc, err := auth.Encrypt(req.Password, s.masterSecret)
	if err != nil {
		httpError(w, fmt.Errorf("encrypt password: %w", err), http.StatusInternalServerError)
		return
	}
	if err := s.store.UpdateRegistry(id, req.Name, req.URL, usernameEnc, passwordEnc); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	var beforeVal any
	if existing != nil {
		beforeVal = map[string]any{"name": existing.Name, "url": existing.URL}
	}
	s.recordAudit(r, nil, "registry", "changed", beforeVal, map[string]any{"name": req.Name, "url": req.URL})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteRegistry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, _ := s.store.GetRegistry(id)
	if err := s.store.DeleteRegistry(id); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if existing != nil {
		s.recordAudit(r, nil, "registry", "removed", map[string]any{"name": existing.Name, "url": existing.URL}, nil)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
