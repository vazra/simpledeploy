package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTarGz(t *testing.T, entries []*tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range entries {
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if h.Size > 0 {
			tw.Write(make([]byte, h.Size))
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestValidateTar_Valid(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/foo.db", Size: 4, Typeflag: tar.TypeReg},
		{Name: "data/bar.db", Size: 4, Typeflag: tar.TypeReg},
	})
	rc, err := validateTarStream(bytes.NewReader(data), 0)
	if err != nil {
		t.Fatalf("valid archive rejected: %v", err)
	}
	rc.Close()
}

func TestValidateTar_AbsolutePath(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "/etc/passwd", Size: 1, Typeflag: tar.TypeReg},
	})
	_, err := validateTarStream(bytes.NewReader(data), 0)
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path rejection, got %v", err)
	}
}

func TestValidateTar_ParentTraversal(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "../../etc/passwd", Size: 1, Typeflag: tar.TypeReg},
	})
	_, err := validateTarStream(bytes.NewReader(data), 0)
	if err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestValidateTar_SymlinkRejected(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/link", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink},
	})
	_, err := validateTarStream(bytes.NewReader(data), 0)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestValidateTar_HardlinkRejected(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/link", Linkname: "data/orig", Typeflag: tar.TypeLink},
	})
	_, err := validateTarStream(bytes.NewReader(data), 0)
	if err == nil {
		t.Fatal("expected hardlink rejection")
	}
}

func TestValidateTar_DeviceRejected(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/dev", Typeflag: tar.TypeBlock, Devmajor: 8, Devminor: 0},
	})
	_, err := validateTarStream(bytes.NewReader(data), 0)
	if err == nil {
		t.Fatal("expected device rejection")
	}
}

func TestValidateTar_PlainTar(t *testing.T) {
	// not gzipped
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	tw.WriteHeader(&tar.Header{Name: "data/x.db", Size: 0, Typeflag: tar.TypeReg})
	tw.Close()
	rc, err := validateTarStream(bytes.NewReader(raw.Bytes()), 0)
	if err != nil {
		t.Fatalf("plain tar should be accepted: %v", err)
	}
	rc.Close()
}

func TestValidateTar_ReplaysBytes(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/foo.db", Size: 8, Typeflag: tar.TypeReg},
	})
	r, err := validateTarStream(bytes.NewReader(data), 0)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	defer r.Close()
	out, _ := io.ReadAll(r)
	if !bytes.Equal(out, data) {
		t.Fatalf("replayed bytes differ from input")
	}
}

func TestValidateTar_DecompressedCap(t *testing.T) {
	// 64 KiB of zeros compresses to a few hundred bytes, so this exercises
	// the decompressed cap, not the raw one.
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/big.bin", Size: 64 << 10, Typeflag: tar.TypeReg},
	})
	if len(data) >= 16<<10 {
		t.Fatalf("test archive unexpectedly large: %d bytes", len(data))
	}
	_, err := validateTarStream(bytes.NewReader(data), 16<<10)
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("expected errArchiveTooLarge, got %v", err)
	}
}

func TestValidateTar_RawCap(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	tw.WriteHeader(&tar.Header{Name: "data/x.db", Size: 8 << 10, Typeflag: tar.TypeReg})
	tw.Write(make([]byte, 8<<10))
	tw.Close()
	_, err := validateTarStream(bytes.NewReader(raw.Bytes()), 4<<10)
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("expected errArchiveTooLarge, got %v", err)
	}
}

func TestValidateTar_TrailingGzipMemberCounts(t *testing.T) {
	// A valid small tar followed by a second gzip member that expands past
	// the cap: the extra member must count against the limit.
	data := makeTarGz(t, []*tar.Header{{Name: "data/a", Size: 1, Typeflag: tar.TypeReg}})
	var extra bytes.Buffer
	gz := gzip.NewWriter(&extra)
	gz.Write(make([]byte, 64<<10))
	gz.Close()
	data = append(data, extra.Bytes()...)
	_, err := validateTarStream(bytes.NewReader(data), 32<<10)
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("expected errArchiveTooLarge, got %v", err)
	}
}

func TestValidateTar_UnderCapAccepted(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/small.bin", Size: 1 << 10, Typeflag: tar.TypeReg},
	})
	rc, err := validateTarStream(bytes.NewReader(data), 64<<10)
	if err != nil {
		t.Fatalf("archive under cap rejected: %v", err)
	}
	rc.Close()
}

// spoolDir points os.TempDir() (and so the restore spool) at a fresh dir.
func spoolDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	return dir
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		t.Errorf("leftover temp file %s", e.Name())
	}
}

func TestValidateTar_SpoolRemovedAfterSuccess(t *testing.T) {
	dir := spoolDir(t)
	data := makeTarGz(t, []*tar.Header{{Name: "data/foo.db", Size: 4, Typeflag: tar.TypeReg}})
	rc, err := validateTarStream(bytes.NewReader(data), 0)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	sf, ok := rc.(*spoolFile)
	if !ok {
		t.Fatalf("validateTarStream returned %T, want *spoolFile", rc)
	}
	if filepath.Dir(sf.f.Name()) != dir {
		t.Fatalf("spool %s not created in TMPDIR %s", sf.f.Name(), dir)
	}
	fi, err := sf.f.Stat()
	if err != nil {
		t.Fatalf("stat spool: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("spool mode = %o, want 600", perm)
	}
	if out, _ := io.ReadAll(rc); !bytes.Equal(out, data) {
		t.Fatal("replayed bytes differ from input")
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	assertDirEmpty(t, dir)
}

func TestValidateTar_SpoolRemovedAfterFailure(t *testing.T) {
	dir := spoolDir(t)
	bad := makeTarGz(t, []*tar.Header{{Name: "../../etc/passwd", Size: 1, Typeflag: tar.TypeReg}})
	if _, err := validateTarStream(bytes.NewReader(bad), 0); err == nil {
		t.Fatal("expected traversal rejection")
	}
	assertDirEmpty(t, dir)

	big := makeTarGz(t, []*tar.Header{{Name: "data/big.bin", Size: 64 << 10, Typeflag: tar.TypeReg}})
	if _, err := validateTarStream(bytes.NewReader(big), 16<<10); !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("expected errArchiveTooLarge, got %v", err)
	}
	assertDirEmpty(t, dir)
}

func TestValidateTar_SpoolDirUnwritable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", missing)
	data := makeTarGz(t, []*tar.Header{{Name: "data/foo.db", Size: 4, Typeflag: tar.TypeReg}})
	_, err := validateTarStream(bytes.NewReader(data), 0)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("expected temp-file error naming %s, got %v", missing, err)
	}
}

// setSpoolDir sets the restore spool dir for the test and restores it after.
func setSpoolDir(t *testing.T, dir string) {
	t.Helper()
	old := restoreSpoolDir.Load()
	SetSpoolDir(dir)
	t.Cleanup(func() { restoreSpoolDir.Store(old) })
}

func TestNewSpoolFile_UsesSpoolDirOwnerOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tmp")
	// An existing dir from an older version may be 0755.
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	setSpoolDir(t, dir)
	sf, err := newSpoolFile()
	if err != nil {
		t.Fatalf("newSpoolFile: %v", err)
	}
	defer sf.Close()
	if filepath.Dir(sf.f.Name()) != dir {
		t.Fatalf("spool %s not created in %s", sf.f.Name(), dir)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("spool dir perm = %o, want 700", perm)
	}
}

func TestNewSpoolFile_CreatesSpoolDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data", "tmp")
	setSpoolDir(t, dir)
	sf, err := newSpoolFile()
	if err != nil {
		t.Fatalf("newSpoolFile: %v", err)
	}
	sf.Close()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("spool dir perm = %o, want 700", perm)
	}
	assertDirEmpty(t, dir)
}

func TestNewSpoolFile_ErrorNamesSpoolDir(t *testing.T) {
	parent := t.TempDir()
	// A file where the dir should be makes MkdirAll fail.
	blocker := filepath.Join(parent, "tmp")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	setSpoolDir(t, filepath.Join(blocker, "sub"))
	if _, err := newSpoolFile(); err == nil || !strings.Contains(err.Error(), "restore temp dir") {
		t.Fatalf("want restore temp dir error, got %v", err)
	}
}

func TestValidateTar_NoCap(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{{Name: "data/big.bin", Size: 64 << 10, Typeflag: tar.TypeReg}})
	rc, err := validateTarStream(bytes.NewReader(data), NoDecompressedLimit)
	if err != nil {
		t.Fatalf("uncapped validate: %v", err)
	}
	rc.Close()
	// Unsafe entries are still refused without a cap.
	bad := makeTarGz(t, []*tar.Header{{Name: "../x", Size: 1, Typeflag: tar.TypeReg}})
	if _, err := validateTarStream(bytes.NewReader(bad), NoDecompressedLimit); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

// stagedSpool returns data in a spool file, as Pipeline.RunRestore stages it.
func stagedSpool(t *testing.T, data []byte) *spoolFile {
	t.Helper()
	sf, err := newSpoolFile()
	if err != nil {
		t.Fatalf("newSpoolFile: %v", err)
	}
	t.Cleanup(func() { sf.Close() })
	if err := sf.fill(bytes.NewReader(data), 0, false); err != nil {
		t.Fatalf("fill: %v", err)
	}
	return sf
}

func TestValidateTar_SpooledInputCheckedInPlace(t *testing.T) {
	spoolDir(t)
	data := makeTarGz(t, []*tar.Header{{Name: "data/foo.db", Size: 4, Typeflag: tar.TypeReg}})
	sf := stagedSpool(t, data)
	rc, err := validateTarStream(sf, 0)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, ok := rc.(*spoolFile); ok {
		t.Fatal("staged input was copied into a new spool")
	}
	if out, _ := io.ReadAll(rc); !bytes.Equal(out, data) {
		t.Fatal("replayed bytes differ from input")
	}
	// Closing the returned reader leaves the caller's spool open.
	rc.Close()
	if _, err := sf.f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("caller spool closed by validateTarStream: %v", err)
	}
}

func TestValidateTar_SpooledInputRejected(t *testing.T) {
	spoolDir(t)
	bad := stagedSpool(t, makeTarGz(t, []*tar.Header{{Name: "/etc/passwd", Size: 1, Typeflag: tar.TypeReg}}))
	if _, err := validateTarStream(bad, 0); err == nil {
		t.Fatal("expected absolute path rejection")
	}
	big := stagedSpool(t, makeTarGz(t, []*tar.Header{{Name: "data/big.bin", Size: 64 << 10, Typeflag: tar.TypeReg}}))
	if _, err := validateTarStream(big, 16<<10); !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("decompressed cap: want errArchiveTooLarge, got %v", err)
	}
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	tw.WriteHeader(&tar.Header{Name: "data/x.db", Size: 8 << 10, Typeflag: tar.TypeReg})
	tw.Write(make([]byte, 8<<10))
	tw.Close()
	if _, err := validateTarStream(stagedSpool(t, raw.Bytes()), 4<<10); !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("raw cap: want errArchiveTooLarge, got %v", err)
	}
}

func TestCapReader_ExactLimit(t *testing.T) {
	out, err := io.ReadAll(newCapReader(bytes.NewReader(make([]byte, 10)), 10))
	if err != nil || len(out) != 10 {
		t.Fatalf("exact-limit read: n=%d err=%v", len(out), err)
	}
	_, err = io.ReadAll(newCapReader(bytes.NewReader(make([]byte, 11)), 10))
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("over-limit read: expected errArchiveTooLarge, got %v", err)
	}
}
