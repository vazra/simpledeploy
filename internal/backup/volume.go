package backup

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

// VolumeStrategy backs up and restores container volumes via tar.
type VolumeStrategy struct{}

func NewVolumeStrategy() *VolumeStrategy {
	return &VolumeStrategy{}
}

func (s *VolumeStrategy) Type() string { return "volume" }

func (s *VolumeStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var results []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "volume") {
			results = append(results, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: cfg.Name + "-" + svc.Name + "-1",
				Label:         "volume",
				Paths:         collectVolumePaths(svc),
			})
			continue
		}

		// Default: collect all volume mount targets (excluding docker.sock)
		paths := collectVolumePaths(svc)
		if len(paths) > 0 {
			results = append(results, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: cfg.Name + "-" + svc.Name + "-1",
				Label:         "volume",
				Paths:         paths,
			})
		}
	}
	return results
}

func collectVolumePaths(svc compose.ServiceConfig) []string {
	var paths []string
	for _, v := range svc.Volumes {
		if v.Target == "/var/run/docker.sock" {
			continue
		}
		if v.Target != "" {
			paths = append(paths, v.Target)
		}
	}
	return paths
}

func (s *VolumeStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	if len(opts.Paths) == 0 {
		return nil, fmt.Errorf("no volume paths specified")
	}

	filename := fmt.Sprintf("%s-%s.tar.gz", opts.ContainerName, time.Now().Format("20060102-150405"))

	args := []string{"exec", opts.ContainerName, "tar", "-czf", "-"}
	args = append(args, opts.Paths...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start tar: %w", err)
	}

	return &BackupResult{
		Reader:   &cmdReadCloser{ReadCloser: stdout, cmd: cmd},
		Filename: filename,
	}, nil
}

func (s *VolumeStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", opts.ContainerName,
		"tar", "-xzf", "-", "-C", "/")
	cmd.Stdin = opts.Reader

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar restore: %w: %s", err, out)
	}
	return nil
}
