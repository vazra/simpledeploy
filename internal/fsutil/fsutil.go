// Package fsutil holds small filesystem helpers that refuse to follow
// symlinks. App directories can contain files that did not come from
// SimpleDeploy (git sync pulls, container bind mounts), so reads and writes
// of managed files (docker-compose.yml, .env, sidecars) must never be
// redirected to a path outside the app directory.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrNotRegular is returned when a path is a symlink, directory, device or
// any other non-regular file.
var ErrNotRegular = errors.New("not a regular file")

// WriteFileAtomic writes data to a temp file in the same directory and
// renames it over path. Rename replaces a symlink at path instead of
// following it, so the write can never land outside the directory.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { _ = os.Remove(tmp) }
	if err := f.Chmod(perm); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// afterLstat is a test hook run between the Lstat check and the open.
var afterLstat = func(string) {}

// OpenRegularFile opens path read-only, refusing symlinks and non-regular
// files. Callers must close the returned file.
func OpenRegularFile(path string) (*os.File, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	afterLstat(path)
	// O_NONBLOCK: a path swapped to a FIFO after the Lstat must not block
	// the open. It has no effect on reads of regular files.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	// Re-check after open to close the Lstat/Open race.
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return f, nil
}

// ReadRegularFile reads path, refusing symlinks and non-regular files.
func ReadRegularFile(path string) ([]byte, error) {
	f, err := OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// EnsureNoSymlinks verifies that every existing path component of target
// below root (including target itself) is not a symlink. Components that
// do not exist yet are allowed. target must be inside root.
func EnsureNoSymlinks(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside %s", target, root)
	}
	if rel == "." {
		return nil
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", cur)
		}
	}
	return nil
}
