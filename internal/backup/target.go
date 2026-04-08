package backup

import (
	"context"
	"io"
)

// Target defines where backup data is stored and retrieved from.
type Target interface {
	Upload(ctx context.Context, filename string, data io.Reader) (int64, error)
	Download(ctx context.Context, filename string) (io.ReadCloser, error)
	Delete(ctx context.Context, filename string) error
}
