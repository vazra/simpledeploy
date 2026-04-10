package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

type domainRequest struct {
	Domain string `json:"domain"`
}

func (s *Server) handleUpdateDomain(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var req domainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	if err := updateComposeDomain(app.ComposePath, req.Domain); err != nil {
		http.Error(w, fmt.Sprintf("update compose: %v", err), http.StatusInternalServerError)
		return
	}

	if s.reconciler != nil {
		go s.reconciler.Reconcile(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "domain": req.Domain})
}

func updateComposeDomain(composePath, domain string) error {
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

	firstServiceNode := servicesNode.Content[1]

	labelsNode := findMapValue(firstServiceNode, "labels")
	if labelsNode == nil {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "labels", Tag: "!!str"}
		labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		firstServiceNode.Content = append(firstServiceNode.Content, keyNode, labelsNode)
	}

	domainValueNode := findMapValue(labelsNode, "simpledeploy.domain")
	if domainValueNode != nil {
		domainValueNode.Value = domain
	} else {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "simpledeploy.domain", Tag: "!!str"}
		valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: domain, Tag: "!!str"}
		labelsNode.Content = append(labelsNode.Content, keyNode, valNode)
	}

	for i := 2; i < len(servicesNode.Content); i += 2 {
		if i+1 >= len(servicesNode.Content) {
			break
		}
		svcNode := servicesNode.Content[i+1]
		svcLabels := findMapValue(svcNode, "labels")
		if svcLabels == nil {
			continue
		}
		dv := findMapValue(svcLabels, "simpledeploy.domain")
		if dv != nil {
			dv.Value = domain
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	return os.WriteFile(composePath, out, 0644)
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
