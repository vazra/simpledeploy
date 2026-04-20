package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
	"gopkg.in/yaml.v3"
)

func (s *Server) handleUpdateEndpoints(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var endpoints []compose.EndpointConfig
	if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate: each endpoint needs at least a domain
	for i, ep := range endpoints {
		if ep.Domain == "" {
			http.Error(w, fmt.Sprintf("endpoint %d: domain is required", i), http.StatusBadRequest)
			return
		}
		if ep.Service == "" {
			http.Error(w, fmt.Sprintf("endpoint %d: service is required", i), http.StatusBadRequest)
			return
		}
	}

	if err := updateComposeEndpoints(app.ComposePath, endpoints); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	s.EnqueueGitCommit([]string{app.ComposePath}, "endpoints:"+slug)

	if s.reconciler != nil {
		go func() { _ = s.reconciler.Reconcile(context.Background()) }()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "endpoints": endpoints})
}

// updateComposeEndpoints removes all existing endpoint labels and writes new ones.
func updateComposeEndpoints(composePath string, endpoints []compose.EndpointConfig) error {
	data, err := os.ReadFile(composePath)
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
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	return os.WriteFile(composePath, out, 0644)
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
