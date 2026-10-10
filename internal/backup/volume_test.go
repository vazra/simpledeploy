package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
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

func TestVolumeStrategy_DetectCleansPaths(t *testing.T) {
	cfg := &compose.AppConfig{
		Name: "myapp",
		Services: []compose.ServiceConfig{{
			Name: "app",
			Volumes: []compose.VolumeMount{
				{Source: "data", Target: "/data/", Type: "volume"},
				{Source: "./cfg", Target: "/etc//app", Type: "bind"},
			},
		}},
	}
	got := NewVolumeStrategy().Detect(cfg)
	if len(got) != 1 {
		t.Fatalf("detected %d services, want 1", len(got))
	}
	want := []string{"/data", "/etc/app"}
	for i, p := range got[0].Paths {
		if p != want[i] {
			t.Errorf("path %d = %q, want %q", i, p, want[i])
		}
	}
	if err := ValidatePaths("volume", got[0].Paths); err != nil {
		t.Errorf("detected paths fail validation: %v", err)
	}
}

func TestVolumeStrategy_Interface(t *testing.T) {
	var _ Strategy = NewVolumeStrategy()
}

// The restore guards below fail before any docker command runs, so these
// tests need no docker daemon. The container name is one that cannot exist.
const noSuchContainer = "sd-test-no-such-container"

func TestVolumeStrategy_BackupRejectsUnsafePaths(t *testing.T) {
	s := NewVolumeStrategy()
	for _, p := range []string{"relative/dir", "/data/-rf", "/data\n/etc"} {
		_, err := s.Backup(context.Background(), BackupOpts{ContainerName: noSuchContainer, Paths: []string{p}})
		if err == nil || !strings.Contains(err.Error(), "backup path") {
			t.Errorf("Backup(%q) err = %v, want backup path error", p, err)
		}
	}
}

func TestVolumeStrategy_RestoreRejectsOversizeArchive(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{{Name: "data/big.bin", Size: 64 << 10, Typeflag: tar.TypeReg}})
	err := NewVolumeStrategy().Restore(context.Background(), RestoreOpts{
		ContainerName:        noSuchContainer,
		Reader:               io.NopCloser(bytes.NewReader(data)),
		MaxDecompressedBytes: 16 << 10,
	})
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("expected errArchiveTooLarge, got %v", err)
	}
}
