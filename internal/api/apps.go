package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/fsutil"
	"github.com/vazra/simpledeploy/internal/store"
)

// parseComposeForDisplay parses an app's compose file for read-only uses
// (showing endpoints, domain ownership). When the .env cannot be used it
// parses without it, when docker-compose.yml is a link to a regular file it
// reads the link's target, and when file references are refused it parses
// without them (compose.ParseForRoutes), so the app's domains stay known.
// The result must never be used to deploy or to write files.
func parseComposeForDisplay(composePath, slug string) (*compose.AppConfig, error) {
	cfg, err := compose.ParseFile(composePath, slug)
	if _, ok := violationsOf(err); ok {
		return compose.ParseForRoutes(composePath, slug)
	}
	switch {
	case err == nil:
		return cfg, nil
	case errors.Is(err, compose.ErrDotEnv):
		data, rerr := fsutil.ReadRegularFile(composePath)
		if rerr != nil {
			return nil, err
		}
		cfg, err = compose.ParseContent(data, composePath, slug, nil)
		if _, ok := violationsOf(err); ok {
			return compose.ParseForRoutes(composePath, slug)
		}
		return cfg, err
	case errors.Is(err, fsutil.ErrNotRegular):
		// Resolve the link, then read the target with the regular-file
		// checks so a FIFO or device target cannot block or be read.
		target, rerr := filepath.EvalSymlinks(composePath)
		if rerr != nil {
			return nil, err
		}
		data, rerr := fsutil.ReadRegularFile(target)
		if rerr != nil {
			return nil, err
		}
		dotEnv, derr := compose.ReadDotEnv(filepath.Dir(composePath))
		if derr != nil {
			dotEnv = nil
		}
		return compose.ParseContent(data, composePath, slug, dotEnv)
	}
	return nil, err
}

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
		if cfg, err := parseComposeForDisplay(app.ComposePath, slug); err == nil {
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
