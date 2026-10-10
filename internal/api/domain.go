package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/fsutil"
	"github.com/vazra/simpledeploy/internal/store"
	"gopkg.in/yaml.v3"
)

// endpointViewJSON builds an endpointView JSON for a single endpoint (or nil).
func endpointViewJSON(ep *compose.EndpointConfig) []byte {
	if ep == nil {
		return nil
	}
	tls := ep.TLS != "" && ep.TLS != "off"
	b, _ := json.Marshal(map[string]any{
		"host": ep.Domain,
		"tls":  tls,
		"path": ep.Path,
	})
	return b
}

// endpointDomainKey canonicalizes an endpoint domain for ownership checks:
// case-insensitive, trailing dot ignored (the proxy compares the same way).
func endpointDomainKey(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}

// SetReservedDomains marks domains app endpoints may not use, e.g. the
// dashboard's config `domain`. Empty entries are ignored.
func (s *Server) SetReservedDomains(domains ...string) {
	reserved := map[string]bool{}
	for _, d := range domains {
		if k := endpointDomainKey(d); k != "" {
			reserved[k] = true
		}
	}
	s.reservedDomains = reserved
}

// refuseEditOnUnsafeCompose rejects label edits (endpoints, IP allowlist)
// on an app whose compose file fails security validation. Such an app only
// keeps its routes while its file is unchanged, so editing it would take it
// offline. Returns true when a response was written.
func refuseEditOnUnsafeCompose(w http.ResponseWriter, cfg *compose.AppConfig) bool {
	violations := compose.ValidateComposeSecurity(cfg)
	if len(violations) == 0 {
		return false
	}
	writeViolations(w, http.StatusConflict, unsafeComposeEdit, violations)
	return true
}

const unsafeComposeEdit = "this app's compose file no longer passes security checks; fix it and redeploy before changing these settings"

// violationsOf returns the violations of a *compose.ViolationError in err.
func violationsOf(err error) ([]string, bool) {
	var ve *compose.ViolationError
	if errors.As(err, &ve) {
		return ve.Violations, true
	}
	return nil, false
}

// writeViolations writes a JSON error with the list of rule violations.
func writeViolations(w http.ResponseWriter, status int, msg string, violations []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error":      msg,
		"violations": violations,
	})
}

// loadComposeForEdit parses the app's current compose file before a label
// edit (endpoints, IP allowlist). When it cannot be parsed or fails the
// security checks, it writes a 409 and returns false: editing it would
// rewrite a file nobody can check.
func loadComposeForEdit(w http.ResponseWriter, app *store.App) (*compose.AppConfig, bool) {
	cfg, err := compose.ParseFile(app.ComposePath, app.Slug)
	if err != nil {
		log.Printf("[api] %s: load compose for edit: %v", app.Slug, err)
		violations, isViolation := violationsOf(err)
		switch {
		case isViolation:
			writeViolations(w, http.StatusConflict, unsafeComposeEdit, violations)
		case errors.Is(err, compose.ErrDotEnv):
			http.Error(w, dotEnvProblem, http.StatusConflict)
		case errors.Is(err, fsutil.ErrNotRegular):
			http.Error(w, "docker-compose.yml is a link or not a regular file; replace it with a regular file before changing these settings", http.StatusConflict)
		default:
			http.Error(w, "this app's compose file could not be read; fix it and redeploy before changing these settings", http.StatusConflict)
		}
		return nil, false
	}
	if refuseEditOnUnsafeCompose(w, cfg) {
		return nil, false
	}
	return cfg, true
}

// endpointName identifies an endpoint in messages by its domain (and path,
// when it has one).
func endpointName(ep compose.EndpointConfig) string {
	return ep.Domain + ep.Path
}

// reservedDomainList returns the reserved (dashboard) domains, sorted.
func (s *Server) reservedDomainList() []string {
	out := make([]string, 0, len(s.reservedDomains))
	for d := range s.reservedDomains {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// otherAppDomains maps each endpoint domain key used by an active app other
// than slug to that app's slug. Endpoints are read from each app's compose
// file (the store only keeps the primary domain, and the merged label table
// loses endpoints of all but one service); the stored primary domain is the
// fallback when the compose file cannot be parsed.
func (s *Server) otherAppDomains(slug string) (map[string]string, error) {
	apps, err := s.store.ListApps()
	if err != nil {
		return nil, err
	}
	owners := map[string]string{}
	claim := func(domain, owner string) {
		if k := endpointDomainKey(domain); k != "" {
			if _, ok := owners[k]; !ok {
				owners[k] = owner
			}
		}
	}
	for _, a := range apps {
		if a.Slug == slug {
			continue
		}
		var cfg *compose.AppConfig
		if a.ComposePath != "" {
			cfg, _ = parseComposeForDisplay(a.ComposePath, a.Slug)
		}
		if cfg == nil {
			claim(a.Domain, a.Slug)
			continue
		}
		for _, ep := range cfg.Endpoints {
			claim(ep.Domain, a.Slug)
		}
	}
	return owners, nil
}

// checkEndpointDomains rejects endpoints on a domain another app already
// uses and, for non-super_admin users, on a reserved (dashboard) domain or a
// wildcard domain. Domains in current (the app's endpoints before the
// change) are exempt from the super_admin-only rules, so an app a
// super_admin set up stays editable. Only super_admin learns which app owns
// a domain. Returns an HTTP status and message, or 0 when all is fine.
func (s *Server) checkEndpointDomains(r *http.Request, slug string, endpoints, current []compose.EndpointConfig) (int, string) {
	isSuper := false
	if u := GetAuthUser(r); u != nil && u.Role == "super_admin" {
		isSuper = true
	}
	// The dashboard domain is off limits to non-super_admin users. A
	// super_admin may route it through an app (the documented way to expose
	// the dashboard via Caddy). Wildcards can catch other apps' subdomains.
	if !isSuper {
		existing := map[string]bool{}
		for _, ep := range current {
			existing[endpointDomainKey(ep.Domain)] = true
		}
		for _, ep := range endpoints {
			key := endpointDomainKey(ep.Domain)
			if existing[key] {
				continue
			}
			if s.reservedDomains[key] {
				return http.StatusConflict, fmt.Sprintf("domain %s is reserved for the SimpleDeploy dashboard; pick another domain", ep.Domain)
			}
			if strings.Contains(ep.Domain, "*") {
				return http.StatusConflict, fmt.Sprintf("domain %s is a wildcard domain; only a super admin can add one", ep.Domain)
			}
		}
	}
	owners, err := s.otherAppDomains(slug)
	if err != nil {
		log.Printf("[api] endpoints %s: list apps: %v", slug, err)
		return http.StatusInternalServerError, "could not check which domains other apps use"
	}
	for _, ep := range endpoints {
		owner, taken := owners[endpointDomainKey(ep.Domain)]
		if !taken {
			continue
		}
		if isSuper {
			return http.StatusConflict, fmt.Sprintf("domain %s is already used by app %q; remove it there first or pick another domain", ep.Domain, owner)
		}
		return http.StatusConflict, fmt.Sprintf("domain %s is already used by another app; pick another domain", ep.Domain)
	}
	return 0, ""
}

func (s *Server) handleUpdateEndpoints(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	// Load old endpoints for before-snapshot.
	parsed, ok := loadComposeForEdit(w, app)
	if !ok {
		return
	}
	oldEndpoints := parsed.Endpoints

	var endpoints []compose.EndpointConfig
	if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate: each endpoint needs a service and a domain whose shape will
	// not break the proxy reload. Catching it here returns a per-endpoint
	// error instead of failing the whole batch when Caddy validates routes.
	seen := make(map[string]bool, len(endpoints))
	firstOnDomain := make(map[string]int, len(endpoints))
	for i := range endpoints {
		ep := &endpoints[i]
		ep.Protocol = compose.NormalizeProtocol(ep.Protocol)
		ep.Path = strings.TrimSpace(ep.Path)
		if ep.Domain == "" {
			http.Error(w, "every endpoint needs a domain", http.StatusBadRequest)
			return
		}
		if len(ep.Domain) > 253 || !validDomain.MatchString(ep.Domain) {
			http.Error(w, fmt.Sprintf("invalid domain %q", ep.Domain), http.StatusBadRequest)
			return
		}
		name := endpointName(*ep)
		if ep.Service == "" {
			http.Error(w, fmt.Sprintf("endpoint %s: service is required", name), http.StatusBadRequest)
			return
		}
		if !compose.ValidProtocol(ep.Protocol) {
			http.Error(w, fmt.Sprintf("endpoint %s: invalid protocol %q (want http, h2c or grpc)", name, ep.Protocol), http.StatusBadRequest)
			return
		}
		if ep.Path != "" && !compose.ValidPath(ep.Path) {
			http.Error(w, fmt.Sprintf("endpoint %s: invalid path %q", ep.Domain, ep.Path), http.StatusBadRequest)
			return
		}
		// Several endpoints may share a domain as long as their matchers
		// (grpc content-type, path) differ.
		key := compose.EndpointMatchKey(*ep)
		if seen[key] {
			http.Error(w, fmt.Sprintf("endpoint %s: duplicate domain %s (same path and protocol matcher)", name, ep.Domain), http.StatusBadRequest)
			return
		}
		seen[key] = true
		// Endpoints sharing a domain share one TLS setup, so their tls
		// modes must agree.
		if j, ok := firstOnDomain[ep.Domain]; ok {
			if a, b := compose.EffectiveTLS(ep.TLS), compose.EffectiveTLS(endpoints[j].TLS); a != b {
				http.Error(w, fmt.Sprintf("endpoint %s: tls %q conflicts with tls %q of endpoint %s; endpoints on one domain must use the same tls", name, a, b, endpointName(endpoints[j])), http.StatusBadRequest)
				return
			}
		} else {
			firstOnDomain[ep.Domain] = i
		}
	}

	// A domain belongs to one app: another app (or the dashboard) using it
	// would make the proxy drop one side's routes.
	if code, msg := s.checkEndpointDomains(r, slug, endpoints, oldEndpoints); code != 0 {
		http.Error(w, msg, code)
		return
	}

	if err := updateComposeEndpoints(app.ComposePath, endpoints); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	s.EnqueueGitCommit([]string{app.ComposePath}, "endpoints:"+slug)

	if s.reconciler != nil {
		// Endpoint changes are label-only: refresh proxy routes directly
		// without triggering a full compose redeploy (which would recreate
		// containers and block Caddy from dropping stale routes for 10s+).
		go func() { _ = s.reconciler.RefreshRoutes(context.Background()) }()
	}

	// Audit: one row per endpoint using added/removed/changed, keyed by
	// matcher (domain + grpc + path) so several endpoints per domain work.
	oldByKey := map[string]compose.EndpointConfig{}
	for _, ep := range oldEndpoints {
		oldByKey[compose.EndpointMatchKey(ep)] = ep
	}
	newByKey := map[string]compose.EndpointConfig{}
	for _, ep := range endpoints {
		newByKey[compose.EndpointMatchKey(ep)] = ep
	}
	appID := app.ID
	for key, newEP := range newByKey {
		ep := newEP
		if oldEP, existed := oldByKey[key]; existed {
			beforeJSON := endpointViewJSON(&oldEP)
			afterJSON := endpointViewJSON(&ep)
			_, _ = s.audit.Record(r.Context(), audit.RecordReq{
				Category: "endpoint",
				Action:   "changed",
				AppID:    &appID,
				AppSlug:  slug,
				Before:   beforeJSON,
				After:    afterJSON,
			})
		} else {
			afterJSON := endpointViewJSON(&ep)
			_, _ = s.audit.Record(r.Context(), audit.RecordReq{
				Category: "endpoint",
				Action:   "added",
				AppID:    &appID,
				AppSlug:  slug,
				After:    afterJSON,
			})
		}
	}
	for key, oldEP := range oldByKey {
		ep := oldEP
		if _, stillExists := newByKey[key]; !stillExists {
			beforeJSON := endpointViewJSON(&ep)
			_, _ = s.audit.Record(r.Context(), audit.RecordReq{
				Category: "endpoint",
				Action:   "removed",
				AppID:    &appID,
				AppSlug:  slug,
				Before:   beforeJSON,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "endpoints": endpoints})
}

// updateComposeEndpoints removes all existing endpoint labels and writes new ones.
func updateComposeEndpoints(composePath string, endpoints []compose.EndpointConfig) error {
	data, err := fsutil.ReadRegularFile(composePath)
	if err != nil {
		return fmt.Errorf("read compose: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("unexpected YAML structure")
	}
	root := doc.Content[0]

	servicesNode := findMapValue(root, "services")
	if servicesNode == nil {
		return fmt.Errorf("no services found in compose file")
	}
	if servicesNode.Kind != yaml.MappingNode || len(servicesNode.Content) < 2 {
		return fmt.Errorf("no services defined")
	}

	// Remove all existing endpoint labels from all services
	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcNode := servicesNode.Content[i]
		labelsNode := findMapValue(svcNode, "labels")
		if labelsNode == nil || labelsNode.Kind != yaml.MappingNode {
			continue
		}
		removeEndpointLabels(labelsNode)
	}

	// Group endpoints by service with per-service indexing
	byService := map[string][]indexedEndpoint{}
	svcIdx := map[string]int{}
	for _, ep := range endpoints {
		idx := svcIdx[ep.Service]
		svcIdx[ep.Service] = idx + 1
		byService[ep.Service] = append(byService[ep.Service], indexedEndpoint{index: idx, ep: ep})
	}

	// Add new endpoint labels to respective services
	for i := 0; i < len(servicesNode.Content)-1; i += 2 {
		svcName := servicesNode.Content[i].Value
		svcNode := servicesNode.Content[i+1]

		eps, ok := byService[svcName]
		if !ok {
			continue
		}

		labelsNode := findMapValue(svcNode, "labels")
		if labelsNode == nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "labels", Tag: "!!str"}
			labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			svcNode.Content = append(svcNode.Content, keyNode, labelsNode)
		}

		for _, ie := range eps {
			prefix := fmt.Sprintf("simpledeploy.endpoints.%d", ie.index)
			addLabel(labelsNode, prefix+".domain", ie.ep.Domain)
			if ie.ep.Port != "" {
				addLabel(labelsNode, prefix+".port", ie.ep.Port)
			}
			if ie.ep.TLS != "" {
				addLabel(labelsNode, prefix+".tls", ie.ep.TLS)
			}
			if ie.ep.Protocol != "" {
				addLabel(labelsNode, prefix+".protocol", ie.ep.Protocol)
			}
			if ie.ep.Path != "" {
				addLabel(labelsNode, prefix+".path", ie.ep.Path)
			}
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	return fsutil.WriteFileAtomic(composePath, out, 0o600)
}

type indexedEndpoint struct {
	index int
	ep    compose.EndpointConfig
}

func addLabel(labelsNode *yaml.Node, key, value string) {
	labelsNode.Content = append(labelsNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value, Tag: "!!str"},
	)
}

// removeEndpointLabels removes simpledeploy.endpoints.* keys from a labels mapping node.
func removeEndpointLabels(labelsNode *yaml.Node) {
	var filtered []*yaml.Node
	for i := 0; i < len(labelsNode.Content)-1; i += 2 {
		key := labelsNode.Content[i].Value
		if strings.HasPrefix(key, "simpledeploy.endpoints.") {
			continue
		}
		filtered = append(filtered, labelsNode.Content[i], labelsNode.Content[i+1])
	}
	labelsNode.Content = filtered
}

func splitLines(data []byte) []string {
	s := string(data)
	return strings.Split(s, "\n")
}

func joinLines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n"))
}

func findMapValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}
