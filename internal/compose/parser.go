package compose

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// AppConfig holds the parsed compose file config plus extracted simpledeploy labels.
type AppConfig struct {
	Name            string
	ComposePath     string
	Domain          string
	Port            string
	TLS             string
	BackupStrategy  string
	BackupSchedule  string
	BackupTarget    string
	BackupRetention string
	AlertCPU        string
	AlertMemory     string
	PathPatterns    string
	Registries      string
	RateLimit       RateLimitLabels
	Services        []ServiceConfig
	Project         *types.Project
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
}

// PortMapping represents a host:container port binding.
type PortMapping struct {
	Host, Container, Protocol string
}

// VolumeMount represents a service volume mount.
type VolumeMount struct {
	Source, Target, Type string
}

// LabelConfig holds all extracted simpledeploy.* labels.
type LabelConfig struct {
	Domain          string
	Port            string
	TLS             string
	BackupStrategy  string
	BackupSchedule  string
	BackupTarget    string
	BackupRetention string
	AlertCPU        string
	AlertMemory     string
	PathPatterns    string
	Registries      string
	RateLimit       RateLimitLabels
}

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

	// merge all labels, first encountered wins
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
		Domain:          lc.Domain,
		Port:            lc.Port,
		TLS:             lc.TLS,
		BackupStrategy:  lc.BackupStrategy,
		BackupSchedule:  lc.BackupSchedule,
		BackupTarget:    lc.BackupTarget,
		BackupRetention: lc.BackupRetention,
		AlertCPU:        lc.AlertCPU,
		AlertMemory:     lc.AlertMemory,
		PathPatterns:    lc.PathPatterns,
		Registries:      lc.Registries,
		RateLimit:       lc.RateLimit,
		Project:         project,
	}

	for name, svc := range project.Services {
		cfg.Services = append(cfg.Services, convertService(name, svc))
	}

	return cfg, nil
}

// ExtractLabels extracts all simpledeploy.* labels from the provided map.
func ExtractLabels(labels map[string]string) LabelConfig {
	return LabelConfig{
		Domain:          labels["simpledeploy.domain"],
		Port:            labels["simpledeploy.port"],
		TLS:             labels["simpledeploy.tls"],
		BackupStrategy:  labels["simpledeploy.backup.strategy"],
		BackupSchedule:  labels["simpledeploy.backup.schedule"],
		BackupTarget:    labels["simpledeploy.backup.target"],
		BackupRetention: labels["simpledeploy.backup.retention"],
		AlertCPU:        labels["simpledeploy.alert.cpu"],
		AlertMemory:     labels["simpledeploy.alert.memory"],
		PathPatterns:    labels["simpledeploy.paths"],
		Registries:      labels["simpledeploy.registries"],
		RateLimit: RateLimitLabels{
			Requests: labels["simpledeploy.ratelimit.requests"],
			Window:   labels["simpledeploy.ratelimit.window"],
			By:       labels["simpledeploy.ratelimit.by"],
			Burst:    labels["simpledeploy.ratelimit.burst"],
		},
	}
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

	return sc
}
