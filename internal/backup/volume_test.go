package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestVolumeStrategy_Type(t *testing.T) {
	s := NewVolumeStrategy()
	if s.Type() != "volume" {
		t.Errorf("expected volume, got %s", s.Type())
	}
}

func TestVolumeStrategy_Detect(t *testing.T) {
	s := NewVolumeStrategy()

	tests := []struct {
		name      string
		cfg       *compose.AppConfig
		wantLen   int
		wantPaths int
	}{
		{
			name: "label override",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{
						Name:  "app",
						Image: "myapp:latest",
						Labels: map[string]string{
							"simpledeploy.backup.strategy": "volume",
						},
						Volumes: []compose.VolumeMount{
							{Source: "data", Target: "/data", Type: "volume"},
						},
					},
				},
			},
			wantLen:   1,
			wantPaths: 1,
		},
		{
			name: "auto-detect from volumes",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{
						Name:  "app",
						Image: "myapp:latest",
						Volumes: []compose.VolumeMount{
							{Source: "data", Target: "/app/data", Type: "volume"},
							{Source: "uploads", Target: "/app/uploads", Type: "volume"},
						},
					},
				},
			},
			wantLen:   1,
			wantPaths: 2,
		},
		{
			name: "excludes docker.sock",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{
						Name:  "app",
						Image: "myapp:latest",
						Volumes: []compose.VolumeMount{
							{Source: "/var/run/docker.sock", Target: "/var/run/docker.sock", Type: "bind"},
							{Source: "data", Target: "/data", Type: "volume"},
						},
					},
				},
			},
			wantLen:   1,
			wantPaths: 1,
		},
		{
			name: "no volumes - no detection",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "web", Image: "nginx:latest"},
				},
			},
			wantLen: 0,
		},
		{
			name: "only docker.sock - no detection",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{
						Name:  "app",
						Image: "myapp:latest",
						Volumes: []compose.VolumeMount{
							{Source: "/var/run/docker.sock", Target: "/var/run/docker.sock", Type: "bind"},
						},
					},
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
			if tt.wantLen > 0 {
				if got[0].Label != "volume" {
					t.Errorf("expected label volume, got %s", got[0].Label)
				}
				if len(got[0].Paths) != tt.wantPaths {
					t.Errorf("expected %d paths, got %d", tt.wantPaths, len(got[0].Paths))
				}
			}
		})
	}
}

func TestVolumeStrategy_Interface(t *testing.T) {
	var _ Strategy = NewVolumeStrategy()
}
