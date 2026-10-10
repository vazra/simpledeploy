package logbuf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func dockerFrame(stream byte, payload string) []byte {
	hdr := make([]byte, 8)
	hdr[0] = stream
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
	return append(hdr, payload...)
}

func TestDockerStreamReaderFrames(t *testing.T) {
	var b bytes.Buffer
	b.Write(dockerFrame(1, "out line\n"))
	b.Write(dockerFrame(2, "err line\n"))
	d := NewDockerStreamReader(&b)

	stream, p, err := d.Next()
	if err != nil || stream != "stdout" || string(p) != "out line\n" {
		t.Fatalf("frame 1 = %q %q %v", stream, p, err)
	}
	stream, p, err = d.Next()
	if err != nil || stream != "stderr" || string(p) != "err line\n" {
		t.Fatalf("frame 2 = %q %q %v", stream, p, err)
	}
	if _, _, err := d.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("end err = %v, want io.EOF", err)
	}
}

func TestDockerStreamReaderRejectsOversizedFrame(t *testing.T) {
	hdr := make([]byte, 8)
	hdr[0] = 1
	binary.BigEndian.PutUint32(hdr[4:], 0xFFFFFFF0) // ~4 GiB declared
	d := NewDockerStreamReader(bytes.NewReader(hdr))
	_, _, err := d.Next()
	if !errors.Is(err, ErrDockerFrameTooLarge) {
		t.Fatalf("err = %v, want ErrDockerFrameTooLarge", err)
	}
	if cap(d.buf) != 0 {
		t.Fatalf("allocated %d bytes for rejected frame", cap(d.buf))
	}
}

func TestDockerStreamReaderMaxSizeFrameAccepted(t *testing.T) {
	payload := strings.Repeat("a", MaxDockerFrameSize)
	d := NewDockerStreamReader(bytes.NewReader(dockerFrame(1, payload)))
	_, p, err := d.Next()
	if err != nil || len(p) != MaxDockerFrameSize {
		t.Fatalf("len=%d err=%v", len(p), err)
	}
}

func TestDockerStreamReaderCorrupt(t *testing.T) {
	cases := map[string][]byte{
		"bad stream id":     dockerFrame(9, "x"),
		"truncated header":  {1, 0, 0},
		"truncated payload": dockerFrame(1, "hello")[:10],
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := NewDockerStreamReader(bytes.NewReader(data)).Next()
			if !errors.Is(err, ErrDockerStreamCorrupt) {
				t.Fatalf("err = %v, want ErrDockerStreamCorrupt", err)
			}
		})
	}
}

func TestDockerStreamReaderSystemError(t *testing.T) {
	d := NewDockerStreamReader(bytes.NewReader(dockerFrame(3, "boom")))
	if _, _, err := d.Next(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}
