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

	// Use synchronous SAVE instead of BGSAVE+poll. SAVE blocks the Redis
	// server for the duration of the write, but for backup sizes typical of
	// simpledeploy apps this is sub-second and avoids the race where
	// BGSAVE's LASTSAVE timestamp (1s resolution) never appears to change
	// on a loaded CI runner despite BGSAVE actually completing. BGSAVE
	// fallback is kept for the (rare) case SAVE is disallowed by config.
	out, err := exec.CommandContext(ctx, "docker", "exec", container, "redis-cli", "SAVE").CombinedOutput()
	if err != nil || !strings.Contains(strings.ToUpper(string(out)), "OK") {
		// SAVE unavailable or errored; try BGSAVE with a wait loop.
		initial, lerr := exec.CommandContext(ctx, "docker", "exec", container, "redis-cli", "LASTSAVE").Output()
		if lerr != nil {
			return nil, fmt.Errorf("redis SAVE: %w: %s; LASTSAVE fallback: %v", err, out, lerr)
		}
		initialTS := strings.TrimSpace(string(initial))
		bgOut, bgErr := exec.CommandContext(ctx, "docker", "exec", container, "redis-cli", "BGSAVE").CombinedOutput()
		if bgErr != nil {
			return nil, fmt.Errorf("redis BGSAVE: %w: %s", bgErr, bgOut)
		}
		if werr := s.waitForSaveSince(ctx, container, initialTS); werr != nil {
			return nil, werr
		}
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
	// Cap wait at ~120s: each iteration costs a docker exec (~500ms on loaded
	// CI runners) plus the 1s sleep, so 30 iterations can be short of real
	// wall time under load. BGSAVE itself is near-instant for test-sized
	// dbs; this is generous enough to absorb docker-exec overhead.
	const maxAttempts = 120
	for i := 0; i < maxAttempts; i++ {
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
	return fmt.Errorf("redis BGSAVE did not complete within %ds", maxAttempts)
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
	cmd.Stdin = limitedGzip(gr, opts.MaxDecompressedBytes)

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
