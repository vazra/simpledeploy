package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"

	"github.com/vazra/simpledeploy/internal/fsutil"
)

// EndpointConfig holds config for a single endpoint (domain/port/tls bound to a service).
type EndpointConfig struct {
	Domain   string `json:"domain"`
	Port     string `json:"port"`
	TLS      string `json:"tls"`
	Service  string `json:"service"`
	Protocol string `json:"protocol,omitempty"` // "", "http", "h2c" or "grpc"
	Path     string `json:"path,omitempty"`     // optional Caddy path matcher, e.g. "/ws*"
	// Index is N from the simpledeploy.endpoints.N.* labels (per service).
	// Used for deterministic ordering and error messages; not serialized.
	Index int `json:"-"`
}

// AppConfig holds the parsed compose file config plus extracted simpledeploy labels.
type AppConfig struct {
	Name            string
	ComposePath     string
	Endpoints       []EndpointConfig
	BackupStrategy  string
	BackupSchedule  string
	BackupTarget    string
	BackupRetention string
	AlertCPU        string
	AlertMemory     string
	Registries      string
	AccessAllow     string
	RateLimit       RateLimitLabels
	Services        []ServiceConfig
	Project         *types.Project

	// fileRefs records include/extends files seen while loading, so the
	// validator can check them (the loaded Project no longer has them).
	fileRefs []fileRef
}

// fileRef is an include or extends reference captured during load.
type fileRef struct {
	kind string // "include" or "extends"
	path string // extends file as written; empty for include
}

// PrimaryDomain returns the domain of the first endpoint, or empty string.
func (a *AppConfig) PrimaryDomain() string {
	if len(a.Endpoints) > 0 {
		return a.Endpoints[0].Domain
	}
	return ""
}

// RateLimitLabels holds simpledeploy.ratelimit.* label values.
type RateLimitLabels struct {
	Requests, Window, By, Burst string
}

// ServiceConfig is a simplified representation of a compose service.
type ServiceConfig struct {
	Name        string
	Image       string
	Ports       []PortMapping
	Environment map[string]string
	Volumes     []VolumeMount
	Restart     string
	Labels      map[string]string
	DependsOn   []string
	DeployMode  string
}

// PortMapping represents a host:container port binding.
type PortMapping struct {
	Host, Container, Protocol string
}

// VolumeMount represents a service volume mount.
type VolumeMount struct {
	Source, Target, Type string
}

// LabelConfig holds all extracted simpledeploy.* labels (non-endpoint labels).
type LabelConfig struct {
	BackupStrategy  string
	BackupSchedule  string
	BackupTarget    string
	BackupRetention string
	AlertCPU        string
	AlertMemory     string
	Registries      string
	AccessAllow     string
	RateLimit       RateLimitLabels
}

var endpointLabelRe = regexp.MustCompile(`^simpledeploy\.endpoints\.(\d+)\.(domain|port|tls|protocol|path)$`)

// ParseFile parses the compose file at path and returns an AppConfig with appName as the name.
// simpledeploy.* labels are collected across all services; the first value found wins.
// ${VAR} references are interpolated the way `docker compose` does it (see
// ParseContent). A docker-compose.yml or .env that is a symlink is refused.
func ParseFile(path, appName string) (*AppConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	content, err := fsutil.ReadRegularFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	dotEnv, err := ReadDotEnv(filepath.Dir(absPath))
	if err != nil {
		return nil, err
	}
	return ParseContent(content, absPath, appName, dotEnv)
}

// ParseForRoutes parses the compose file at path for routing only, for a
// file ParseFile refuses over its file references (*ViolationError). It
// drops a top-level include and each service's label_file and file-based
// extends, and ignores the app's .env. The result must never be deployed:
// it only keeps the routes of an unchanged running app known.
func ParseForRoutes(path, appName string) (*AppConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	content, err := fsutil.ReadRegularFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	stripped, err := stripFileRefs(content)
	if err != nil {
		return nil, fmt.Errorf("parse compose: %w", err)
	}
	return ParseContent(stripped, absPath, appName, nil)
}

// ErrDotEnv marks an app .env file that cannot be used: a symlink, or a
// syntax error that docker compose would reject too.
var ErrDotEnv = errors.New("the app's .env file cannot be used")

// ReadDotEnv returns the .env file in dir, or nil when there is none. A
// symlinked .env is refused so it cannot pull in a file from outside the
// app folder.
func ReadDotEnv(dir string) ([]byte, error) {
	data, err := fsutil.ReadRegularFile(filepath.Join(dir, ".env"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrDotEnv, err)
	}
	return data, nil
}

// ParseContent parses compose content as if it were stored at path, with
// dotEnv as the content of the .env file next to it (nil for none). This
// lets callers validate a compose file before it is written to disk.
//
// It mirrors what `docker compose -f path` sees: relative paths resolve
// against the folder of path, and ${VAR} references are interpolated from
// dotEnv overridden by the process environment (docker compose inherits our
// environment). Without this, a value set in .env could hide a dangerous
// setting from the validator.
//
// A top-level include, or a label_file or extends file that is not a
// regular file inside the app folder, returns a *ViolationError.
func ParseContent(content []byte, path, appName string, dotEnv []byte) (*AppConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	env, err := interpolationEnv(dotEnv)
	if err != nil {
		return nil, err
	}
	// The loader reads include, extends and label_file targets itself, so
	// refuse unsafe ones on the raw YAML before it runs.
	if err := checkRawFileRefs(content, filepath.Dir(absPath)); err != nil {
		return nil, err
	}

	var refs []fileRef
	listener := func(event string, md map[string]any) {
		switch event {
		case "include":
			refs = append(refs, fileRef{kind: "include"})
		case "extends":
			if f, ok := md["file"].(string); ok && f != "" {
				refs = append(refs, fileRef{kind: "extends", path: f})
			}
		}
	}

	configDetails := types.ConfigDetails{
		WorkingDir:  filepath.Dir(absPath),
		ConfigFiles: []types.ConfigFile{{Filename: absPath, Content: content}},
		Environment: env,
	}
	project, err := loader.LoadWithContext(
		context.Background(),
		configDetails,
		func(o *loader.Options) {
			o.SetProjectName(appName, true)
			o.SkipConsistencyCheck = true
			o.SkipNormalization = false
			// Do not read env_file contents while parsing: the validator
			// only needs the paths, and the files may not be ours to read.
			o.SkipResolveEnvironment = true
			o.SkipInclude = true
			o.Profiles = composeProfiles(env)
			o.Listeners = append(o.Listeners, listener)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("load compose: %w", err)
	}

	// merge non-endpoint labels, first encountered wins
	merged := map[string]string{}
	for _, svc := range project.Services {
		for k, v := range svc.Labels {
			if _, exists := merged[k]; !exists {
				merged[k] = v
			}
		}
	}

	lc := ExtractLabels(merged)

	cfg := &AppConfig{
		Name:            appName,
		ComposePath:     absPath,
		BackupStrategy:  lc.BackupStrategy,
		BackupSchedule:  lc.BackupSchedule,
		BackupTarget:    lc.BackupTarget,
		BackupRetention: lc.BackupRetention,
		AlertCPU:        lc.AlertCPU,
		AlertMemory:     lc.AlertMemory,
		Registries:      lc.Registries,
		AccessAllow:     lc.AccessAllow,
		RateLimit:       lc.RateLimit,
		Project:         project,
		fileRefs:        refs,
	}

	// Extract endpoints per service
	for name, svc := range project.Services {
		eps := extractEndpoints(svc.Labels, name)
		cfg.Endpoints = append(cfg.Endpoints, eps...)
	}
	// Deterministic order: domain, then label index N, then service name.
	// Services come from a map, so without the tie-breakers the order of
	// endpoints sharing a domain would vary between parses.
	sort.SliceStable(cfg.Endpoints, func(i, j int) bool {
		a, b := cfg.Endpoints[i], cfg.Endpoints[j]
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.Index != b.Index {
			return a.Index < b.Index
		}
		return a.Service < b.Service
	})

	for name, svc := range project.Services {
		cfg.Services = append(cfg.Services, convertService(name, svc))
	}

	return cfg, nil
}

// interpolationEnv builds the variables docker compose interpolates with:
// the app's .env values, overridden by the process environment.
func interpolationEnv(dotEnv []byte) (map[string]string, error) {
	osEnv := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			osEnv[k] = v
		}
	}
	env := map[string]string{}
	if len(dotEnv) > 0 {
		vals, err := dotenv.UnmarshalBytesWithLookup(dotEnv, func(k string) (string, bool) {
			v, ok := osEnv[k]
			return v, ok
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrDotEnv, err)
		}
		for k, v := range vals {
			env[k] = v
		}
	}
	for k, v := range osEnv {
		env[k] = v
	}
	return env, nil
}

// composeProfiles returns the profiles docker compose would activate from
// COMPOSE_PROFILES (which may come from .env).
func composeProfiles(env map[string]string) []string {
	var out []string
	for _, p := range strings.Split(env["COMPOSE_PROFILES"], ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ExtractLabels extracts non-endpoint simpledeploy.* labels from the provided map.
func ExtractLabels(labels map[string]string) LabelConfig {
	return LabelConfig{
		BackupStrategy:  labels["simpledeploy.backup.strategy"],
		BackupSchedule:  labels["simpledeploy.backup.schedule"],
		BackupTarget:    labels["simpledeploy.backup.target"],
		BackupRetention: labels["simpledeploy.backup.retention"],
		AlertCPU:        labels["simpledeploy.alert.cpu"],
		AlertMemory:     labels["simpledeploy.alert.memory"],
		Registries:      labels["simpledeploy.registries"],
		AccessAllow:     labels["simpledeploy.access.allow"],
		RateLimit: RateLimitLabels{
			Requests: labels["simpledeploy.ratelimit.requests"],
			Window:   labels["simpledeploy.ratelimit.window"],
			By:       labels["simpledeploy.ratelimit.by"],
			Burst:    labels["simpledeploy.ratelimit.burst"],
		},
	}
}

// extractEndpoints scans labels for simpledeploy.endpoints.N.{domain,port,tls,protocol,path}
// and returns EndpointConfigs sorted by index, with Service set to serviceName.
func extractEndpoints(labels types.Labels, serviceName string) []EndpointConfig {
	byIndex := map[int]*EndpointConfig{}
	for k, v := range labels {
		m := endpointLabelRe.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		if byIndex[idx] == nil {
			byIndex[idx] = &EndpointConfig{Service: serviceName, Index: idx}
		}
		switch m[2] {
		case "domain":
			byIndex[idx].Domain = v
		case "port":
			byIndex[idx].Port = v
		case "tls":
			byIndex[idx].TLS = v
		case "protocol":
			byIndex[idx].Protocol = NormalizeProtocol(v)
		case "path":
			byIndex[idx].Path = strings.TrimSpace(v)
		}
	}

	// Sort by index
	indices := make([]int, 0, len(byIndex))
	for idx := range byIndex {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	eps := make([]EndpointConfig, 0, len(indices))
	for _, idx := range indices {
		eps = append(eps, *byIndex[idx])
	}
	return eps
}

func convertService(name string, svc types.ServiceConfig) ServiceConfig {
	sc := ServiceConfig{
		Name:      name,
		Image:     svc.Image,
		Restart:   svc.Restart,
		Labels:    make(map[string]string),
		DependsOn: make([]string, 0, len(svc.DependsOn)),
	}

	for k, v := range svc.Labels {
		sc.Labels[k] = v
	}

	for dep := range svc.DependsOn {
		sc.DependsOn = append(sc.DependsOn, dep)
	}

	sc.Environment = make(map[string]string)
	for k, v := range svc.Environment {
		if v != nil {
			sc.Environment[k] = *v
		} else {
			sc.Environment[k] = ""
		}
	}

	for _, p := range svc.Ports {
		sc.Ports = append(sc.Ports, PortMapping{
			Host:      p.Published,
			Container: fmt.Sprintf("%d", p.Target),
			Protocol:  p.Protocol,
		})
	}

	for _, v := range svc.Volumes {
		sc.Volumes = append(sc.Volumes, VolumeMount{
			Source: v.Source,
			Target: v.Target,
			Type:   v.Type,
		})
	}

	if svc.Deploy != nil {
		sc.DeployMode = svc.Deploy.Mode
	}

	return sc
}
