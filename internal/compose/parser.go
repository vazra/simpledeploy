package compose

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// EndpointConfig holds config for a single endpoint (domain/port/tls bound to a service).
type EndpointConfig struct {
	Domain  string `json:"domain"`
	Port    string `json:"port"`
	TLS     string `json:"tls"`
	Service string `json:"service"`
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

var endpointLabelRe = regexp.MustCompile(`^simpledeploy\.endpoints\.(\d+)\.(domain|port|tls)$`)

// ParseFile parses the compose file at path and returns an AppConfig with appName as the name.
// simpledeploy.* labels are collected across all services; the first value found wins.
func ParseFile(path, appName string) (*AppConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	configDetails, err := loader.LoadConfigFiles(
		context.Background(),
		[]string{absPath},
		filepath.Dir(absPath),
	)
	if err != nil {
		return nil, fmt.Errorf("load config files: %w", err)
	}

	project, err := loader.LoadWithContext(
		context.Background(),
		*configDetails,
		func(o *loader.Options) {
			o.SetProjectName(appName, true)
			o.SkipConsistencyCheck = true
			o.SkipNormalization = false
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
	}

	// Extract endpoints per service
	for name, svc := range project.Services {
		eps := extractEndpoints(svc.Labels, name)
		cfg.Endpoints = append(cfg.Endpoints, eps...)
	}
	// Stable sort by index (extractEndpoints returns sorted per-service)
	sort.SliceStable(cfg.Endpoints, func(i, j int) bool {
		return cfg.Endpoints[i].Domain < cfg.Endpoints[j].Domain
	})

	for name, svc := range project.Services {
		cfg.Services = append(cfg.Services, convertService(name, svc))
	}

	return cfg, nil
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

// extractEndpoints scans labels for simpledeploy.endpoints.N.{domain,port,tls}
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
			byIndex[idx] = &EndpointConfig{Service: serviceName}
		}
		switch m[2] {
		case "domain":
			byIndex[idx].Domain = v
		case "port":
			byIndex[idx].Port = v
		case "tls":
			byIndex[idx].TLS = v
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
