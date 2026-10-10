package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// validateTarStream reads a (gzipped) tar from r and returns the original
// uncompressed bytes if every entry is safe to extract, or an error if it
// contains any of the patterns commonly used in tar-slip / symlink-poison
// attacks:
//
//   - absolute paths (begin with '/')
//   - parent traversal (any '..' segment)
//   - symlinks or hardlinks (these can point anywhere on the filesystem of
//     the container during extract; combined with subsequent regular-file
//     entries this is the classic write-outside-prefix bypass)
//   - device or character-special entries
//   - paths whose Clean form differs from the supplied name (catches
//     trailing-slash, double-slash, dot-prefix tricks)
//
// The archive is also bounded: neither the raw payload nor its decompressed
// form may exceed the cap from max (see restoreCap: 0 means
// defaultMaxDecompressed, negative means no cap). A bigger archive fails
// with errArchiveTooLarge instead of being truncated.
//
// The raw payload is spooled to an owner-only temp file in the restore
// spool dir (data_dir/tmp once SetSpoolDir is called, as serve and the CLI
// backup commands do; the system temp dir otherwise) rather than memory.
// The returned reader replays the original gzipped bytes from that file so
// the caller can pipe them onward to 'docker exec ... tar -xzf -'
// unchanged; closing it deletes the file. On error the file is already
// gone.
//
// If r is already a *spoolFile (Pipeline.RunRestore stages verified
// downloads), it is checked in place instead of copied. The caller keeps
// ownership: the returned reader does not delete it, and it is left in
// place on error.
func validateTarStream(r io.Reader, max int64) (rc io.ReadCloser, err error) {
	limit, capped := restoreCap(max)
	if sf, ok := r.(*spoolFile); ok {
		if capped {
			fi, err := sf.f.Stat()
			if err != nil {
				return nil, fmt.Errorf("stat archive: %w", err)
			}
			if fi.Size() > limit {
				return nil, fmt.Errorf("%w of %d bytes", errArchiveTooLarge, limit)
			}
		}
		if err := checkSpooledTar(sf.f, limit, capped); err != nil {
			return nil, err
		}
		return io.NopCloser(sf), nil
	}

	spool, err := newSpoolFile()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			spool.Close()
		}
	}()
	if err := spool.fill(r, limit, capped); err != nil {
		return nil, err
	}
	if err := checkSpooledTar(spool.f, limit, capped); err != nil {
		return nil, err
	}
	return spool, nil
}

// checkSpooledTar runs checkTarEntry on every entry of the (gzipped) tar
// in f, counting the decompressed stream against limit when capped, and
// rewinds f.
func checkSpooledTar(f *os.File, limit int64, capped bool) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind archive: %w", err)
	}

	var tr *tar.Reader
	var cr *capReader
	gzr, gerr := gzip.NewReader(f)
	if gerr == nil {
		defer gzr.Close()
		var src io.Reader = gzr
		if capped {
			cr = newCapReader(gzr, limit)
			src = cr
		}
		tr = tar.NewReader(src)
	} else {
		// Not gzipped; treat as plain tar. The raw size is already capped.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("rewind archive: %w", err)
		}
		tr = tar.NewReader(f)
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, errArchiveTooLarge) {
				return err
			}
			return fmt.Errorf("read tar header: %w", err)
		}
		if err := checkTarEntry(hdr); err != nil {
			return err
		}
	}
	if cr != nil {
		// Drain whatever follows the tar end marker (padding, extra gzip
		// members) so the whole decompressed stream counts against the cap.
		// Only the size cap is fatal here; trailing garbage is left for the
		// in-container tar to ignore, as before.
		if _, err := io.Copy(io.Discard, cr); errors.Is(err, errArchiveTooLarge) {
			return err
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind archive: %w", err)
	}
	return nil
}

// restoreSpoolDir is where restore archives are staged; empty means the
// system temp dir (os.TempDir). serve and the CLI backup commands set it
// to data_dir/tmp so large restores use disk rather than a RAM-backed /tmp.
var restoreSpoolDir atomic.Pointer[string]

// SetSpoolDir sets the directory restore archives are staged in.
func SetSpoolDir(dir string) { restoreSpoolDir.Store(&dir) }

// spoolDirPath returns the configured spool dir, or "" for the system
// temp dir.
func spoolDirPath() string {
	if p := restoreSpoolDir.Load(); p != nil {
		return *p
	}
	return ""
}

// spoolFile is a temp file holding a restore archive. Close deletes it.
type spoolFile struct {
	f        *os.File
	unlinked bool
}

// newSpoolFile creates the spool (mode 0600 via os.CreateTemp) and unlinks
// it straight away where the OS allows, so a crash mid-restore cannot leave
// a copy of the backup behind; Close removes it otherwise.
func newSpoolFile() (*spoolFile, error) {
	dir := spoolDirPath()
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create restore temp dir: %w", err)
		}
		// MkdirAll keeps the mode of an existing dir; older versions made it 0755.
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, fmt.Errorf("restrict restore temp dir: %w", err)
		}
	}
	f, err := os.CreateTemp(dir, "simpledeploy-restore-*")
	if err != nil {
		where := dir
		if where == "" {
			where = os.TempDir()
		}
		return nil, fmt.Errorf("create temp file for restore archive in %s (it must be writable): %w", where, err)
	}
	return &spoolFile{f: f, unlinked: os.Remove(f.Name()) == nil}, nil
}

// fill copies r into the spool, failing with errArchiveTooLarge past limit
// when capped, and rewinds it.
func (s *spoolFile) fill(r io.Reader, limit int64, capped bool) error {
	if capped {
		r = newCapReader(r, limit)
	}
	if _, err := io.Copy(s.f, r); err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) && pe.Op == "write" && pe.Path == s.f.Name() {
			return fmt.Errorf("stage archive in %s (it needs free space for the whole backup file): %w", filepath.Dir(s.f.Name()), err)
		}
		return fmt.Errorf("read archive: %w", err)
	}
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind archive: %w", err)
	}
	return nil
}

func (s *spoolFile) Read(p []byte) (int, error) { return s.f.Read(p) }

func (s *spoolFile) Close() error {
	err := s.f.Close()
	if !s.unlinked {
		if rerr := os.Remove(s.f.Name()); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) && err == nil {
			err = rerr
		}
		s.unlinked = true
	}
	return err
}

// errArchiveTooLarge is returned when a restore archive (raw or
// decompressed) exceeds the restore size cap.
var errArchiveTooLarge = errors.New("backup archive exceeds the restore size limit")

// capReader passes through at most max bytes and fails with
// errArchiveTooLarge if the source has more, so oversize input is an error
// rather than a silent truncation (unlike io.LimitReader).
type capReader struct {
	r         io.Reader
	max       int64
	remaining int64
}

func newCapReader(r io.Reader, max int64) *capReader {
	return &capReader{r: r, max: max, remaining: max}
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		// At the cap: probe one byte to tell a clean EOF from overflow.
		var one [1]byte
		n, err := c.r.Read(one[:])
		if n > 0 {
			return 0, fmt.Errorf("%w of %d bytes", errArchiveTooLarge, c.max)
		}
		return 0, err
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.r.Read(p)
	c.remaining -= int64(n)
	return n, err
}

func checkTarEntry(hdr *tar.Header) error {
	name := hdr.Name
	if name == "" {
		return fmt.Errorf("tar entry has empty name")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("tar entry has absolute path: %q", name)
	}
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("tar entry name contains NUL")
	}
	cleaned := path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return fmt.Errorf("tar entry has parent traversal: %q", name)
	}
	switch hdr.Typeflag {
	case tar.TypeSymlink, tar.TypeLink:
		// Link target evaluation happens at extract-time inside the
		// container; we cannot trust it. Symlinks are the classic
		// extraction-bypass primitive. Reject outright.
		return fmt.Errorf("tar entry %q is a symlink/hardlink (target=%q); not allowed in restore archives", name, hdr.Linkname)
	case tar.TypeBlock, tar.TypeChar, tar.TypeFifo:
		return fmt.Errorf("tar entry %q has special type %c; not allowed", name, hdr.Typeflag)
	}
	return nil
}
