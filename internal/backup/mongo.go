package backup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

var mongoKeywords = []string{"mongo"}

// cmdReadCloser wraps an io.ReadCloser from a command's stdout and waits
// for the command to finish on Close. Reusable by any strategy that
// streams directly from docker exec.
type cmdReadCloser struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (c *cmdReadCloser) Close() error {
	err := c.ReadCloser.Close()
	waitErr := c.cmd.Wait()
	if err != nil {
		return err
	}
	return waitErr
}

// MongoStrategy backs up and restores MongoDB via mongodump/mongorestore.
type MongoStrategy struct{}

func NewMongoStrategy() *MongoStrategy {
	return &MongoStrategy{}
}

func (s *MongoStrategy) Type() string { return "mongo" }

func (s *MongoStrategy) Detect(cfg *compose.AppConfig) []DetectedService {
	var results []DetectedService
	for _, svc := range cfg.Services {
		if matchesLabel(svc.Labels, "mongo") || matchesImageKeywords(svc.Image, mongoKeywords) {
			results = append(results, DetectedService{
				ServiceName:   svc.Name,
				ContainerName: cfg.Name + "-" + svc.Name + "-1",
				Label:         "mongo",
			})
		}
	}
	return results
}

func (s *MongoStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	filename := fmt.Sprintf("%s-%s.archive.gz", opts.ContainerName, time.Now().Format("20060102-150405"))

	// Read root credentials from container env at dump time; caller can
	// override via opts.Credentials.
	script := `set -e
u="${MONGO_INITDB_ROOT_USERNAME:-}"
p="${MONGO_INITDB_ROOT_PASSWORD:-}"
if [ -n "$u" ] && [ -n "$p" ]; then
  exec mongodump --archive --gzip --authenticationDatabase admin -u "$u" -p "$p"
else
  exec mongodump --archive --gzip
fi`
	if u := opts.Credentials["MONGO_INITDB_ROOT_USERNAME"]; u != "" {
		p := opts.Credentials["MONGO_INITDB_ROOT_PASSWORD"]
		script = fmt.Sprintf(`exec mongodump --archive --gzip --authenticationDatabase admin -u %q -p %q`, u, p)
	}

	cmd := exec.CommandContext(ctx, "docker", "exec", opts.ContainerName, "sh", "-c", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mongodump: %w", err)
	}

	return &BackupResult{
		Reader:   &cmdReadCloser{ReadCloser: stdout, cmd: cmd},
		Filename: filename,
	}, nil
}

func (s *MongoStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	script := `set -e
u="${MONGO_INITDB_ROOT_USERNAME:-}"
p="${MONGO_INITDB_ROOT_PASSWORD:-}"
if [ -n "$u" ] && [ -n "$p" ]; then
  exec mongorestore --archive --gzip --drop --authenticationDatabase admin -u "$u" -p "$p"
else
  exec mongorestore --archive --gzip --drop
fi`
	if u := opts.Credentials["MONGO_INITDB_ROOT_USERNAME"]; u != "" {
		p := opts.Credentials["MONGO_INITDB_ROOT_PASSWORD"]
		script = fmt.Sprintf(`exec mongorestore --archive --gzip --drop --authenticationDatabase admin -u %q -p %q`, u, p)
	}

	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", opts.ContainerName, "sh", "-c", script)
	cmd.Stdin = opts.Reader

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mongorestore: %w: %s", err, out)
	}
	return nil
}
