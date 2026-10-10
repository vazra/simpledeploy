package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
)

// systemPaths are host folders a bind mount may not touch. A bind is
// refused when its source is one of these, is inside one, or is a parent
// of one (binding /var would expose /var/run/docker.sock). The app's own
// folder is always allowed, even when apps_dir sits under one of these.
var systemPaths = []string{
	"/bin",
	"/boot",
	"/dev",
	"/etc",
	"/home",
	"/lib",
	"/lib32",
	"/lib64",
	"/libx32",
	"/proc",
	"/root",
	"/run",
	"/sbin",
	"/snap",
	"/sys",
	"/usr",
	"/var/backups",
	"/var/lib",
	"/var/log",
	"/var/mail",
	"/var/run",
	"/var/spool",
}

// readOnlyPaths are system folders that may be bind-mounted read-only
// (log shippers and monitoring agents commonly read /var/log). Writable
// binds of them are still refused.
var readOnlyPaths = []string{
	"/var/log",
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

// networkVolumeTypes are local-driver mount types that never reach a host
// path: remote shares and memory-backed tmpfs.
var networkVolumeTypes = map[string]struct{}{
	"nfs":   {},
	"nfs4":  {},
	"cifs":  {},
	"smb3":  {},
	"tmpfs": {},
}

// protected holds SimpleDeploy's own folders and the operator's allowed
// host folders, set once at startup.
var protected struct {
	sync.RWMutex
	dataDir, appsDir string
	allowed          []string
}

// SetAllowedHostPaths registers host folders the operator explicitly allows
// apps to bind-mount even though they sit under a protected system folder
// (config.yaml allowed_bind_paths, e.g. /home/media). SimpleDeploy's data
// folder and other apps' folders stay protected. Relative or empty entries
// are ignored.
func SetAllowedHostPaths(paths []string) {
	var allowed []string
	for _, p := range paths {
		if filepath.IsAbs(p) {
			if c := filepath.Clean(p); c != "/" {
				allowed = append(allowed, c)
			}
		}
	}
	protected.Lock()
	defer protected.Unlock()
	protected.allowed = allowed
}

// SetProtectedPaths registers SimpleDeploy's data folder and apps folder.
// Bind mounts may not reach the data folder (database, secrets, backups)
// and may only reach the apps folder inside the app's own folder. Empty
// values disable that rule.
func SetProtectedPaths(dataDir, appsDir string) {
	protected.Lock()
	defer protected.Unlock()
	protected.dataDir = cleanAbs(dataDir)
	protected.appsDir = cleanAbs(appsDir)
}

// ViolationError reports a compose file refused by ValidateComposeSecurity.
type ViolationError struct {
	Violations []string
}

func (e *ViolationError) Error() string {
	return "compose file breaks security rules: " + strings.Join(e.Violations, "; ")
}

// ValidateComposeSecurity checks a parsed compose project for dangerous directives.
// Returns a list of violations. Empty list means safe.
func ValidateComposeSecurity(cfg *AppConfig) []string {
	if cfg == nil || cfg.Project == nil {
		return []string{"compose file could not be checked"}
	}
	h := newHostPaths(cfg)
	var violations []string

	for _, ref := range cfg.fileRefs {
		switch ref.kind {
		case "include":
			violations = append(violations, includeViolation)
		case "extends":
			if !h.confined(ref.path) {
				violations = append(violations, fmt.Sprintf("extends file %q must be inside the app folder", ref.path))
			}
		}
	}

	// Check every service, including ones behind an inactive profile: a
	// profile can be switched on without touching docker-compose.yml.
	services := map[string]types.ServiceConfig{}
	for name, svc := range cfg.Project.DisabledServices {
		services[name] = svc
	}
	for name, svc := range cfg.Project.Services {
		services[name] = svc
	}
	for _, name := range sortedKeys(services) {
		violations = append(violations, validateService(name, services[name], h)...)
	}

	for _, name := range sortedKeys(cfg.Project.Volumes) {
		violations = append(violations, validateVolume(name, cfg.Project.Volumes[name], cfg, h)...)
	}

	for _, key := range sortedKeys(cfg.Project.Networks) {
		n := cfg.Project.Networks[key]
		name := strings.ToLower(strings.TrimSpace(n.Name))
		if name == "host" || strings.HasPrefix(name, "container:") || strings.EqualFold(strings.TrimSpace(n.Driver), "host") {
			violations = append(violations, fmt.Sprintf("network %q: host or container networking not allowed", key))
		}
	}

	violations = append(violations, bindLayoutViolations(services, cfg.Project.Volumes, h)...)

	// secrets/configs with file: are bind-mounted from the host.
	for _, name := range sortedKeys(cfg.Project.Secrets) {
		s := cfg.Project.Secrets[name]
		if s.File != "" && !bool(s.External) && !h.confined(s.File) {
			violations = append(violations, fmt.Sprintf("secret %q: file %q must be inside the app folder", name, s.File))
		}
	}
	for _, name := range sortedKeys(cfg.Project.Configs) {
		c := cfg.Project.Configs[name]
		if c.File != "" && !bool(c.External) && !h.confined(c.File) {
			violations = append(violations, fmt.Sprintf("config %q: file %q must be inside the app folder", name, c.File))
		}
	}

	return violations
}

// bindLayoutViolations refuses a writable bind of the app folder itself and
// any bind whose host path lies inside another writable bind's host path.
// Mounts of local volumes whose driver_opts bind a host folder count as
// binds of that folder.
func bindLayoutViolations(services map[string]types.ServiceConfig, volumes types.Volumes, h hostPaths) []string {
	type bind struct {
		svc, src string
		rw       bool
	}
	var binds []bind
	for _, name := range sortedKeys(services) {
		for _, vol := range services[name].Volumes {
			if vol.Type == types.VolumeTypeVolume {
				if device, ro, ok := volumeHostBind(volumes[vol.Source]); ok {
					src := filepath.Clean(device)
					if real, ok := resolvePath(src); ok {
						src = real
					}
					binds = append(binds, bind{svc: name, src: src, rw: !vol.ReadOnly && !ro})
				}
				continue
			}
			src := vol.Source
			if src == "" || (vol.Type != types.VolumeTypeBind && !strings.HasPrefix(src, "/")) {
				continue
			}
			if !filepath.IsAbs(src) {
				if h.appDir == "" {
					continue
				}
				src = filepath.Join(h.appDir, src)
			}
			src = filepath.Clean(src)
			if real, ok := resolvePath(src); ok {
				src = real
			}
			binds = append(binds, bind{svc: name, src: src, rw: !vol.ReadOnly})
		}
	}
	var out []string
	seen := map[string]bool{}
	addOnce := func(msg string) {
		if !seen[msg] {
			seen[msg] = true
			out = append(out, msg)
		}
	}
	for _, b := range binds {
		if b.rw && h.appReal != "" && b.src == h.appReal {
			addOnce(fmt.Sprintf("service %q: writable bind of the app folder itself not allowed (mount a subfolder such as ./data, or add :ro)", b.svc))
		}
	}
	for _, a := range binds {
		if !a.rw {
			continue
		}
		for _, b := range binds {
			if b.src != a.src && within(b.src, a.src) {
				addOnce(fmt.Sprintf("service %q: bind %q is inside the writable bind %q; use separate folders", b.svc, b.src, a.src))
			}
		}
	}
	return out
}

// volumeHostBind reports whether a top-level volume is a local-driver bind
// of a host folder (type none, or o with bind/rbind) with an absolute
// device. ro is true when its mount options include ro.
func volumeHostBind(v types.VolumeConfig) (device string, ro, ok bool) {
	if v.Driver != "" && v.Driver != "local" {
		return "", false, false
	}
	opts := v.DriverOpts
	device = strings.TrimSpace(opts["device"])
	typ := strings.ToLower(strings.TrimSpace(opts["type"]))
	if !filepath.IsAbs(device) || !(typ == "none" || hasMountOpt(opts["o"], "bind") || hasMountOpt(opts["o"], "rbind")) {
		return "", false, false
	}
	return device, hasMountOpt(opts["o"], "ro"), true
}

func validateService(name string, svc types.ServiceConfig, h hostPaths) []string {
	var violations []string
	add := func(format string, args ...any) {
		violations = append(violations, fmt.Sprintf("service %q: ", name)+fmt.Sprintf(format, args...))
	}

	if svc.Privileged {
		add("privileged mode not allowed")
	}
	if svc.UseAPISocket {
		add("use_api_socket not allowed")
	}
	for _, hk := range append(append([]types.ServiceHook{}, svc.PostStart...), svc.PreStop...) {
		if hk.Privileged {
			add("privileged post_start/pre_stop hooks not allowed")
			break
		}
	}
	if svc.Provider != nil {
		add("provider services not allowed")
	}

	networkMode := strings.ToLower(strings.TrimSpace(svc.NetworkMode))
	if networkMode == "host" {
		add("network_mode 'host' not allowed")
	} else if strings.HasPrefix(networkMode, "container:") {
		add("network_mode %q not allowed (it joins another container's network)", svc.NetworkMode)
	}

	if svc.Pid == "host" {
		add("pid mode 'host' not allowed")
	} else if svc.Pid != "" {
		// "container:" and "service:" forms let the service inspect and
		// signal arbitrary neighbors.
		add("pid %q not allowed", svc.Pid)
	}

	ipc := strings.ToLower(strings.TrimSpace(svc.Ipc))
	if ipc == "host" {
		add("ipc mode 'host' not allowed")
	} else if strings.HasPrefix(ipc, "container:") {
		add("ipc %q not allowed (it shares another container's memory)", svc.Ipc)
	}

	if strings.EqualFold(strings.TrimSpace(svc.Uts), "host") {
		add("uts 'host' not allowed")
	}

	// userns_mode/cgroup: only "host" punches through namespace isolation.
	// Empty is the default (isolated) and stays allowed.
	if strings.EqualFold(svc.UserNSMode, "host") {
		add("userns_mode 'host' not allowed")
	}
	if strings.EqualFold(svc.Cgroup, "host") {
		add("cgroup 'host' not allowed")
	}

	for _, c := range svc.CapAdd {
		if _, bad := dangerousCaps[normalizeCap(c)]; bad {
			add("dangerous capability %q not allowed", c)
		}
	}

	// security_opt: disabling apparmor/seccomp/etc. negates host isolation
	// regardless of other compose hardening.
	for _, opt := range svc.SecurityOpt {
		if unsafeSecurityOpt(opt) {
			add("security_opt %q not allowed", opt)
		}
	}

	// devices and device_cgroup_rules expose host device nodes.
	if len(svc.Devices) > 0 {
		add("'devices' not allowed")
	}
	if len(svc.DeviceCgroupRules) > 0 {
		add("'device_cgroup_rules' not allowed")
	}

	// volumes_from imports another (possibly privileged) container's
	// volumes wholesale.
	if len(svc.VolumesFrom) > 0 {
		add("'volumes_from' not allowed")
	}

	// Bind mounts. Short syntax arrives as type "bind" with the source made
	// absolute by the loader; treat any absolute source as a host path.
	for _, vol := range svc.Volumes {
		src := vol.Source
		if src == "" || (vol.Type != types.VolumeTypeBind && !strings.HasPrefix(src, "/")) {
			continue
		}
		if reason := h.bindReason(src, vol.ReadOnly); reason != "" {
			add("mounting %q is not allowed (%s)", src, reason)
		}
	}

	// Files docker compose reads from the host on the app's behalf.
	for _, f := range svc.EnvFiles {
		if f.Path != "" && !h.confined(f.Path) {
			add("env_file %q must be inside the app folder", f.Path)
		}
	}
	for _, f := range svc.LabelFiles {
		if f != "" && !h.confined(f) {
			add("label_file %q must be inside the app folder", f)
		}
	}

	if b := svc.Build; b != nil {
		localContext := b.Context != "" && isLocalContext(b.Context)
		if localContext && !h.confined(b.Context) {
			add("build context %q must be inside the app folder", b.Context)
		}
		if localContext && b.DockerfileInline == "" && b.Dockerfile != "" {
			df := b.Dockerfile
			if !filepath.IsAbs(df) {
				df = filepath.Join(b.Context, df)
			}
			if !h.confined(df) {
				add("dockerfile %q must be inside the app folder", b.Dockerfile)
			}
		}
		for _, ctxName := range sortedKeys(b.AdditionalContexts) {
			v := b.AdditionalContexts[ctxName]
			if v != "" && isLocalContext(v) && !h.confined(v) {
				add("build context %q must be inside the app folder", v)
			}
		}
		for _, key := range b.SSH {
			if key.Path != "" && !h.confined(key.Path) {
				add("build ssh key %q must be inside the app folder", key.Path)
			}
		}
		if strings.EqualFold(b.Network, "host") {
			add("build network 'host' not allowed")
		}
		if b.Privileged {
			add("privileged build not allowed")
		}
		for _, e := range b.Entitlements {
			switch strings.ToLower(strings.TrimSpace(e)) {
			case "security.insecure", "network.host":
				add("build entitlement %q not allowed", e)
			}
		}
	}

	return violations
}

func validateVolume(name string, v types.VolumeConfig, cfg *AppConfig, h hostPaths) []string {
	var violations []string
	add := func(format string, args ...any) {
		violations = append(violations, fmt.Sprintf("volume %q: ", name)+fmt.Sprintf(format, args...))
	}

	// An explicit name like simpledeploy-<other>_data attaches another
	// app's storage. The only simpledeploy- name allowed is the one this
	// volume gets by default (simpledeploy-<app>_<key>).
	if strings.HasPrefix(v.Name, "simpledeploy-") &&
		v.Name != cfg.Project.Name+"_"+name && v.Name != "simpledeploy-"+cfg.Name+"_"+name {
		own := "simpledeploy-" + strings.TrimPrefix(cfg.Name, "simpledeploy-")
		def := own + "_" + name
		if strings.Contains(strings.TrimPrefix(v.Name, "simpledeploy-"), "_") && !strings.HasPrefix(v.Name, own+"_") {
			add("uses storage %q from another app; only the default name %q is allowed", v.Name, def)
		} else {
			add("name %q is not allowed; the only simpledeploy- name allowed is the default %q", v.Name, def)
		}
	}

	opts := v.DriverOpts
	if len(opts) == 0 {
		return violations
	}

	if v.Driver != "" && v.Driver != "local" {
		// Third-party drivers: their options are unknown, so treat any host
		// path they are pointed at like a bind mount.
		for _, key := range []string{"device", "mountpoint"} {
			p := strings.TrimSpace(opts[key])
			if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") {
				if reason := h.bindReason(p, false); reason != "" {
					add("host folder %q is not allowed (%s)", p, reason)
				}
			}
		}
		return violations
	}

	// The local driver runs mount(2) on the host with these options, so a
	// type like ext4, overlay or proc can expose host disks or processes.
	typ := strings.ToLower(strings.TrimSpace(opts["type"]))
	device := strings.TrimSpace(opts["device"])
	switch {
	case typ == "none" || hasMountOpt(opts["o"], "bind") || hasMountOpt(opts["o"], "rbind"):
		if device == "" {
			break
		}
		if !filepath.IsAbs(device) {
			add("device %q must be a full path", device)
			break
		}
		if reason := h.bindReason(device, false); reason != "" {
			add("host folder %q is not allowed (%s)", device, reason)
		}
	case typ == "" && device == "":
		// Plain local volume.
	default:
		if _, ok := networkVolumeTypes[typ]; !ok {
			add("driver_opts type %q not allowed", opts["type"])
		}
	}
	return violations
}

// ValidateComposeForDeploy is ValidateComposeSecurity plus endpoint label
// validation (ValidateEndpoints). Used by paths that accept new compose
// files (deploy API, bundle import) so bad routing labels are rejected up
// front. The reconciler disk scan only warns on endpoint problems, so an
// existing app is never dropped from routing after an upgrade.
func ValidateComposeForDeploy(cfg *AppConfig) []string {
	violations := ValidateComposeSecurity(cfg)
	if cfg == nil {
		return violations
	}
	return append(violations, ValidateEndpoints(cfg.Endpoints)...)
}

// hostPaths holds the folders a compose file's host paths are checked
// against. Protected folders are kept both as written and with symlinks
// resolved.
type hostPaths struct {
	appDir, appReal string // the app's own folder; empty when unknown
	dataDirs        []string
	appsDirs        []string
	allowed         []string // operator-allowed host folders (and resolved forms)
}

func newHostPaths(cfg *AppConfig) hostPaths {
	var h hostPaths
	if cfg.ComposePath != "" {
		appDir := filepath.Dir(cleanAbs(cfg.ComposePath))
		if real, ok := resolvePath(appDir); ok {
			h.appDir, h.appReal = appDir, real
		}
	}
	protected.RLock()
	dataDir, appsDir, allowed := protected.dataDir, protected.appsDir, protected.allowed
	protected.RUnlock()
	h.dataDirs = withResolved(dataDir)
	h.appsDirs = withResolved(appsDir)
	for _, a := range allowed {
		h.allowed = append(h.allowed, withResolved(a)...)
	}
	return h
}

// bindReason returns why the host path src may not be mounted, or "" when
// it is allowed. Symlinks are followed, so a link inside the app folder
// pointing at /etc is caught. readOnly relaxes the rule for readOnlyPaths.
func (h hostPaths) bindReason(src string, readOnly bool) string {
	if !filepath.IsAbs(src) {
		if h.appDir == "" {
			return "the path could not be checked"
		}
		src = filepath.Join(h.appDir, src)
	}
	lex := filepath.Clean(src)
	real, ok := resolvePath(lex)
	if !ok {
		return "the path could not be checked"
	}
	if h.inApp(lex, real) {
		return ""
	}
	// A path written inside the app folder only fails through a symlink,
	// so judge it by where it really points.
	if h.appDir == "" || !within(lex, h.appDir) {
		if reason := h.protectedReason(lex, readOnly); reason != "" {
			return reason
		}
	}
	if real != lex {
		if reason := h.protectedReason(real, readOnly); reason != "" {
			return fmt.Sprintf("it links to %q, %s", real, reason)
		}
	}
	return ""
}

// protectedReason checks the most specific rule first so the message
// names the real problem.
func (h hostPaths) protectedReason(p string, readOnly bool) string {
	for _, a := range h.appsDirs {
		if p != a && within(p, a) {
			return "another app's folder"
		}
	}
	for _, d := range h.dataDirs {
		if overlaps(p, d) {
			return "SimpleDeploy's data folder"
		}
	}
	for _, a := range h.appsDirs {
		if within(a, p) {
			return "it holds every app's folder"
		}
	}
	for _, a := range h.allowed {
		if within(p, a) {
			return ""
		}
	}
	if readOnly {
		for _, ro := range readOnlyPaths {
			if within(p, ro) {
				return ""
			}
		}
	}
	for _, s := range systemPaths {
		if overlaps(p, s) {
			return "protected system folder"
		}
	}
	return ""
}

// confined reports whether p (relative paths are taken from the app
// folder) stays inside the app folder after resolving symlinks.
func (h hostPaths) confined(p string) bool {
	if h.appDir == "" {
		return false
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(h.appDir, p)
	}
	lex := filepath.Clean(p)
	real, ok := resolvePath(lex)
	return ok && h.inApp(lex, real)
}

func (h hostPaths) inApp(lex, real string) bool {
	return h.appDir != "" && within(lex, h.appDir) && within(real, h.appReal)
}

// resolvePath resolves symlinks in the longest existing prefix of the
// clean absolute path p; components that do not exist yet are kept as is.
// ok is false when a component exists but cannot be resolved (dangling
// link, loop, permission error).
func resolvePath(p string) (string, bool) {
	cur, rest := p, ""
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest), true
		}
		if _, err := os.Lstat(cur); err == nil || !os.IsNotExist(err) {
			return "", false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p, true
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// within reports whether p is base or inside base (clean absolute paths).
func within(p, base string) bool {
	if base == "/" {
		return strings.HasPrefix(p, "/")
	}
	return p == base || strings.HasPrefix(p, base+"/")
}

// overlaps reports whether a and b are the same path or one contains the other.
func overlaps(a, b string) bool {
	return within(a, b) || within(b, a)
}

func cleanAbs(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

func withResolved(p string) []string {
	if p == "" {
		return nil
	}
	out := []string{p}
	if real, ok := resolvePath(p); ok && real != p {
		out = append(out, real)
	}
	return out
}

func normalizeCap(c string) string {
	return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_")
}

// unsafeSecurityOpt reports whether a security_opt entry turns off a host
// protection. Docker accepts both "key=value" and "key:value".
func unsafeSecurityOpt(opt string) bool {
	s := strings.ToLower(strings.ReplaceAll(opt, " ", ""))
	key, val := s, ""
	if i := strings.IndexAny(s, "=:"); i >= 0 {
		key, val = s[:i], s[i+1:]
	}
	switch key {
	case "seccomp":
		// "unconfined" or a custom profile file, which can allow anything.
		return val != "builtin"
	case "apparmor", "systempaths":
		return val == "unconfined"
	case "label":
		// type:spc_t and similar run the container in a privileged domain.
		return val == "disable" || strings.HasPrefix(val, "type:")
	case "no-new-privileges":
		return val == "false"
	}
	return false
}

// isLocalContext mirrors compose's rule for build contexts that are host
// paths rather than URLs, images or other services.
func isLocalContext(v string) bool {
	if strings.Contains(v, "://") || strings.HasPrefix(v, types.ServicePrefix) {
		return false
	}
	for _, prefix := range []string{"https://", "http://", "git://", "ssh://", "github.com/", "git@"} {
		if strings.HasPrefix(v, prefix) {
			return false
		}
	}
	return true
}

func hasMountOpt(o, want string) bool {
	for _, part := range strings.Split(strings.ToLower(o), ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
