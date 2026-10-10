package backup

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
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
			paths = append(paths, path.Clean(v.Target))
		}
	}
	return paths
}

func (s *VolumeStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	if len(opts.Paths) == 0 {
		return nil, fmt.Errorf("no volume paths specified")
	}
	if err := ValidatePaths("volume", opts.Paths); err != nil {
		return nil, err
	}

	filename := fmt.Sprintf("%s-%s.tar.gz", opts.ContainerName, time.Now().Format("20060102-150405"))

	cmd := exec.CommandContext(ctx, "docker", tarCreateArgs(opts.ContainerName, opts.Paths)...)
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
	// Validate the archive before handing it to the in-container tar so an
	// attacker-uploaded backup with absolute paths, '..' segments, or
	// symlinks cannot escape the container's volume layout (and via bind
	// mounts, the host).
	safe, err := validateTarStream(opts.Reader, opts.MaxDecompressedBytes)
	if err != nil {
		return fmt.Errorf("reject restore archive: %w", err)
	}
	defer safe.Close()
	// Keep extract flags portable: BusyBox tar (Alpine) lacks
	// --no-same-owner/--no-overwrite-dir. validateTarStream already
	// rejects the dangerous archive shapes those flags were guarding.
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", opts.ContainerName,
		"tar", "-xzf", "-", "-C", "/")
	cmd.Stdin = safe

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar restore: %w: %s", err, out)
	}
	return nil
}

// tarCreateArgs builds the 'docker exec <container> tar -czf - ...' argv
// for paths. -C / + relative paths so archive entries are normalized to
// relative form (validateTarStream on restore rejects absolute paths), and
// "--" ends option parsing so no operand can be read as a tar flag.
func tarCreateArgs(container string, paths []string) []string {
	args := []string{"exec", container, "tar", "-czf", "-", "-C", "/", "--"}
	for _, p := range paths {
		args = append(args, strings.TrimPrefix(p, "/"))
	}
	return args
}
