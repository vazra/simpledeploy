package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestRedisStrategy_Type(t *testing.T) {
	s := NewRedisStrategy()
	if s.Type() != "redis" {
		t.Errorf("expected redis, got %s", s.Type())
	}
}

func TestRedisStrategy_Detect(t *testing.T) {
	s := NewRedisStrategy()

	tests := []struct {
		name    string
		cfg     *compose.AppConfig
		wantLen int
	}{
		{
			name: "redis image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "cache", Image: "redis:7-alpine"},
				},
			},
			wantLen: 1,
		},
		{
			name: "valkey image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "cache", Image: "valkey/valkey:latest"},
				},
			},
			wantLen: 1,
		},
		{
			name: "dragonfly image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "cache", Image: "docker.dragonflydb.io/dragonflydb/dragonfly:latest"},
				},
			},
			wantLen: 1,
		},
		{
			name: "label override",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "kv", Image: "custom:latest", Labels: map[string]string{
						"simpledeploy.backup.strategy": "redis",
					}},
				},
			},
			wantLen: 1,
		},
		{
			name: "no match",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "web", Image: "nginx:latest"},
				},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Detect(tt.cfg)
			if len(got) != tt.wantLen {
				t.Errorf("Detect() returned %d results, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[0].Label != "redis" {
				t.Errorf("expected label redis, got %s", got[0].Label)
			}
		})
	}
}

func TestRedisStrategy_Interface(t *testing.T) {
	var _ Strategy = NewRedisStrategy()
}
