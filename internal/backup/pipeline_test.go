package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestPipelineBackupSuccess(t *testing.T) {
	strategy := &mockStrategy{data: "hello-backup", filename: "dump.sql.gz"}
	target := newMockTarget()
	pipe := NewPipeline(strategy, target, nil)

	result, err := pipe.RunBackup(context.Background(), BackupOpts{ContainerName: "db"}, nil, nil)
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	if result.FilePath != "dump.sql.gz" {
		t.Errorf("FilePath = %q, want dump.sql.gz", result.FilePath)
	}
	if result.SizeBytes != int64(len("hello-backup")) {
		t.Errorf("SizeBytes = %d, want %d", result.SizeBytes, len("hello-backup"))
	}
	if result.Checksum == "" {
		t.Error("Checksum is empty")
	}

	// verify uploaded data
	data, ok := target.uploaded["dump.sql.gz"]
	if !ok {
		t.Fatal("file not uploaded to target")
	}
	if string(data) != "hello-backup" {
		t.Errorf("uploaded data = %q, want hello-backup", string(data))
	}
}

func TestPipelineBackupStrategyError(t *testing.T) {
	strategy := &mockStrategy{err: fmt.Errorf("disk full")}
	target := newMockTarget()
	pipe := NewPipeline(strategy, target, nil)

	result, err := pipe.RunBackup(context.Background(), BackupOpts{}, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Error("expected nil result on error")
	}
	if !strings.Contains(err.Error(), "backup") {
		t.Errorf("error = %q, should contain 'backup'", err.Error())
	}
	if len(target.uploaded) != 0 {
		t.Error("target should have no uploads on strategy error")
	}
}

func TestPipelineBackupUploadError(t *testing.T) {
	strategy := &mockStrategy{data: "data", filename: "f.tar"}
	target := newMockTarget()
	target.err = fmt.Errorf("s3 down")
	pipe := NewPipeline(strategy, target, nil)

	_, err := pipe.RunBackup(context.Background(), BackupOpts{}, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "upload") {
		t.Errorf("error = %q, should contain 'upload'", err.Error())
	}
}

func TestPipelineRestoreSuccess(t *testing.T) {
	data := "restore-data"
	strategy := &mockStrategy{data: data, filename: "dump.sql.gz"}
	target := newMockTarget()
	pipe := NewPipeline(strategy, target, nil)

	// first backup to get checksum and file in target
	result, err := pipe.RunBackup(context.Background(), BackupOpts{ContainerName: "db"}, nil, nil)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// restore with checksum verification
	err = pipe.RunRestore(context.Background(), RestoreOpts{ContainerName: "db"}, result.FilePath, result.Checksum, nil, nil)
	if err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
}

func TestPipelineRestoreChecksumMismatch(t *testing.T) {
	strategy := &mockStrategy{data: "data", filename: "f.tar"}
	target := newMockTarget()

	// manually put data in target
	target.uploaded["f.tar"] = []byte("data")

	pipe := NewPipeline(strategy, target, nil)

	err := pipe.RunRestore(context.Background(), RestoreOpts{ContainerName: "db"}, "f.tar", "badhash", nil, nil)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %q, should contain 'checksum mismatch'", err.Error())
	}
}

func TestPipelineRestoreNoChecksum(t *testing.T) {
	// capture what Restore receives
	var restoredData []byte
	strategy := &restoreCapture{captured: &restoredData}
	target := newMockTarget()
	target.uploaded["f.tar"] = []byte("raw-data")

	pipe := NewPipeline(strategy, target, nil)

	err := pipe.RunRestore(context.Background(), RestoreOpts{ContainerName: "db"}, "f.tar", "", nil, nil)
	if err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
}

// restoreCapture is a strategy that captures data passed to Restore.
type restoreCapture struct {
	captured *[]byte
}

func (s *restoreCapture) Type() string                                    { return "capture" }
func (s *restoreCapture) Detect(cfg *compose.AppConfig) []DetectedService { return nil }
func (s *restoreCapture) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *restoreCapture) Restore(ctx context.Context, opts RestoreOpts) error {
	data, _ := io.ReadAll(opts.Reader)
	*s.captured = data
	return nil
}

// readerCapture records what Restore receives.
type readerCapture struct {
	data     []byte
	spool    *spoolFile
	maxBytes int64
}

func (s *readerCapture) Type() string                                    { return "capture" }
func (s *readerCapture) Detect(cfg *compose.AppConfig) []DetectedService { return nil }
func (s *readerCapture) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *readerCapture) Restore(ctx context.Context, opts RestoreOpts) error {
	s.spool, _ = opts.Reader.(*spoolFile)
	s.maxBytes = opts.MaxDecompressedBytes
	s.data, _ = io.ReadAll(opts.Reader)
	return nil
}

func checksumOf(t *testing.T, data string) string {
	t.Helper()
	cw := NewChecksumWriter()
	io.Copy(io.Discard, cw.TeeReader(strings.NewReader(data)))
	return cw.Sum()
}

func TestPipelineRestore_StreamsVerifiedSpool(t *testing.T) {
	dir := spoolDir(t)
	target := newMockTarget()
	target.uploaded["f.tar.gz"] = []byte("archive-bytes")
	strategy := &readerCapture{}
	pipe := NewPipeline(strategy, target, nil)

	opts := RestoreOpts{ContainerName: "db", MaxDecompressedBytes: NoDecompressedLimit}
	if err := pipe.RunRestore(context.Background(), opts, "f.tar.gz", checksumOf(t, "archive-bytes"), nil, nil); err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if strategy.spool == nil {
		t.Fatal("strategy did not get the staged spool file")
	}
	if _, err := strategy.spool.f.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Errorf("spool still open after restore (stat err = %v)", err)
	}
	if string(strategy.data) != "archive-bytes" {
		t.Errorf("strategy read %q, want archive-bytes", strategy.data)
	}
	if strategy.maxBytes != NoDecompressedLimit {
		t.Errorf("MaxDecompressedBytes = %d, want %d", strategy.maxBytes, NoDecompressedLimit)
	}
	assertDirEmpty(t, dir)
}

func TestPipelineRestore_ChecksumMismatchSpoolRemoved(t *testing.T) {
	dir := spoolDir(t)
	target := newMockTarget()
	target.uploaded["f.tar.gz"] = []byte("tampered")
	strategy := &readerCapture{}
	pipe := NewPipeline(strategy, target, nil)

	err := pipe.RunRestore(context.Background(), RestoreOpts{}, "f.tar.gz", checksumOf(t, "original"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
	if strategy.data != nil {
		t.Error("strategy ran despite checksum mismatch")
	}
	assertDirEmpty(t, dir)
}

func TestPipelineRestore_RawSizeCap(t *testing.T) {
	dir := spoolDir(t)
	target := newMockTarget()
	data := strings.Repeat("x", 4<<10)
	target.uploaded["f.tar.gz"] = []byte(data)
	strategy := &readerCapture{}
	pipe := NewPipeline(strategy, target, nil)

	err := pipe.RunRestore(context.Background(), RestoreOpts{MaxDecompressedBytes: 1 << 10}, "f.tar.gz", checksumOf(t, data), nil, nil)
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("want errArchiveTooLarge, got %v", err)
	}
	if strategy.data != nil {
		t.Error("strategy ran for an oversize archive")
	}
	assertDirEmpty(t, dir)

	if err := pipe.RunRestore(context.Background(), RestoreOpts{MaxDecompressedBytes: NoDecompressedLimit}, "f.tar.gz", checksumOf(t, data), nil, nil); err != nil {
		t.Fatalf("uncapped restore: %v", err)
	}
}

func TestPipelineRestore_SpoolDir(t *testing.T) {
	dir := t.TempDir()
	setSpoolDir(t, dir)
	target := newMockTarget()
	target.uploaded["f"] = []byte("abc")
	strategy := &readerCapture{}
	pipe := NewPipeline(strategy, target, nil)
	if err := pipe.RunRestore(context.Background(), RestoreOpts{}, "f", checksumOf(t, "abc"), nil, nil); err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	assertDirEmpty(t, dir)
}
