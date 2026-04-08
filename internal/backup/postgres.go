package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// PostgresStrategy backs up and restores a Postgres container via pg_dump/psql.
type PostgresStrategy struct{}

func NewPostgresStrategy() *PostgresStrategy {
	return &PostgresStrategy{}
}

func (s *PostgresStrategy) Backup(ctx context.Context, containerName string) (io.ReadCloser, string, error) {
	filename := fmt.Sprintf("%s-%s.sql.gz", containerName, time.Now().Format("20060102-150405"))

	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "pg_dump", "-U", "postgres")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start pg_dump: %w", err)
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

	return pr, filename, nil
}

func (s *PostgresStrategy) Restore(ctx context.Context, containerName string, data io.Reader) error {
	gr, err := gzip.NewReader(data)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "psql", "-U", "postgres")
	cmd.Stdin = gr

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("psql: %w: %s", err, out)
	}
	return nil
}
