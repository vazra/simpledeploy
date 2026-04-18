package compose

import (
	"bytes"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

var endpointLabelPrefixRe = regexp.MustCompile(`^simpledeploy\.endpoints\.\d+\.`)

// InjectSharedNetwork rewrites a compose YAML to declare the given shared
// external network and attach every service that has endpoint labels to it.
// Returns new bytes, whether any change was made, and an error.
func InjectSharedNetwork(yamlContent []byte, networkName string) ([]byte, bool, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(yamlContent, &root); err != nil {
		return yamlContent, false, fmt.Errorf("parse yaml: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return yamlContent, false, nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return yamlContent, false, nil
	}

	changed := false
	serviceChanged := false

	services := mappingValue(top, "services")
	if services != nil && services.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(services.Content); i += 2 {
			svc := services.Content[i+1]
			if svc.Kind != yaml.MappingNode {
				continue
			}
			if !serviceHasEndpointLabel(svc) {
				continue
			}
			if attachServiceNetwork(svc, networkName) {
				changed = true
				serviceChanged = true
			}
		}
	}

	// Only touch the top-level networks block when at least one service was
	// attached; otherwise the file is unaffected by this injection.
	if serviceChanged || anyServiceAttachedAlready(services, networkName) {
		if ensureTopLevelNetwork(top, networkName) {
			changed = true
		}
	}

	if !changed {
		return yamlContent, false, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return yamlContent, false, fmt.Errorf("encode yaml: %w", err)
	}
	enc.Close()
	out := buf.Bytes()
	if bytes.Equal(out, yamlContent) {
		return yamlContent, false, nil
	}
	return out, true, nil
}

// anyServiceAttachedAlready returns true if any service in the services
// mapping has the network listed in its networks block. Used to ensure the
// top-level declaration is present for idempotent runs where services were
// already attached in a prior pass.
func anyServiceAttachedAlready(services *yaml.Node, name string) bool {
	if services == nil || services.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		svc := services.Content[i+1]
		if svc.Kind != yaml.MappingNode {
			continue
		}
		nets := mappingValue(svc, "networks")
		if nets == nil {
			continue
		}
		switch nets.Kind {
		case yaml.SequenceNode:
			for _, item := range nets.Content {
				if item.Kind == yaml.ScalarNode && item.Value == name {
					return true
				}
			}
		case yaml.MappingNode:
			if mappingValue(nets, name) != nil {
				return true
			}
		}
	}
	return false
}

// ensureTopLevelNetwork adds networks.<name>: {external: true, name: <name>}
// if missing. Returns true if changed.
func ensureTopLevelNetwork(top *yaml.Node, name string) bool {
	nets := mappingValue(top, "networks")
	if nets == nil {
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: "networks"}
		val := &yaml.Node{Kind: yaml.MappingNode}
		top.Content = append(top.Content, key, val)
		nets = val
	}
	if nets.Kind != yaml.MappingNode {
		return false
	}
	if mappingValue(nets, name) != nil {
		return false
	}
	nets.Content = append(nets.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: name},
		&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "external"},
			{Kind: yaml.ScalarNode, Value: "true", Tag: "!!bool"},
			{Kind: yaml.ScalarNode, Value: "name"},
			{Kind: yaml.ScalarNode, Value: name},
		}},
	)
	return true
}

func serviceHasEndpointLabel(svc *yaml.Node) bool {
	labels := mappingValue(svc, "labels")
	if labels == nil {
		return false
	}
	switch labels.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(labels.Content); i += 2 {
			k := labels.Content[i]
			if k.Kind == yaml.ScalarNode && endpointLabelPrefixRe.MatchString(k.Value) {
				return true
			}
		}
	case yaml.SequenceNode:
		for _, item := range labels.Content {
			if item.Kind != yaml.ScalarNode {
				continue
			}
			key := item.Value
			if eq := indexOf(key, '='); eq >= 0 {
				key = key[:eq]
			}
			if endpointLabelPrefixRe.MatchString(key) {
				return true
			}
		}
	}
	return false
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// attachServiceNetwork appends the network name to the service's networks
// block. Handles missing/short-form/map-form variants. Returns true if changed.
func attachServiceNetwork(svc *yaml.Node, name string) bool {
	nets := mappingValue(svc, "networks")
	if nets == nil {
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: "networks"}
		val := &yaml.Node{
			Kind:    yaml.SequenceNode,
			Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: name}},
		}
		svc.Content = append(svc.Content, key, val)
		return true
	}
	switch nets.Kind {
	case yaml.SequenceNode:
		for _, item := range nets.Content {
			if item.Kind == yaml.ScalarNode && item.Value == name {
				return false
			}
		}
		nets.Content = append(nets.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: name})
		return true
	case yaml.MappingNode:
		if mappingValue(nets, name) != nil {
			return false
		}
		nets.Content = append(nets.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: name},
			&yaml.Node{Kind: yaml.MappingNode},
		)
		return true
	}
	return false
}

// mappingValue returns the value node for the given key in a mapping, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
