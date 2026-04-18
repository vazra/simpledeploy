package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tmp := t.TempDir()
	dockerenv := filepath.Join(tmp, ".dockerenv")
	if err := os.WriteFile(dockerenv, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(tmp, "does-not-exist")

	tests := []struct {
		name     string
		path     string
		dev      string
		upstream string
		want     Mode
	}{
		{"native_no_dockerenv", missing, "", "", ModeNative},
		{"docker_linux_host", dockerenv, "", "", ModeDocker},
		{"docker_desktop", dockerenv, "", "host.docker.internal", ModeDockerDesktop},
		{"docker_dev_wins_over_desktop", dockerenv, "1", "host.docker.internal", ModeDockerDev},
		{"docker_dev_alone", dockerenv, "1", "", ModeDockerDev},
		{"docker_desktop_empty_dev", dockerenv, "", "host.docker.internal", ModeDockerDesktop},
		{"dev_mode_zero_ignored", dockerenv, "0", "host.docker.internal", ModeDockerDesktop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := func(k string) string {
				switch k {
				case "SIMPLEDEPLOY_DEV_MODE":
					return tt.dev
				case "SIMPLEDEPLOY_UPSTREAM_HOST":
					return tt.upstream
				}
				return ""
			}
			got := detect(detectConfig{dockerenvPath: tt.path, env: env})
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLabel(t *testing.T) {
	cases := map[Mode]string{
		ModeNative:        "Native",
		ModeDocker:        "Docker",
		ModeDockerDesktop: "Desktop",
		ModeDockerDev:     "Dev",
		Mode("unknown"):   "",
	}
	for m, want := range cases {
		if got := m.Label(); got != want {
			t.Errorf("%s: got %q, want %q", m, got, want)
		}
	}
}
