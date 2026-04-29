package backup

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

// SQLiteStrategy backs up SQLite databases inside containers using
// the .backup command for consistency. Label-only detection (no image keywords).
type SQLiteStrategy struct{}

func NewSQLiteStrategy() *SQLiteStrategy {
	return &SQLiteStrategy{}
}

func (s *SQLiteStrategy) Type() string { return "sqlite" }

func (s *SQLiteStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var results []DetectedService
	for _, svc := range cfg.Services {
		if !matchesLabel(svc.Labels, "sqlite") {
			continue
		}

		// Collect volume mount targets as potential DB paths
		var paths []string
		for _, v := range svc.Volumes {
			if v.Target != "" {
				paths = append(paths, v.Target)
			}
		}

		results = append(results, DetectedService{
			ServiceName:   svc.Name,
			ContainerName: cfg.Name + "-" + svc.Name + "-1",
			Label:         "sqlite",
			Paths:         paths,
		})
	}
	return results
}

func (s *SQLiteStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	container := opts.ContainerName
	filename := fmt.Sprintf("%s-%s.sqlite.tar.gz", container, time.Now().Format("20060102-150405"))

	if len(opts.Paths) == 0 {
		return nil, fmt.Errorf("no SQLite database paths specified")
	}

	// Use sqlite3 .backup for each path to get a consistent snapshot
	var backupPaths []string
	for i, dbPath := range opts.Paths {
		tmpPath := fmt.Sprintf("/tmp/sd-backup-%d.db", i)
		backupCmd := fmt.Sprintf(".backup '%s'", tmpPath)
		cmd := exec.CommandContext(ctx, "docker", "exec", container,
			"sqlite3", dbPath, backupCmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("sqlite3 backup %s: %w: %s", dbPath, err, out)
		}
		backupPaths = append(backupPaths, tmpPath)
	}

	// Tar+gzip all backup files and stream out
	tarArgs := []string{"exec", container, "tar", "-czf", "-"}
	tarArgs = append(tarArgs, backupPaths...)

	cmd := exec.CommandContext(ctx, "docker", tarArgs...)
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

func (s *SQLiteStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	container := opts.ContainerName

	if len(opts.Paths) == 0 {
		return fmt.Errorf("no SQLite database paths specified")
	}

	// Validate the archive before handing it to the in-container tar to
	// block tar-slip / symlink-poison from a hostile uploaded backup.
	safe, err := validateTarStream(opts.Reader)
	if err != nil {
		return fmt.Errorf("reject restore archive: %w", err)
	}

	// Extract tar into /tmp inside container
	extractCmd := exec.CommandContext(ctx, "docker", "exec", "-i", container,
		"tar", "-xzf", "-", "-C", "/", "--no-same-owner", "--no-overwrite-dir")
	extractCmd.Stdin = safe

	if out, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar extract: %w: %s", err, out)
	}

	// Copy each backup file to its original path
	for i, dbPath := range opts.Paths {
		tmpPath := fmt.Sprintf("/tmp/sd-backup-%d.db", i)
		cpCmd := exec.CommandContext(ctx, "docker", "exec", container,
			"cp", tmpPath, dbPath)
		if out, err := cpCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore %s: %w: %s", dbPath, err, out)
		}

		// Cleanup temp file
		_ = exec.CommandContext(ctx, "docker", "exec", container,
			"rm", "-f", tmpPath).Run()
	}

	return nil
}
