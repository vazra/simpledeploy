package backup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// VolumeStrategy backs up and restores a container volume via tar.
type VolumeStrategy struct {
	VolumePath string // path inside container, defaults to "/data"
}

func NewVolumeStrategy(volumePath string) *VolumeStrategy {
	if volumePath == "" {
		volumePath = "/data"
	}
	return &VolumeStrategy{VolumePath: volumePath}
}

func (s *VolumeStrategy) Backup(ctx context.Context, containerName string) (io.ReadCloser, string, error) {
	filename := fmt.Sprintf("%s-%s.tar.gz", containerName, time.Now().Format("20060102-150405"))

	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "tar", "-czf", "-", "-C", s.VolumePath, ".")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start tar: %w", err)
	}

	pr, pw := io.Pipe()

	go func() {
		_, copyErr := io.Copy(pw, stdout)
		if copyErr != nil {
			pw.CloseWithError(copyErr)
			cmd.Wait()
			return
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			pw.CloseWithError(fmt.Errorf("tar: %w", waitErr))
			return
		}
		pw.Close()
	}()

	return pr, filename, nil
}

func (s *VolumeStrategy) Restore(ctx context.Context, containerName string, data io.Reader) error {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "tar", "-xzf", "-", "-C", s.VolumePath)
	cmd.Stdin = data

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar restore: %w: %s", err, out)
	}
	return nil
}
