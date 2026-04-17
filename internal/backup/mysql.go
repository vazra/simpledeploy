package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

var mysqlKeywords = []string{"mysql", "mariadb", "percona"}

// MySQLStrategy backs up and restores MySQL/MariaDB via mysqldump.
type MySQLStrategy struct{}

func NewMySQLStrategy() *MySQLStrategy {
	return &MySQLStrategy{}
}

func (s *MySQLStrategy) Type() string { return "mysql" }

func (s *MySQLStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var results []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "mysql") || matchesImageKeywords(svc.Image, mysqlKeywords) {
			results = append(results, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: cfg.Name + "-" + svc.Name + "-1",
				Label:         "mysql",
			})
		}
	}
	return results
}

func (s *MySQLStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	filename := fmt.Sprintf("%s-%s.sql.gz", opts.ContainerName, time.Now().Format("20060102-150405"))

	// Read MYSQL_ROOT_PASSWORD from the container env at dump time, falling
	// back to opts.Credentials if the caller supplied one explicitly.
	script := `set -e
pw="${MYSQL_ROOT_PASSWORD:-}"
if [ -n "$pw" ]; then
  exec mysqldump --all-databases -u root -p"$pw"
else
  exec mysqldump --all-databases -u root
fi`
	var envArgs []string
	if pw := opts.Credentials["MYSQL_ROOT_PASSWORD"]; pw != "" {
		envArgs = []string{"-e", "MYSQL_ROOT_PASSWORD=" + pw}
	}

	args := append([]string{"exec"}, envArgs...)
	args = append(args, opts.ContainerName, "sh", "-c", script)
	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mysqldump: %w", err)
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
			pw.CloseWithError(fmt.Errorf("mysqldump: %w", waitErr))
			return
		}
		pw.Close()
	}()

	return &BackupResult{Reader: pr, Filename: filename}, nil
}

func (s *MySQLStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	gr, err := gzip.NewReader(opts.Reader)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	script := `set -e
pw="${MYSQL_ROOT_PASSWORD:-}"
if [ -n "$pw" ]; then
  exec mysql -u root -p"$pw"
else
  exec mysql -u root
fi`
	var envArgs []string
	if pw := opts.Credentials["MYSQL_ROOT_PASSWORD"]; pw != "" {
		envArgs = []string{"-e", "MYSQL_ROOT_PASSWORD=" + pw}
	}

	args := append([]string{"exec", "-i"}, envArgs...)
	args = append(args, opts.ContainerName, "sh", "-c", script)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = gr

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mysql restore: %w: %s", err, truncateOutput(out))
	}
	return nil
}
