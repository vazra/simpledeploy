package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

var redisKeywords = []string{"redis", "valkey", "dragonfly"}

// RedisStrategy backs up and restores Redis by triggering BGSAVE
// then copying the RDB file.
type RedisStrategy struct{}

func NewRedisStrategy() *RedisStrategy {
	return &RedisStrategy{}
}

func (s *RedisStrategy) Type() string { return "redis" }

func (s *RedisStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var results []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "redis") || matchesImageKeywords(svc.Image, redisKeywords) {
			results = append(results, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: cfg.Name + "-" + svc.Name + "-1",
				Label:         "redis",
			})
		}
	}
	return results
}

func (s *RedisStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	container := opts.ContainerName
	filename := fmt.Sprintf("%s-%s.rdb.gz", container, time.Now().Format("20060102-150405"))

	// Capture the pre-BGSAVE LASTSAVE timestamp first. If we read it after
	// BGSAVE, a fast save (e.g. an empty DB) may already have bumped the
	// timestamp and the poll loop would never observe a change.
	initial, err := exec.CommandContext(ctx, "docker", "exec", container, "redis-cli", "LASTSAVE").Output()
	if err != nil {
		return nil, fmt.Errorf("redis LASTSAVE (initial): %w", err)
	}
	initialTS := strings.TrimSpace(string(initial))

	// Trigger BGSAVE
	out, err := exec.CommandContext(ctx, "docker", "exec", container, "redis-cli", "BGSAVE").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("redis BGSAVE: %w: %s", err, out)
	}

	if err := s.waitForSaveSince(ctx, container, initialTS); err != nil {
		return nil, err
	}

	// Copy RDB file out via docker cp piped to stdout
	cmd := exec.CommandContext(ctx, "docker", "cp", container+":/data/dump.rdb", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start docker cp: %w", err)
	}

	pr, pw := io.Pipe()
	gz := gzip.NewWriter(pw)

	go func() {
		_, copyErr := io.Copy(gz, stdout)
		gz.Close()
		if copyErr != nil {
			pw.CloseWithError(copyErr)
			cmd.Wait()
			return
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			pw.CloseWithError(fmt.Errorf("docker cp: %w", waitErr))
			return
		}
		pw.Close()
	}()

	return &BackupResult{Reader: pr, Filename: filename}, nil
}

func (s *RedisStrategy) waitForSaveSince(ctx context.Context, container, initialTS string) error {
	for i := 0; i < 30; i++ {
		current, err := exec.CommandContext(ctx, "docker", "exec", container, "redis-cli", "LASTSAVE").Output()
		if err != nil {
			return fmt.Errorf("redis LASTSAVE: %w", err)
		}
		if strings.TrimSpace(string(current)) != initialTS {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("redis BGSAVE did not complete within 30s")
}

func (s *RedisStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	container := opts.ContainerName

	// Stop the container
	if out, err := exec.CommandContext(ctx, "docker", "stop", container).CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %w: %s", err, out)
	}

	// Decompress and write to temp file, then docker cp in
	gr, err := gzip.NewReader(opts.Reader)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	// docker cp reads from stdin as a tar archive; we pipe the decompressed rdb
	// We need to create a tar with the rdb file and pipe it to docker cp
	cmd := exec.CommandContext(ctx, "docker", "cp", "-", container+":/data/")
	cmd.Stdin = gr

	if out, err := cmd.CombinedOutput(); err != nil {
		if restartOut, restartErr := exec.CommandContext(ctx, "docker", "start", container).CombinedOutput(); restartErr != nil {
			return fmt.Errorf("docker cp restore: %w: %s (restart also failed: %s)", err, truncateOutput(out), truncateOutput(restartOut))
		}
		return fmt.Errorf("docker cp restore: %w: %s", err, truncateOutput(out))
	}

	// Start the container back
	if out, err := exec.CommandContext(ctx, "docker", "start", container).CombinedOutput(); err != nil {
		return fmt.Errorf("docker start: %w: %s", err, out)
	}

	return nil
}
