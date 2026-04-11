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
	BasePath string
}

func NewLocalTarget(basePath string) *LocalTarget {
	return &LocalTarget{BasePath: basePath}
}

// validateBackupFilename rejects filenames with path traversal or separators.
func validateBackupFilename(filename string) error {
	if strings.Contains(filename, "..") {
		return fmt.Errorf("invalid backup filename: contains '..'")
	}
	clean := filepath.Clean(filename)
	if filepath.IsAbs(clean) {
		return fmt.Errorf("invalid backup filename: absolute path")
	}
	return nil
}

func (t *LocalTarget) Upload(ctx context.Context, filename string, data io.Reader) (int64, error) {
	if err := validateBackupFilename(filename); err != nil {
		return 0, err
	}
	path := filepath.Join(t.BasePath, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	return io.Copy(f, data)
}

func (t *LocalTarget) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	if err := validateBackupFilename(filename); err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(t.BasePath, filename))
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

func (t *LocalTarget) Delete(ctx context.Context, filename string) error {
	if err := validateBackupFilename(filename); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(t.BasePath, filename)); err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}
