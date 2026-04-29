package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

// serviceInfo is the enriched per-service response for GET /apps/:slug/services.
// One entry per compose service (not per container), with replica count and
// scale eligibility derived from the compose file.
type serviceInfo struct {
	Service     string `json:"service"`
	State       string `json:"state"`
	Health      string `json:"health,omitempty"`
	Replicas    int    `json:"replicas"`
	Scalable    bool   `json:"scalable"`
	ScaleReason string `json:"scale_reason,omitempty"`
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	go func() {
		if err := s.reconciler.RestartOne(context.Background(), slug); err != nil {
			fmt.Fprintf(os.Stderr, "restart %s: %v\n", slug, err)
		}
	}()
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "status": "restarting"})
	_, _ = s.audit.Record(ctx, audit.RecordReq{
		Category: "lifecycle",
		Action:   "restarted",
		AppSlug:  slug,
		After:    afterJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	if err := s.reconciler.StopOne(r.Context(), slug); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "status": "stopped"})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "stopped",
		AppSlug:  slug,
		After:    afterJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	if err := s.reconciler.StartOne(r.Context(), slug); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "status": "running"})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "started",
		AppSlug:  slug,
		After:    afterJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	go func() {
		if err := s.reconciler.PullOne(context.Background(), slug); err != nil {
			fmt.Fprintf(os.Stderr, "pull %s: %v\n", slug, err)
		}
	}()
	afterJSON, _ := json.Marshal(map[string]any{"name": slug})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "image_pulled",
		AppSlug:  slug,
		After:    afterJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	var body struct {
		Scales map[string]int `json:"scales"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body.Scales) == 0 {
		http.Error(w, "scales is required", http.StatusBadRequest)
		return
	}

	// Capture before/after replica counts for audit. Before-state comes from
	// live container counts so down/up-scales are visible in the audit log.
	var beforeReplicas, afterReplicas int
	if statuses, err := s.reconciler.AppServices(r.Context(), slug); err == nil {
		for _, c := range statuses {
			if _, want := body.Scales[c.Service]; want {
				beforeReplicas++
			}
		}
	}
	for _, v := range body.Scales {
		afterReplicas += v
	}

	// Validate against compose config: refuse to scale services that are not
	// scalable. This stops silly requests (e.g. scaling a postgres) before
	// docker compose returns a generic failure.
	cfg, cfgErr := s.reconciler.AppConfig(slug)
	if cfgErr != nil {
		log.Printf("[api] scale %s: load compose config: %v", slug, cfgErr)
	}
	if cfg != nil {
		known := map[string]*compose.ServiceConfig{}
		for i := range cfg.Services {
			known[cfg.Services[i].Name] = &cfg.Services[i]
		}
		for name := range body.Scales {
			svc, ok := known[name]
			if !ok {
				http.Error(w, "unknown service: "+name, http.StatusBadRequest)
				return
			}
			if okScale, reason := svc.ScaleEligibility(); !okScale {
				http.Error(w, "cannot scale "+name+": "+reason, http.StatusBadRequest)
				return
			}
		}
	}

	if err := s.reconciler.ScaleOne(r.Context(), slug, body.Scales); err != nil {
		// Forward the actual error (which wraps docker compose stderr) so the
		// UI can show something more useful than "Internal Server Error".
		log.Printf("[api] scale %s: %v", slug, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	beforeJSON, _ := json.Marshal(map[string]any{"name": slug, "replicas": beforeReplicas})
	afterJSON, _ := json.Marshal(map[string]any{"name": slug, "replicas": afterReplicas})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "scaled",
		AppSlug:  slug,
		Before:   beforeJSON,
		After:    afterJSON,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGetServices(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	containers, err := s.reconciler.AppServices(r.Context(), slug)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	// Aggregate container-level statuses by service name.
	type agg struct {
		state    string
		health   string
		replicas int
	}
	byName := map[string]*agg{}
	order := []string{}
	for _, c := range containers {
		a, ok := byName[c.Service]
		if !ok {
			a = &agg{}
			byName[c.Service] = a
			order = append(order, c.Service)
		}
		a.replicas++
		// Worst-state wins so a single bad replica is visible.
		if worseState(c.State, a.state) {
			a.state = c.State
		}
		if worseHealth(c.Health, a.health) {
			a.health = c.Health
		}
	}

	// Pull compose config to enrich with scale eligibility and to surface
	// configured services that aren't running yet.
	cfg, _ := s.reconciler.AppConfig(slug)
	scaleInfo := map[string]struct {
		scalable bool
		reason   string
	}{}
	if cfg != nil {
		for i := range cfg.Services {
			svc := &cfg.Services[i]
			ok, reason := svc.ScaleEligibility()
			scaleInfo[svc.Name] = struct {
				scalable bool
				reason   string
			}{ok, reason}
			if _, seen := byName[svc.Name]; !seen {
				byName[svc.Name] = &agg{}
				order = append(order, svc.Name)
			}
		}
	}

	out := make([]serviceInfo, 0, len(order))
	for _, name := range order {
		a := byName[name]
		info := scaleInfo[name]
		state := a.state
		if state == "" && a.replicas == 0 {
			state = "stopped"
		}
		out = append(out, serviceInfo{
			Service:     name,
			State:       state,
			Health:      a.health,
			Replicas:    a.replicas,
			Scalable:    info.scalable,
			ScaleReason: info.reason,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// worseState reports whether candidate is worse than current (running < other states).
func worseState(candidate, current string) bool {
	if current == "" {
		return candidate != ""
	}
	rank := func(s string) int {
		switch s {
		case "running":
			return 0
		case "restarting":
			return 2
		case "":
			return -1
		default:
			return 1
		}
	}
	return rank(candidate) > rank(current)
}

func worseHealth(candidate, current string) bool {
	if current == "" {
		return candidate != ""
	}
	rank := func(s string) int {
		switch s {
		case "":
			return -1
		case "healthy":
			return 0
		case "starting":
			return 1
		case "unhealthy":
			return 2
		default:
			return 0
		}
	}
	return rank(candidate) > rank(current)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	var body struct {
		VersionID int64 `json:"version_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.VersionID == 0 {
		http.Error(w, "version_id is required", http.StatusBadRequest)
		return
	}
	if err := s.reconciler.RollbackOne(r.Context(), slug, body.VersionID); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{"version_id": body.VersionID})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "rolled_back",
		AppSlug:  slug,
		After:    afterJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	versions, err := s.reconciler.ListVersions(r.Context(), slug)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if versions == nil {
		versions = []store.ComposeVersion{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid version id", http.StatusBadRequest)
		return
	}
	// Capture version number + app for audit before deletion.
	var beforeJSON []byte
	var appID *int64
	var appSlug string
	if v, err := s.store.GetComposeVersion(id); err == nil && v != nil {
		beforeJSON, _ = json.Marshal(map[string]any{"version": v.Version})
		if a, err := s.store.GetAppByID(v.AppID); err == nil && a != nil {
			aid := a.ID
			appID = &aid
			appSlug = a.Slug
		}
	}
	if err := s.store.DeleteComposeVersion(id); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "compose",
		Action:   "version_removed",
		AppID:    appID,
		AppSlug:  appSlug,
		Before:   beforeJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListDeployEvents(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if s.reconciler == nil {
		http.Error(w, "reconciler not configured", http.StatusInternalServerError)
		return
	}
	events, err := s.reconciler.ListDeployEvents(r.Context(), slug)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []store.DeployEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
