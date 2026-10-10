package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWriteFileAtomicReplacesSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".env")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(link, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(outside)
	if string(got) != "original" {
		t.Fatalf("symlink target was modified: %q", got)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("expected regular file, got %v", fi.Mode())
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(link)
	if string(data) != "A=1\n" {
		t.Fatalf("content = %q", data)
	}
}

func TestReadRegularFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	os.WriteFile(target, []byte("x"), 0o600)
	link := filepath.Join(dir, "link")
	os.Symlink(target, link)

	if _, err := ReadRegularFile(link); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("want ErrNotRegular, got %v", err)
	}
	if _, err := ReadRegularFile(dir); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("dir: want ErrNotRegular, got %v", err)
	}
	data, err := ReadRegularFile(target)
	if err != nil || string(data) != "x" {
		t.Fatalf("regular read: %q %v", data, err)
	}
	if _, err := ReadRegularFile(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing: want not-exist, got %v", err)
	}
}

// A regular file swapped to a FIFO between the Lstat check and the open
// must be refused without blocking (no writer ever opens the FIFO).
func TestOpenRegularFileFIFOSwapDoesNotBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var swapErr error
	afterLstat = func(p string) {
		if swapErr = os.Remove(p); swapErr == nil {
			swapErr = syscall.Mkfifo(p, 0o600)
		}
	}
	t.Cleanup(func() { afterLstat = func(string) {} })

	done := make(chan error, 1)
	go func() {
		f, err := OpenRegularFile(path)
		if f != nil {
			f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if swapErr != nil {
			t.Skipf("mkfifo not supported: %v", swapErr)
		}
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("want ErrNotRegular, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OpenRegularFile blocked on a FIFO")
	}
}

func TestEnsureNoSymlinks(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "app"), 0o700)
	os.WriteFile(filepath.Join(root, "app", "docker-compose.yml"), []byte("x"), 0o600)
	if err := EnsureNoSymlinks(root, filepath.Join(root, "app", "docker-compose.yml")); err != nil {
		t.Fatalf("regular path: %v", err)
	}
	if err := EnsureNoSymlinks(root, filepath.Join(root, "app", "missing", "x")); err != nil {
		t.Fatalf("missing path: %v", err)
	}
	os.Symlink(t.TempDir(), filepath.Join(root, "linked"))
	if err := EnsureNoSymlinks(root, filepath.Join(root, "linked", "docker-compose.yml")); err == nil {
		t.Fatal("expected error for symlinked parent")
	}
	os.Symlink("/etc/hostname", filepath.Join(root, "app", ".env"))
	if err := EnsureNoSymlinks(root, filepath.Join(root, "app", ".env")); err == nil {
		t.Fatal("expected error for symlinked file")
	}
	if err := EnsureNoSymlinks(root, filepath.Join(root, "..", "x")); err == nil {
		t.Fatal("expected error for path outside root")
	}
}
