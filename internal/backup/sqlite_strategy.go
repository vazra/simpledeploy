package backup

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
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
				paths = append(paths, path.Clean(v.Target))
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
	if err := ValidatePaths("sqlite", opts.Paths); err != nil {
		return nil, err
	}

	// Use sqlite3 .backup for each path to get a consistent snapshot.
	// dbPath and the dot-command are separate argv elements (no shell), so
	// spaces and other characters ValidateSQLitePath allows need no
	// escaping beyond the single quotes in sqliteBackupDotCmd.
	var backupPaths []string
	for _, dbPath := range opts.Paths {
		tmpPath := sqliteTmpPath(dbPath)
		cmd := exec.CommandContext(ctx, "docker", "exec", container,
			"sqlite3", dbPath, sqliteBackupDotCmd(tmpPath))
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("sqlite3 backup %s: %w: %s", dbPath, err, out)
		}
		backupPaths = append(backupPaths, tmpPath)
	}

	// Tar+gzip all backup files and stream out. Entries are relative
	// (validateTarStream on restore rejects absolute paths to block
	// tar-slip from a hostile uploaded backup); see tarCreateArgs.
	cmd := exec.CommandContext(ctx, "docker", tarCreateArgs(container, backupPaths)...)
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
	if err := ValidatePaths("sqlite", opts.Paths); err != nil {
		return err
	}

	// Validate the archive before handing it to the in-container tar to
	// block tar-slip / symlink-poison from a hostile uploaded backup.
	safe, err := validateTarStream(opts.Reader, opts.MaxDecompressedBytes)
	if err != nil {
		return fmt.Errorf("reject restore archive: %w", err)
	}
	defer safe.Close()

	// Extract tar inside container. validateTarStream above rejects the
	// symlink/hardlink/parent-traversal/absolute-path vectors that
	// --no-same-owner/--no-overwrite-dir were guarding against; keep the
	// extract flags portable so BusyBox tar (Alpine) works too.
	extractCmd := exec.CommandContext(ctx, "docker", "exec", "-i", container,
		"tar", "-xzf", "-", "-C", "/")
	extractCmd.Stdin = safe

	if out, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar extract: %w: %s", err, out)
	}

	// Copy each backup file to its original path. The tmp paths match the
	// Backup() side; see sqliteTmpPath.
	for _, dbPath := range opts.Paths {
		tmpPath := sqliteTmpPath(dbPath)
		cpCmd := exec.CommandContext(ctx, "docker", "exec", container,
			"cp", "--", tmpPath, dbPath)
		if out, err := cpCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore %s: %w: %s", dbPath, err, out)
		}

		// Cleanup temp file
		_ = exec.CommandContext(ctx, "docker", "exec", container,
			"rm", "-f", "--", tmpPath).Run()
	}

	return nil
}

// sqliteTmpPath is the in-container snapshot path for dbPath. It is derived
// from the source file's basename so concurrent runs against different
// databases do not collide. Predictable but inside the container's /tmp,
// which is in our trust boundary.
func sqliteTmpPath(dbPath string) string {
	return "/tmp/sd-backup-" + filepath.Base(dbPath)
}

// sqliteBackupDotCmd builds the sqlite3 '.backup' dot-command. sqlite3
// takes a single-quoted dot-command argument literally up to the next
// single quote, so spaces and '@' are fine; ValidateSQLitePath refuses
// quotes (and backslash, backtick, '$') so the argument cannot be closed
// early.
func sqliteBackupDotCmd(tmpPath string) string {
	return ".backup '" + tmpPath + "'"
}
