package backup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalTarget_Type(t *testing.T) {
	lt := NewLocalTarget("/tmp")
	if lt.Type() != "local" {
		t.Errorf("Type() = %q, want %q", lt.Type(), "local")
	}
}

func TestLocalTarget_Test(t *testing.T) {
	// Valid directory passes
	dir := t.TempDir()
	lt := NewLocalTarget(dir)
	if err := lt.Test(context.Background()); err != nil {
		t.Errorf("Test() on valid dir: %v", err)
	}

	// Nonexistent directory fails
	lt2 := NewLocalTarget("/tmp/nonexistent-simpledeploy-test-dir-12345")
	if err := lt2.Test(context.Background()); err == nil {
		t.Error("Test() on nonexistent dir should fail")
	}
}

func TestLocalTarget_UploadReturnsPath(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	data := []byte("hello backup world")
	path, size, err := target.Upload(ctx, "test.txt", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if path != "test.txt" {
		t.Errorf("path = %q, want %q", path, "test.txt")
	}
	if size != int64(len(data)) {
		t.Errorf("size = %d, want %d", size, len(data))
	}

	// Verify file exists on disk with correct content
	content, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Equal(content, data) {
		t.Errorf("file content = %q, want %q", content, data)
	}
}

func TestLocalTarget_ImplementsInterface(t *testing.T) {
	var _ Target = NewLocalTarget("/tmp")
}

func TestLocalTarget_UploadDownload(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	data := []byte("hello backup world")
	_, _, err := target.Upload(ctx, "test.txt", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("upload: %v", err)
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

func TestLocalTarget_UploadCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	_, _, err := target.Upload(ctx, "subdir/nested/file.gz", bytes.NewReader([]byte("data")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
}

func TestLocalTarget_Delete(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	_, _, err := target.Upload(ctx, "del.txt", bytes.NewReader([]byte("bye")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if err := target.Delete(ctx, "del.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = target.Download(ctx, "del.txt")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestLocalTarget_DownloadMissing(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	_, err := target.Download(ctx, "nonexistent.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLocalTarget_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	target := NewLocalTarget(dir)
	ctx := context.Background()

	_, _, err := target.Upload(ctx, "../escape.txt", bytes.NewReader([]byte("bad")))
	if err == nil {
		t.Error("upload with .. should fail")
	}

	_, err = target.Download(ctx, "../escape.txt")
	if err == nil {
		t.Error("download with .. should fail")
	}

	err = target.Delete(ctx, "../escape.txt")
	if err == nil {
		t.Error("delete with .. should fail")
	}
}

func TestLocalTarget_FilePath(t *testing.T) {
	lt := NewLocalTarget("/data/backups")
	got := lt.FilePath("app/backup.tar.gz")
	want := filepath.Join("/data/backups", "app/backup.tar.gz")
	if got != want {
		t.Errorf("FilePath() = %q, want %q", got, want)
	}
}
