package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestMySQLStrategy_Type(t *testing.T) {
	s := NewMySQLStrategy()
	if s.Type() != "mysql" {
		t.Errorf("expected mysql, got %s", s.Type())
	}
}

func TestMySQLStrategy_Detect(t *testing.T) {
	s := NewMySQLStrategy()

	tests := []struct {
		name    string
		cfg     *compose.AppConfig
		wantLen int
	}{
		{
			name: "mysql image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "mysql:8"},
				},
			},
			wantLen: 1,
		},
		{
			name: "mariadb image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "mariadb:11"},
				},
			},
			wantLen: 1,
		},
		{
			name: "percona image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "db", Image: "percona/percona-server:8.0"},
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
						"simpledeploy.backup.strategy": "mysql",
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
			if tt.wantLen > 0 && got[0].Label != "mysql" {
				t.Errorf("expected label mysql, got %s", got[0].Label)
			}
		})
	}
}

func TestMySQLStrategy_Interface(t *testing.T) {
	var _ Strategy = NewMySQLStrategy()
}
