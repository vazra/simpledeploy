package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type accessRequest struct {
	Allow string `json:"allow"`
}

func (s *Server) handleUpdateAccess(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var req accessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Allow != "" {
		for _, entry := range strings.Split(req.Allow, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if strings.Contains(entry, "/") {
				if _, _, err := net.ParseCIDR(entry); err != nil {
					http.Error(w, fmt.Sprintf("invalid CIDR %q: %v", entry, err), http.StatusBadRequest)
					return
				}
			} else {
				if net.ParseIP(entry) == nil {
					http.Error(w, fmt.Sprintf("invalid IP %q", entry), http.StatusBadRequest)
					return
				}
			}
		}
	}

	if err := updateComposeAccessAllow(app.ComposePath, req.Allow); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	if s.reconciler != nil {
		go func() { _ = s.reconciler.Reconcile(context.Background()) }()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "allow": req.Allow})
}

func updateComposeAccessAllow(composePath, allow string) error {
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

	// Collect all access.allow value nodes across services
	var allowNodes []*yaml.Node
	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcNode := servicesNode.Content[i]
		labelsNode := findMapValue(svcNode, "labels")
		if labelsNode == nil {
			continue
		}
		av := findMapValue(labelsNode, "simpledeploy.access.allow")
		if av != nil {
			allowNodes = append(allowNodes, av)
		}
	}

	// If existing labels found, do surgical replacement preserving formatting
	if len(allowNodes) > 0 {
		lines := splitLines(data)
		for _, node := range allowNodes {
			lineIdx := node.Line - 1
			if lineIdx < 0 || lineIdx >= len(lines) {
				continue
			}
			lines[lineIdx] = replaceAllowValue(lines[lineIdx], node.Value, allow)
		}
		return os.WriteFile(composePath, joinLines(lines), 0644)
	}

	// No existing label: no-op if allow is empty
	if allow == "" {
		return nil
	}

	// Add to first service
	firstServiceNode := servicesNode.Content[1]
	labelsNode := findMapValue(firstServiceNode, "labels")
	if labelsNode == nil {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "labels", Tag: "!!str"}
		labelsNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		firstServiceNode.Content = append(firstServiceNode.Content, keyNode, labelsNode)
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "simpledeploy.access.allow", Tag: "!!str"}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: allow, Tag: "!!str"}
	labelsNode.Content = append(labelsNode.Content, keyNode, valNode)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	return os.WriteFile(composePath, out, 0644)
}

// replaceAllowValue replaces the old allow value on a YAML line, preserving
// quoting style and surrounding formatting.
func replaceAllowValue(line string, oldVal, newVal string) string {
	for _, pattern := range []string{
		fmt.Sprintf(`"%s"`, oldVal),
		fmt.Sprintf(`'%s'`, oldVal),
		oldVal,
	} {
		if idx := strings.Index(line, pattern); idx >= 0 {
			replacement := newVal
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
