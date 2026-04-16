package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

func TestChecksumTeeReaderPassesDataThrough(t *testing.T) {
	data := "hello world"
	cw := NewChecksumWriter()
	r := cw.TeeReader(strings.NewReader(data))

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != data {
		t.Fatalf("TeeReader changed data: got %q, want %q", string(out), data)
	}
}

func TestChecksumSumReturns64CharHex(t *testing.T) {
	cw := NewChecksumWriter()
	r := cw.TeeReader(strings.NewReader("test data"))
	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatal(err)
	}

	sum := cw.Sum()
	if len(sum) != 64 {
		t.Fatalf("expected 64-char hex, got %d chars: %s", len(sum), sum)
	}

	// verify it matches direct sha256
	h := sha256.Sum256([]byte("test data"))
	expected := hex.EncodeToString(h[:])
	if sum != expected {
		t.Fatalf("checksum mismatch: got %s, want %s", sum, expected)
	}
}

func TestVerifyChecksumPass(t *testing.T) {
	data := []byte("some backup data")
	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	err := VerifyChecksum(bytes.NewReader(data), expected)
	if err != nil {
		t.Fatalf("expected pass, got error: %v", err)
	}
}

func TestVerifyChecksumFail(t *testing.T) {
	data := []byte("some backup data")
	err := VerifyChecksum(bytes.NewReader(data), "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error for mismatched checksum")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected 'checksum mismatch' in error, got: %v", err)
	}
}
