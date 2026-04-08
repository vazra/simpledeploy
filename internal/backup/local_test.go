package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func TestLocalTargetImplementsInterface(t *testing.T) {
	var _ Target = NewLocalTarget("/tmp")
}

func TestLocalTargetUploadDownload(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	data := []byte("hello backup world")
	n, err := target.Upload(ctx, "test.txt", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if n != int64(len(data)) {
		t.Errorf("uploaded %d bytes, want %d", n, len(data))
	}

	rc, err := target.Download(ctx, "test.txt")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestLocalTargetUploadCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	_, err := target.Upload(ctx, "subdir/nested/file.gz", bytes.NewReader([]byte("data")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
}

func TestLocalTargetDelete(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	_, err := target.Upload(ctx, "del.txt", bytes.NewReader([]byte("bye")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if err := target.Delete(ctx, "del.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = target.Download(ctx, "del.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist after delete, got %v", err)
	}
}

func TestLocalTargetDownloadMissing(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	_, err := target.Download(ctx, "nonexistent.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}
