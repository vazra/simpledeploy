package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	opts := store.ListAppsOptions{}
	if v := r.URL.Query().Get("include_archived"); v == "1" || v == "true" {
		opts.IncludeArchived = true
	}
	apps, err := s.store.ListAppsWithOptions(opts)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if apps == nil {
		apps = []store.App{}
	}
	// Filter for non-super_admin callers: only return apps they have access to.
	if user := GetAuthUser(r); user != nil && user.Role != "super_admin" {
		allowed, err := s.store.GetUserAppSlugs(user.ID)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		allowSet := make(map[string]struct{}, len(allowed))
		for _, slug := range allowed {
			allowSet[slug] = struct{}{}
		}
		filtered := apps[:0]
		for _, a := range apps {
			if _, ok := allowSet[a.Slug]; ok {
				filtered = append(filtered, a)
			}
		}
		apps = filtered
		if apps == nil {
			apps = []store.App{}
		}
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

	labels, _ := s.store.GetAppLabels(slug)

	// Extract endpoints from compose file (includes service names)
	var endpoints []compose.EndpointConfig
	if app.ComposePath != "" {
		if cfg, err := compose.ParseFile(app.ComposePath, slug); err == nil {
			endpoints = cfg.Endpoints
		}
	}
	if endpoints == nil {
		endpoints = []compose.EndpointConfig{}
	}

	type appResponse struct {
		store.App
		Deploying bool                     `json:"deploying"`
		Labels    map[string]string        `json:"Labels,omitempty"`
		Endpoints []compose.EndpointConfig `json:"endpoints"`
	}
	resp := appResponse{
		App:       *app,
		Deploying: s.reconciler != nil && s.reconciler.IsDeploying(slug),
		Labels:    labels,
		Endpoints: endpoints,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
