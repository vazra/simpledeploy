package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalTarget stores backups on the local filesystem.
type LocalTarget struct {
	BasePath string
}

func NewLocalTarget(basePath string) *LocalTarget {
	return &LocalTarget{BasePath: basePath}
}

func (t *LocalTarget) Upload(ctx context.Context, filename string, data io.Reader) (int64, error) {
	path := filepath.Join(t.BasePath, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	return io.Copy(f, data)
}

func (t *LocalTarget) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	f, err := os.Open(filepath.Join(t.BasePath, filename))
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

func (t *LocalTarget) Delete(ctx context.Context, filename string) error {
	if err := os.Remove(filepath.Join(t.BasePath, filename)); err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}
