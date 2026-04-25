package api

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// composeAuditView parses yamlText in-memory and returns a JSON snapshot
// containing only the fields the audit renderer needs (image, env, ports,
// replicas, labels per service). Returns nil on empty input or parse error.
func composeAuditView(yamlText string) []byte {
	if yamlText == "" {
		return nil
	}
	var raw struct {
		Services map[string]struct {
			Image       string   `yaml:"image"`
			Environment any      `yaml:"environment"`
			Ports       []string `yaml:"ports"`
			Deploy      struct {
				Replicas int `yaml:"replicas"`
			} `yaml:"deploy"`
			Labels map[string]string `yaml:"labels"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(yamlText), &raw); err != nil {
		return nil
	}
	services := map[string]any{}
	for name, svc := range raw.Services {
		env := map[string]string{}
		switch v := svc.Environment.(type) {
		case map[string]any:
			for k, val := range v {
				env[k] = composeValToString(val)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					for i := 0; i < len(s); i++ {
						if s[i] == '=' {
							env[s[:i]] = s[i+1:]
							break
						}
					}
				}
			}
		}
		services[name] = map[string]any{
			"image":    svc.Image,
			"env":      env,
			"ports":    svc.Ports,
			"replicas": svc.Deploy.Replicas,
			"labels":   svc.Labels,
		}
	}
	out, _ := json.Marshal(map[string]any{"services": services})
	return out
}

func composeValToString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
