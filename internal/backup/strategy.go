package backup

import (
	"context"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

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
	// MaxDecompressedBytes caps the bytes a strategy produces from the
	// (possibly gzipped) Reader, and the raw archive size where it is
	// staged on disk. 0 means defaultMaxDecompressed; NoDecompressedLimit
	// (any negative value) disables the cap.
	MaxDecompressedBytes int64
}

// NoDecompressedLimit as RestoreOpts.MaxDecompressedBytes disables the
// restore size cap.
const NoDecompressedLimit int64 = -1

// defaultMaxDecompressed bounds gzip decompression on restore paths to
// guard against compression-bomb DoS. 8 GiB is high enough to cover real
// large-DB dumps but low enough that the host disk does not fill silently.
const defaultMaxDecompressed = 8 << 30

// RestoreMaxGBEnv overrides the restore size cap, in whole GiB.
const RestoreMaxGBEnv = "SIMPLEDEPLOY_RESTORE_MAX_GB"

// restoreCap maps a MaxDecompressedBytes value to a byte cap. capped is
// false when the value disables the cap.
func restoreCap(max int64) (limit int64, capped bool) {
	switch {
	case max < 0:
		return 0, false
	case max == 0:
		return defaultMaxDecompressed, true
	default:
		return max, true
	}
}

// restoreMaxFromEnv reads RestoreMaxGBEnv. set is false when it is unset.
// An invalid value falls back to the default cap.
func restoreMaxFromEnv() (limit int64, set bool) {
	v := strings.TrimSpace(os.Getenv(RestoreMaxGBEnv))
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 || n > math.MaxInt64>>30 {
		log.Printf("[backup] %s=%q is not a whole number of GiB above 0; using the default %d GiB restore limit",
			RestoreMaxGBEnv, v, defaultMaxDecompressed>>30)
		return defaultMaxDecompressed, true
	}
	return n << 30, true
}

// UploadRestoreMaxBytes is the size cap for archives uploaded through the
// API: RestoreMaxGBEnv when set, else 8 GiB.
func UploadRestoreMaxBytes() int64 {
	if n, ok := restoreMaxFromEnv(); ok {
		return n
	}
	return defaultMaxDecompressed
}

// runRestoreMaxBytes is the size cap for restoring SimpleDeploy's own
// backup runs: RestoreMaxGBEnv when set, else no cap.
func runRestoreMaxBytes() int64 {
	if n, ok := restoreMaxFromEnv(); ok {
		return n
	}
	return NoDecompressedLimit
}

// limitedGzip caps gr at the limit from max (see restoreCap), failing
// with errArchiveTooLarge past it rather than truncating. With the cap
// disabled gr is returned as is.
func limitedGzip(gr io.Reader, max int64) io.Reader {
	limit, capped := restoreCap(max)
	if !capped {
		return gr
	}
	return newCapReader(gr, limit)
}
