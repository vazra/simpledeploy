package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

var endpointLabelRe = regexp.MustCompile(`^simpledeploy\.endpoints\.(\d+)\.(domain|port|tls)$`)

// extractEndpointsFromLabels builds EndpointConfigs from flat app_labels.
// Service field is not populated since labels are merged across services.
func extractEndpointsFromLabels(labels map[string]string) []compose.EndpointConfig {
	byIndex := map[int]*compose.EndpointConfig{}
	for k, v := range labels {
		m := endpointLabelRe.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		if byIndex[idx] == nil {
			byIndex[idx] = &compose.EndpointConfig{}
		}
		switch m[2] {
		case "domain":
			byIndex[idx].Domain = v
		case "port":
			byIndex[idx].Port = v
		case "tls":
			byIndex[idx].TLS = v
		}
	}

	indices := make([]int, 0, len(byIndex))
	for idx := range byIndex {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	eps := make([]compose.EndpointConfig, 0, len(indices))
	for _, idx := range indices {
		eps = append(eps, *byIndex[idx])
	}
	return eps
}

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
		Deploying bool                    `json:"deploying"`
		Labels    map[string]string       `json:"Labels,omitempty"`
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
