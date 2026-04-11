package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/store"
)

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.ListApps()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if apps == nil {
		apps = []store.App{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apps)
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	type appResponse struct {
		store.App
		Deploying bool `json:"deploying"`
	}
	resp := appResponse{
		App:       *app,
		Deploying: s.reconciler != nil && s.reconciler.IsDeploying(slug),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
