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

func TestSQLiteStrategy_Type(t *testing.T) {
	s := NewSQLiteStrategy()
	if s.Type() != "sqlite" {
		t.Errorf("expected sqlite, got %s", s.Type())
	}
}

func TestSQLiteStrategy_Detect(t *testing.T) {
	s := NewSQLiteStrategy()

	tests := []struct {
		name      string
		cfg       *compose.AppConfig
		wantLen   int
		wantPaths int
	}{
		{
			name: "label match with volumes",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{
						Name:  "app",
						Image: "myapp:latest",
						Labels: map[string]string{
							"simpledeploy.backup.strategy": "sqlite",
						},
						Volumes: []compose.VolumeMount{
							{Source: "data", Target: "/app/data", Type: "volume"},
							{Source: "./config", Target: "/app/config", Type: "bind"},
						},
					},
				},
			},
			wantLen:   1,
			wantPaths: 2,
		},
		{
			name: "label match no volumes",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{
						Name:  "app",
						Image: "myapp:latest",
						Labels: map[string]string{
							"simpledeploy.backup.strategy": "sqlite",
						},
					},
				},
			},
			wantLen:   1,
			wantPaths: 0,
		},
		{
			name: "no label - no detection even with sqlite image",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{Name: "app", Image: "sqlite:latest"},
				},
			},
			wantLen: 0,
		},
		{
			name: "wrong label",
			cfg: &compose.AppConfig{
				Name: "myapp",
				Services: []compose.ServiceConfig{
					{
						Name:  "db",
						Image: "postgres:16",
						Labels: map[string]string{
							"simpledeploy.backup.strategy": "postgres",
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
				if got[0].Label != "sqlite" {
					t.Errorf("expected label sqlite, got %s", got[0].Label)
				}
				if len(got[0].Paths) != tt.wantPaths {
					t.Errorf("expected %d paths, got %d", tt.wantPaths, len(got[0].Paths))
				}
			}
		})
	}
}

func TestSQLiteStrategy_Interface(t *testing.T) {
	var _ Strategy = NewSQLiteStrategy()
}

func TestSQLiteStrategy_RejectsUnsafePaths(t *testing.T) {
	s := NewSQLiteStrategy()
	for _, p := range []string{"/data/x'.db", "/data/x\".db", "/data/x\n.db", "relative.db", "/data/-x.db"} {
		if _, err := s.Backup(context.Background(), BackupOpts{ContainerName: noSuchContainer, Paths: []string{p}}); err == nil {
			t.Errorf("Backup(%q) succeeded, want path error", p)
		}
		data := makeTarGz(t, []*tar.Header{{Name: "tmp/sd-backup-x.db", Size: 1, Typeflag: tar.TypeReg}})
		err := s.Restore(context.Background(), RestoreOpts{
			ContainerName: noSuchContainer,
			Paths:         []string{p},
			Reader:        io.NopCloser(bytes.NewReader(data)),
		})
		if err == nil || !strings.Contains(err.Error(), "path") {
			t.Errorf("Restore(%q) err = %v, want path error", p, err)
		}
	}
}

func TestSQLiteStrategy_RestoreRejectsOversizeArchive(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{{Name: "tmp/sd-backup-app.db", Size: 64 << 10, Typeflag: tar.TypeReg}})
	err := NewSQLiteStrategy().Restore(context.Background(), RestoreOpts{
		ContainerName:        noSuchContainer,
		Paths:                []string{"/data/app.db"},
		Reader:               io.NopCloser(bytes.NewReader(data)),
		MaxDecompressedBytes: 16 << 10,
	})
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("expected errArchiveTooLarge, got %v", err)
	}
}
