package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

type ChecksumWriter struct {
	hash io.Writer
}

func NewChecksumWriter() *ChecksumWriter {
	return &ChecksumWriter{hash: sha256.New()}
}

func (c *ChecksumWriter) TeeReader(r io.Reader) io.Reader {
	return io.TeeReader(r, c.hash)
}

func (c *ChecksumWriter) Sum() string {
	h := c.hash.(interface{ Sum([]byte) []byte })
	return hex.EncodeToString(h.Sum(nil))
}

func VerifyChecksum(r io.Reader, expectedHex string) error {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return fmt.Errorf("reading data for checksum: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedHex {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHex, actual)
	}
	return nil
}
