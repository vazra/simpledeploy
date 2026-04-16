package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestPostgresStrategy_Type(t *testing.T) {
	s := NewPostgresStrategy()
	if s.Type() != "postgres" {
		t.Errorf("expected postgres, got %s", s.Type())
	}
}

func TestPostgresStrategy_Detect(t *testing.T) {
	s := NewPostgresStrategy()

	tests := []struct {
		name    string
		cfg     *compose.AppConfig
		wantLen int
	}{
		{
			name: "postgres image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "postgres:16"},
				},
			},
			wantLen: 1,
		},
		{
			name: "postgis image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "postgis/postgis:16-3.4"},
				},
			},
			wantLen: 1,
		},
		{
			name: "timescale image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "timescale/timescaledb:latest"},
				},
			},
			wantLen: 1,
		},
		{
			name: "supabase image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "supabase/postgres:15"},
				},
			},
			wantLen: 1,
		},
		{
			name: "label override",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "custom-db:latest", Labels: map[string]string{
						"simpledeploy.backup.strategy": "postgres",
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
		{
			name: "multiple services mixed",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "web", Image: "nginx:latest"},
					{Name: "db", Image: "postgres:16"},
					{Name: "cache", Image: "redis:7"},
				},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Detect(tt.cfg)
			if len(got) != tt.wantLen {
				t.Errorf("Detect() returned %d results, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 {
				if got[0].Label != "postgres" {
					t.Errorf("expected label postgres, got %s", got[0].Label)
				}
				if got[0].ContainerName != tt.cfg.Name+"-"+got[0].ServiceName+"-1" {
					t.Errorf("unexpected container name: %s", got[0].ContainerName)
				}
			}
		})
	}
}

func TestPostgresStrategy_Interface(t *testing.T) {
	var _ Strategy = NewPostgresStrategy()
}
