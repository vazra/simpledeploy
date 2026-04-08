package backup

import (
	"context"
	"io"
)

// Strategy defines how to backup and restore a container's data.
// Backup returns a data stream, a suggested filename, and an error.
type Strategy interface {
	Backup(ctx context.Context, containerName string) (io.ReadCloser, string, error)
	Restore(ctx context.Context, containerName string, data io.Reader) error
}
