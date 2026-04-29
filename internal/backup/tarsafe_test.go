package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
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
	if _, err := validateTarStream(bytes.NewReader(data)); err != nil {
		t.Fatalf("valid archive rejected: %v", err)
	}
}

func TestValidateTar_AbsolutePath(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "/etc/passwd", Size: 1, Typeflag: tar.TypeReg},
	})
	_, err := validateTarStream(bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path rejection, got %v", err)
	}
}

func TestValidateTar_ParentTraversal(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "../../etc/passwd", Size: 1, Typeflag: tar.TypeReg},
	})
	_, err := validateTarStream(bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestValidateTar_SymlinkRejected(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/link", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink},
	})
	_, err := validateTarStream(bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestValidateTar_HardlinkRejected(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/link", Linkname: "data/orig", Typeflag: tar.TypeLink},
	})
	_, err := validateTarStream(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected hardlink rejection")
	}
}

func TestValidateTar_DeviceRejected(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/dev", Typeflag: tar.TypeBlock, Devmajor: 8, Devminor: 0},
	})
	_, err := validateTarStream(bytes.NewReader(data))
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
	if _, err := validateTarStream(bytes.NewReader(raw.Bytes())); err != nil {
		t.Fatalf("plain tar should be accepted: %v", err)
	}
}

func TestValidateTar_ReplaysBytes(t *testing.T) {
	data := makeTarGz(t, []*tar.Header{
		{Name: "data/foo.db", Size: 8, Typeflag: tar.TypeReg},
	})
	r, err := validateTarStream(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	out, _ := io.ReadAll(r)
	if !bytes.Equal(out, data) {
		t.Fatalf("replayed bytes differ from input")
	}
}
