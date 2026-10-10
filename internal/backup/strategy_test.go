package backup

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestRestoreMaxBytesEnv(t *testing.T) {
	cases := []struct {
		env        string
		upload     int64
		backupRuns int64
	}{
		{"", defaultMaxDecompressed, NoDecompressedLimit},
		{"2", 2 << 30, 2 << 30},
		{" 20 ", 20 << 30, 20 << 30},
		// Invalid values fall back to the default cap.
		{"0", defaultMaxDecompressed, defaultMaxDecompressed},
		{"-1", defaultMaxDecompressed, defaultMaxDecompressed},
		{"1.5", defaultMaxDecompressed, defaultMaxDecompressed},
		{"lots", defaultMaxDecompressed, defaultMaxDecompressed},
		{"99999999999", defaultMaxDecompressed, defaultMaxDecompressed},
	}
	for _, tc := range cases {
		t.Setenv(RestoreMaxGBEnv, tc.env)
		if got := UploadRestoreMaxBytes(); got != tc.upload {
			t.Errorf("%s=%q: UploadRestoreMaxBytes = %d, want %d", RestoreMaxGBEnv, tc.env, got, tc.upload)
		}
		if got := runRestoreMaxBytes(); got != tc.backupRuns {
			t.Errorf("%s=%q: runRestoreMaxBytes = %d, want %d", RestoreMaxGBEnv, tc.env, got, tc.backupRuns)
		}
	}
}

func TestRestoreCap(t *testing.T) {
	if n, ok := restoreCap(0); !ok || n != defaultMaxDecompressed {
		t.Errorf("restoreCap(0) = %d, %v", n, ok)
	}
	if n, ok := restoreCap(10); !ok || n != 10 {
		t.Errorf("restoreCap(10) = %d, %v", n, ok)
	}
	if _, ok := restoreCap(NoDecompressedLimit); ok {
		t.Error("restoreCap(NoDecompressedLimit) should disable the cap")
	}
}

func TestLimitedGzip(t *testing.T) {
	src := bytes.Repeat([]byte("x"), 100)
	// Oversize input fails instead of being cut short.
	if _, err := io.ReadAll(limitedGzip(bytes.NewReader(src), 10)); !errors.Is(err, errArchiveTooLarge) {
		t.Errorf("capped read: err = %v, want errArchiveTooLarge", err)
	}
	if out, err := io.ReadAll(limitedGzip(bytes.NewReader(src), 100)); err != nil || len(out) != 100 {
		t.Errorf("read at cap = %d bytes, %v; want 100, nil", len(out), err)
	}
	if out, err := io.ReadAll(limitedGzip(bytes.NewReader(src), NoDecompressedLimit)); err != nil || len(out) != 100 {
		t.Errorf("uncapped read = %d bytes, %v; want 100, nil", len(out), err)
	}
}
