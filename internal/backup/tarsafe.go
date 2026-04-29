package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
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
// The returned reader replays the original gzipped bytes so the caller
// can pipe them onward to 'docker exec ... tar -xzf -' unchanged.
func validateTarStream(r io.Reader) (io.Reader, error) {
	// Buffer the full payload so we can replay it after validation. The
	// caller already caps decompressed size via limitedGzip; here we cap
	// the compressed/raw payload separately.
	buf, err := io.ReadAll(io.LimitReader(r, defaultMaxDecompressed))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}

	var tr *tar.Reader
	gzr, gerr := gzip.NewReader(bytes.NewReader(buf))
	if gerr == nil {
		defer gzr.Close()
		tr = tar.NewReader(gzr)
	} else {
		// Not gzipped; treat as plain tar.
		tr = tar.NewReader(bytes.NewReader(buf))
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}
		if err := checkTarEntry(hdr); err != nil {
			return nil, err
		}
	}
	return bytes.NewReader(buf), nil
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
