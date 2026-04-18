// Package deployment detects how the simpledeploy server binary was launched
// (native vs containerized) so the API can surface mode-appropriate guidance.
package deployment

import "os"

type Mode string

const (
	ModeNative        Mode = "native"
	ModeDocker        Mode = "docker"
	ModeDockerDesktop Mode = "docker-desktop"
	ModeDockerDev     Mode = "docker-dev"
)

// Label returns the short UI-facing string for a mode, or "" if unknown.
func (m Mode) Label() string {
	switch m {
	case ModeNative:
		return "Native"
	case ModeDocker:
		return "Docker"
	case ModeDockerDesktop:
		return "Desktop"
	case ModeDockerDev:
		return "Dev"
	}
	return ""
}

type detectConfig struct {
	dockerenvPath string
	env           func(string) string
}

func detect(c detectConfig) Mode {
	if _, err := os.Stat(c.dockerenvPath); err != nil {
		return ModeNative
	}
	if c.env("SIMPLEDEPLOY_DEV_MODE") == "1" {
		return ModeDockerDev
	}
	if c.env("SIMPLEDEPLOY_UPSTREAM_HOST") != "" {
		return ModeDockerDesktop
	}
	return ModeDocker
}

// Detect probes the runtime environment once and returns the mode.
func Detect() Mode {
	return detect(detectConfig{
		dockerenvPath: "/.dockerenv",
		env:           os.Getenv,
	})
}
