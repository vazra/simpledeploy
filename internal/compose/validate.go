package compose

import (
	"fmt"
	"strings"
)

// dangerousVolumePaths contains host paths that must not be bind-mounted.
var dangerousVolumePaths = []string{
	"/etc",
	"/proc",
	"/sys",
	"/dev",
	"/var/run/docker.sock",
	"/root",
}

// ValidateComposeSecurity checks a parsed compose project for dangerous directives.
// Returns a list of violations. Empty list means safe.
func ValidateComposeSecurity(cfg *AppConfig) []string {
	var violations []string

	for _, svc := range cfg.Project.Services {
		name := svc.Name

		if svc.Privileged {
			violations = append(violations, fmt.Sprintf("service %q: privileged mode not allowed", name))
		}

		if svc.NetworkMode == "host" {
			violations = append(violations, fmt.Sprintf("service %q: network_mode 'host' not allowed", name))
		}

		if svc.Pid == "host" {
			violations = append(violations, fmt.Sprintf("service %q: pid mode 'host' not allowed", name))
		}

		if svc.Ipc != "" && svc.Ipc == "host" {
			violations = append(violations, fmt.Sprintf("service %q: ipc mode 'host' not allowed", name))
		}

		// Check dangerous capabilities
		if svc.CapAdd != nil {
			for _, cap := range svc.CapAdd {
				upper := strings.ToUpper(cap)
				if upper == "ALL" || upper == "SYS_ADMIN" || upper == "SYS_PTRACE" || upper == "NET_ADMIN" {
					violations = append(violations, fmt.Sprintf("service %q: dangerous capability %q not allowed", name, cap))
				}
			}
		}

		// Check dangerous volume mounts
		for _, vol := range svc.Volumes {
			if vol.Type != "bind" {
				continue
			}
			src := vol.Source
			for _, dangerous := range dangerousVolumePaths {
				if src == dangerous || strings.HasPrefix(src, dangerous+"/") {
					violations = append(violations, fmt.Sprintf("service %q: bind mount of %q not allowed", name, src))
				}
			}
			if strings.Contains(src, "..") {
				violations = append(violations, fmt.Sprintf("service %q: path traversal in volume source %q", name, src))
			}
		}
	}

	return violations
}
