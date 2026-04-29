package backup

import (
	"context"
	"io"

	"github.com/vazra/simpledeploy/internal/compose"
)

type Strategy interface {
	Type() string
	Detect(cfg *compose.AppConfig) []DetectedService
	Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error)
	Restore(ctx context.Context, opts RestoreOpts) error
}

type DetectedService struct {
	ServiceName   string            `json:"service_name"`
	ContainerName string            `json:"container_name"`
	Label         string            `json:"label"`
	Paths         []string          `json:"paths,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type BackupOpts struct {
	ContainerName string
	Paths         []string
	Credentials   map[string]string
}

type BackupResult struct {
	Reader   io.ReadCloser
	Filename string
}

type RestoreOpts struct {
	ContainerName string
	Paths         []string
	Credentials   map[string]string
	Reader        io.ReadCloser
	// MaxDecompressedBytes caps the total bytes a strategy is willing to
	// produce from the (potentially gzipped) Reader. 0 means use the default
	// from defaultMaxDecompressed. Set explicitly to override.
	MaxDecompressedBytes int64
}

// defaultMaxDecompressed bounds gzip decompression on restore paths to
// guard against compression-bomb DoS. 8 GiB is high enough to cover real
// large-DB dumps but low enough that the host disk does not fill silently.
const defaultMaxDecompressed = 8 << 30

// limitedGzip wraps gr in an io.LimitReader using max (or the default cap
// when max <= 0). The returned reader exposes Close so callers can keep
// their existing defer gr.Close() pattern via the underlying gzip reader.
func limitedGzip(gr io.Reader, max int64) io.Reader {
	if max <= 0 {
		max = defaultMaxDecompressed
	}
	return io.LimitReader(gr, max)
}
