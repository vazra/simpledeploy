package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalTarget stores backups on the local filesystem.
type LocalTarget struct {
	dir string
}

func NewLocalTarget(dir string) *LocalTarget {
	return &LocalTarget{dir: dir}
}

func (t *LocalTarget) Type() string { return "local" }

func (t *LocalTarget) Test(ctx context.Context) error {
	info, err := os.Stat(t.dir)
	if err != nil {
		return fmt.Errorf("backup directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", t.dir)
	}
	return nil
}

func (t *LocalTarget) Upload(ctx context.Context, filename string, data io.Reader) (string, int64, error) {
	if strings.Contains(filename, "..") || filepath.IsAbs(filename) {
		return "", 0, fmt.Errorf("invalid filename: %s", filename)
	}
	path := filepath.Join(t.dir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", 0, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	n, err := io.Copy(f, data)
	if err != nil {
		return "", 0, fmt.Errorf("write: %w", err)
	}
	return filename, n, nil
}

func (t *LocalTarget) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	if strings.Contains(path, "..") || filepath.IsAbs(path) {
		return nil, fmt.Errorf("invalid path: %s", path)
	}
	f, err := os.Open(filepath.Join(t.dir, path))
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	return f, nil
}

func (t *LocalTarget) Delete(ctx context.Context, path string) error {
	if strings.Contains(path, "..") || filepath.IsAbs(path) {
		return fmt.Errorf("invalid path: %s", path)
	}
	return os.Remove(filepath.Join(t.dir, path))
}

// FilePath returns absolute filesystem path for download streaming.
func (t *LocalTarget) FilePath(filename string) string {
	return filepath.Join(t.dir, filename)
}
