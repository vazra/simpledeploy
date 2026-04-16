package backup

import (
	"context"
	"io"
)

type Target interface {
	Type() string
	Upload(ctx context.Context, filename string, data io.Reader) (path string, size int64, err error)
	Download(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Test(ctx context.Context) error
}

type TargetFactory func(configJSON string) (Target, error)
