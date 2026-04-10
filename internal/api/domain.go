package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

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

	// Collect all domain value nodes across services
	var domainNodes []*yaml.Node
	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcNode := servicesNode.Content[i]
		labelsNode := findMapValue(svcNode, "labels")
		if labelsNode == nil {
			continue
		}
		dv := findMapValue(labelsNode, "simpledeploy.domain")
		if dv != nil {
			domainNodes = append(domainNodes, dv)
		}
	}

	// If existing labels found, do surgical replacement preserving formatting
	if len(domainNodes) > 0 {
		lines := splitLines(data)
		for _, node := range domainNodes {
			lineIdx := node.Line - 1 // yaml.Node.Line is 1-based
			if lineIdx < 0 || lineIdx >= len(lines) {
				continue
			}
			lines[lineIdx] = replaceDomainValue(lines[lineIdx], node.Value, domain)
		}
		return os.WriteFile(composePath, joinLines(lines), 0644)
	}

	// No existing label: add to first service via full marshal (unavoidable)
	firstServiceNode := servicesNode.Content[1]
	labelsNode := findMapValue(firstServiceNode, "labels")
	if labelsNode == nil {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "labels", Tag: "!!str"}
		labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		firstServiceNode.Content = append(firstServiceNode.Content, keyNode, labelsNode)
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "simpledeploy.domain", Tag: "!!str"}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: domain, Tag: "!!str"}
	labelsNode.Content = append(labelsNode.Content, keyNode, valNode)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	return os.WriteFile(composePath, out, 0644)
}

// replaceDomainValue replaces the old domain value on a YAML line, preserving
// quoting style and surrounding formatting.
func replaceDomainValue(line string, oldVal, newVal string) string {
	// Try quoted variants first, then unquoted
	for _, pattern := range []string{
		fmt.Sprintf(`"%s"`, oldVal),
		fmt.Sprintf(`'%s'`, oldVal),
		oldVal,
	} {
		if idx := strings.Index(line, pattern); idx >= 0 {
			replacement := newVal
			// Preserve original quoting
			if strings.HasPrefix(pattern, `"`) {
				replacement = fmt.Sprintf(`"%s"`, newVal)
			} else if strings.HasPrefix(pattern, `'`) {
				replacement = fmt.Sprintf(`'%s'`, newVal)
			}
			return line[:idx] + replacement + line[idx+len(pattern):]
		}
	}
	return line
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
