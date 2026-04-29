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
	"/var/lib/docker",
	"/var/lib/kubelet",
	"/boot",
	"/lib/modules",
	"/run",
}

// dangerousCaps lists Linux capabilities that grant container-escape or
// host-read primitives. Allow list approach is preferable but breaks too
// many legitimate compose files; this deny-list captures the worst cases.
var dangerousCaps = map[string]struct{}{
	"ALL":             {},
	"SYS_ADMIN":       {},
	"SYS_PTRACE":      {},
	"SYS_MODULE":      {},
	"SYS_RAWIO":       {},
	"SYS_BOOT":        {},
	"SYS_TIME":        {},
	"NET_ADMIN":       {},
	"NET_RAW":         {},
	"DAC_READ_SEARCH": {},
	"DAC_OVERRIDE":    {},
	"BPF":             {},
	"PERFMON":         {},
	"MKNOD":           {},
}

// dangerousSecurityOpts disable mandatory access control or seccomp.
var dangerousSecurityOpts = []string{
	"apparmor=unconfined",
	"seccomp=unconfined",
	"label=disable",
	"systempaths=unconfined",
	"no-new-privileges=false",
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
		for _, cap := range svc.CapAdd {
			upper := strings.ToUpper(strings.TrimPrefix(cap, "CAP_"))
			if _, bad := dangerousCaps[upper]; bad {
				violations = append(violations, fmt.Sprintf("service %q: dangerous capability %q not allowed", name, cap))
			}
		}

		// Check security_opt: disabling apparmor/seccomp/etc. negates host
		// isolation regardless of other compose hardening.
		for _, opt := range svc.SecurityOpt {
			normalized := strings.ToLower(strings.ReplaceAll(opt, " ", ""))
			for _, bad := range dangerousSecurityOpts {
				if normalized == bad {
					violations = append(violations, fmt.Sprintf("service %q: security_opt %q not allowed", name, opt))
				}
			}
		}

		// userns_mode/cgroup/runtime: only "host" / non-empty values that
		// punch through namespace isolation are flagged. Empty is the
		// default (isolated) and stays allowed.
		if strings.EqualFold(svc.UserNSMode, "host") {
			violations = append(violations, fmt.Sprintf("service %q: userns_mode 'host' not allowed", name))
		}
		if strings.EqualFold(svc.Cgroup, "host") {
			violations = append(violations, fmt.Sprintf("service %q: cgroup 'host' not allowed", name))
		}

		// devices: bind a host /dev path into the container.
		if len(svc.Devices) > 0 {
			violations = append(violations, fmt.Sprintf("service %q: 'devices' not allowed", name))
		}

		// volumes_from imports another (possibly privileged) container's
		// volumes wholesale.
		if len(svc.VolumesFrom) > 0 {
			violations = append(violations, fmt.Sprintf("service %q: 'volumes_from' not allowed", name))
		}

		// pid: anything except empty (default) is unsafe in shared-host
		// scenarios. We already block "host"; also block "container:" and
		// "service:" forms which let the service inspect/signal arbitrary
		// neighbors.
		if svc.Pid != "" && svc.Pid != "host" {
			violations = append(violations, fmt.Sprintf("service %q: pid %q not allowed", name, svc.Pid))
		}

		// Check dangerous volume mounts. Treat both type=bind and
		// type=volume-with-driver-opts(type=none, device=/host/path) as
		// host binds.
		for _, vol := range svc.Volumes {
			src := vol.Source
			isBind := vol.Type == "bind"
			// Some compose loaders emit "" as default for bind, others "bind".
			// If we can identify the volume-with-host-bind shim via the
			// VolumeOptions or the source containing "/", treat as bind.
			if !isBind && strings.HasPrefix(src, "/") {
				isBind = true
			}
			if !isBind {
				continue
			}
			for _, dangerous := range dangerousVolumePaths {
				if src == dangerous || strings.HasPrefix(src, dangerous+"/") {
					violations = append(violations, fmt.Sprintf("service %q: bind mount of %q not allowed", name, src))
				}
			}
			if strings.Contains(src, "..") {
				violations = append(violations, fmt.Sprintf("service %q: path traversal in volume source %q", name, src))
			}
			if src == "/" {
				violations = append(violations, fmt.Sprintf("service %q: bind mount of root filesystem not allowed", name))
			}
		}
	}

	// Volumes with driver_opts that resolve to a host bind (type=none + o=bind
	// + device=/path) are effectively bind mounts but Service.Volumes records
	// them as Type=volume. Inspect Project.Volumes for the same shim.
	for vname, v := range cfg.Project.Volumes {
		if v.Driver != "" && v.Driver != "local" {
			continue
		}
		opts := v.DriverOpts
		if opts == nil {
			continue
		}
		if strings.EqualFold(opts["type"], "none") || strings.Contains(strings.ToLower(opts["o"]), "bind") {
			device := opts["device"]
			if device == "" {
				continue
			}
			for _, dangerous := range dangerousVolumePaths {
				if device == dangerous || strings.HasPrefix(device, dangerous+"/") {
					violations = append(violations, fmt.Sprintf("volume %q: host-bind shim into %q not allowed", vname, device))
				}
			}
			if device == "/" {
				violations = append(violations, fmt.Sprintf("volume %q: host-bind shim into / not allowed", vname))
			}
			if strings.Contains(device, "..") {
				violations = append(violations, fmt.Sprintf("volume %q: host-bind shim path traversal in %q", vname, device))
			}
		}
	}

	return violations
}
