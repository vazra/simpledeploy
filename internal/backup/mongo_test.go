package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestMongoStrategy_Type(t *testing.T) {
	s := NewMongoStrategy()
	if s.Type() != "mongo" {
		t.Errorf("expected mongo, got %s", s.Type())
	}
}

func TestMongoStrategy_Detect(t *testing.T) {
	s := NewMongoStrategy()

	tests := []struct {
		name    string
		cfg     *compose.AppConfig
		wantLen int
	}{
		{
			name: "mongo image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "mongo:7"},
				},
			},
			wantLen: 1,
		},
		{
			name: "mongo image with registry",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "registry.example.com/mongo:6"},
				},
			},
			wantLen: 1,
		},
		{
			name: "label override",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "custom:latest", Labels: map[string]string{
						"simpledeploy.backup.strategy": "mongo",
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
			name: "mysql is not mongo",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "mysql:8"},
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
			if tt.wantLen > 0 && got[0].Label != "mongo" {
				t.Errorf("expected label mongo, got %s", got[0].Label)
			}
		})
	}
}

func TestMongoStrategy_Interface(t *testing.T) {
	var _ Strategy = NewMongoStrategy()
}
