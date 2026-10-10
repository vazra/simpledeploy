package logbuf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxDockerFrameSize caps one frame of a multiplexed Docker log stream.
const MaxDockerFrameSize = 1 << 20

// ErrDockerFrameTooLarge is returned when a frame header declares a payload
// larger than MaxDockerFrameSize.
var ErrDockerFrameTooLarge = errors.New("docker log frame exceeds size limit")

// ErrDockerStreamCorrupt is returned for a malformed frame header or a
// stream cut off mid-frame.
var ErrDockerStreamCorrupt = errors.New("corrupt docker log stream")

// DockerStreamReader demultiplexes the framing Docker uses for logs of
// containers without a TTY: an 8-byte header (stream id, three zero bytes,
// big-endian uint32 payload size) followed by the payload. The declared
// size is not trusted: frames larger than MaxDockerFrameSize are rejected
// instead of allocated. Streams of TTY containers are not framed and must
// not be passed through this reader.
type DockerStreamReader struct {
	r   io.Reader
	max int
	hdr [8]byte
	buf []byte
}

// NewDockerStreamReader wraps a multiplexed (non-TTY) Docker log stream.
func NewDockerStreamReader(r io.Reader) *DockerStreamReader {
	return &DockerStreamReader{r: r, max: MaxDockerFrameSize}
}

// Next returns the next frame's stream name ("stdout" or "stderr") and its
// payload. The payload slice is reused by the following call. It returns
// io.EOF at a clean end of stream. A daemon error frame (stream id 3) is
// returned as an error carrying the daemon's message.
func (d *DockerStreamReader) Next() (string, []byte, error) {
	if _, err := io.ReadFull(d.r, d.hdr[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return "", nil, fmt.Errorf("%w: truncated frame header", ErrDockerStreamCorrupt)
		}
		return "", nil, err
	}
	var stream string
	switch d.hdr[0] {
	case 0, 1:
		stream = "stdout"
	case 2:
		stream = "stderr"
	case 3:
		stream = "system"
	default:
		return "", nil, fmt.Errorf("%w: invalid stream id %d", ErrDockerStreamCorrupt, d.hdr[0])
	}
	size := binary.BigEndian.Uint32(d.hdr[4:8])
	if uint64(size) > uint64(d.max) {
		return "", nil, fmt.Errorf("%w: %d bytes (max %d)", ErrDockerFrameTooLarge, size, d.max)
	}
	if cap(d.buf) < int(size) {
		d.buf = make([]byte, size)
	}
	d.buf = d.buf[:size]
	if _, err := io.ReadFull(d.r, d.buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return "", nil, fmt.Errorf("%w: truncated frame payload", ErrDockerStreamCorrupt)
		}
		return "", nil, err
	}
	if stream == "system" {
		return "", nil, fmt.Errorf("docker daemon error: %s", d.buf)
	}
	return stream, d.buf, nil
}
