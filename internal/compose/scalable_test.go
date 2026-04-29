package compose

import (
	"strings"
	"testing"
)

func TestScaleEligibility(t *testing.T) {
	cases := []struct {
		name      string
		svc       ServiceConfig
		scalable  bool
		reasonSub string
	}{
		{
			name:     "plain web image",
			svc:      ServiceConfig{Name: "web", Image: "nginx:1.25"},
			scalable: true,
		},
		{
			name:      "host port binding",
			svc:       ServiceConfig{Name: "web", Image: "nginx", Ports: []PortMapping{{Host: "8080", Container: "80"}}},
			scalable:  false,
			reasonSub: "host port",
		},
		{
			name:      "named volume",
			svc:       ServiceConfig{Name: "data", Image: "myapp", Volumes: []VolumeMount{{Source: "appdata", Target: "/var/lib/app", Type: "volume"}}},
			scalable:  false,
			reasonSub: "persistent volume",
		},
		{
			name:     "bind mount is fine",
			svc:      ServiceConfig{Name: "web", Image: "nginx", Volumes: []VolumeMount{{Source: "./html", Target: "/usr/share/nginx/html", Type: "bind"}}},
			scalable: true,
		},
		{
			name:      "postgres image",
			svc:       ServiceConfig{Name: "db", Image: "postgres:16"},
			scalable:  false,
			reasonSub: "stateful",
		},
		{
			name:      "bitnami postgresql",
			svc:       ServiceConfig{Name: "db", Image: "bitnami/postgresql:15"},
			scalable:  false,
			reasonSub: "stateful",
		},
		{
			name:      "redis image",
			svc:       ServiceConfig{Name: "cache", Image: "redis:7-alpine"},
			scalable:  false,
			reasonSub: "stateful",
		},
		{
			name:      "deploy mode global",
			svc:       ServiceConfig{Name: "agent", Image: "myagent", DeployMode: "global"},
			scalable:  false,
			reasonSub: "global",
		},
		{
			name:      "label opt-out wins",
			svc:       ServiceConfig{Name: "web", Image: "nginx", Labels: map[string]string{"simpledeploy.scalable": "false"}},
			scalable:  false,
			reasonSub: "non-scalable",
		},
		{
			name:     "label opt-in overrides stateful image",
			svc:      ServiceConfig{Name: "weird", Image: "postgres:16", Labels: map[string]string{"simpledeploy.scalable": "true"}},
			scalable: true,
		},
		{
			name:     "exporter sidecar is not flagged stateful",
			svc:      ServiceConfig{Name: "metrics", Image: "prometheuscommunity/postgres-exporter:v0.15"},
			scalable: true,
		},
		{
			name:     "node-exporter is not flagged stateful",
			svc:      ServiceConfig{Name: "node", Image: "prom/node-exporter:latest"},
			scalable: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := tc.svc.ScaleEligibility()
			if ok != tc.scalable {
				t.Fatalf("scalable=%v reason=%q want=%v", ok, reason, tc.scalable)
			}
			if !tc.scalable && tc.reasonSub != "" && !strings.Contains(reason, tc.reasonSub) {
				t.Fatalf("reason=%q want substring %q", reason, tc.reasonSub)
			}
		})
	}
}
