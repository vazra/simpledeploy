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

// Helper functions shared by all strategies.

func truncateOutput(b []byte) []byte {
	if len(b) > 500 {
		return b[:500]
	}
	return b
}

func matchesLabel(labels map[string]string, strategy string) bool {
	if labels == nil {
		return false
	}
	return labels["simpledeploy.backup.strategy"] == strategy
}

func matchesImageKeywords(image string, keywords []string) bool {
	lower := strings.ToLower(image)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

var postgresKeywords = []string{"postgres", "postgis", "timescale", "supabase"}

// PostgresStrategy backs up and restores a Postgres container via pg_dump/psql.
type PostgresStrategy struct{}

func NewPostgresStrategy() *PostgresStrategy {
	return &PostgresStrategy{}
}

func (s *PostgresStrategy) Type() string { return "postgres" }

func (s *PostgresStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var results []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "postgres") || matchesImageKeywords(svc.Image, postgresKeywords) {
			results = append(results, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: cfg.Name + "-" + svc.Name + "-1",
				Label:         "postgres",
			})
		}
	}
	return results
}

func (s *PostgresStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	filename := fmt.Sprintf("%s-%s.sql.gz", opts.ContainerName, time.Now().Format("20060102-150405"))

	// Resolve the Postgres user + database from the container environment.
	// Falls back to POSTGRES_USER=postgres and POSTGRES_DB=$POSTGRES_USER to
	// match upstream postgres image defaults. Using the container env lets us
	// back up the right database without the caller knowing the credentials.
	script := `set -e
user="${POSTGRES_USER:-postgres}"
db="${POSTGRES_DB:-$user}"
exec pg_dump -U "$user" -d "$db"`
	var envArgs []string
	if u := opts.Credentials["POSTGRES_USER"]; u != "" {
		envArgs = []string{"-e", "POSTGRES_USER=" + u}
	}

	args := append([]string{"exec"}, envArgs...)
	args = append(args, opts.ContainerName, "sh", "-c", script)
	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pg_dump: %w", err)
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
			pw.CloseWithError(fmt.Errorf("pg_dump: %w", waitErr))
			return
		}
		pw.Close()
	}()

	return &BackupResult{Reader: pr, Filename: filename}, nil
}

func (s *PostgresStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	gr, err := gzip.NewReader(opts.Reader)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	script := `set -e
user="${POSTGRES_USER:-postgres}"
db="${POSTGRES_DB:-$user}"
exec psql -U "$user" -d "$db"`
	var envArgs []string
	if u := opts.Credentials["POSTGRES_USER"]; u != "" {
		envArgs = []string{"-e", "POSTGRES_USER=" + u}
	}

	args := append([]string{"exec", "-i"}, envArgs...)
	args = append(args, opts.ContainerName, "sh", "-c", script)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = gr

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("psql: %w: %s", err, truncateOutput(out))
	}
	return nil
}
