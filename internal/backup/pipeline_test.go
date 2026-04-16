package backup

import (
	"context"
	"fmt"
	"io"
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
