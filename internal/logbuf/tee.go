package logbuf

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"time"
)

// TruncatedMarker is appended to a line that was cut at the line cap.
const TruncatedMarker = "...[truncated]"

// DefaultTeeLineCap bounds a single captured line in Tee. Longer lines are
// truncated for the capture sink; the original output is never altered.
const DefaultTeeLineCap = 256 * 1024

const teeReadSize = 32 * 1024

// Backoff between failed reads in Tee: doubles per consecutive failure up to
// the max and resets after a successful read. Variables so tests can shrink
// them.
var (
	teeRetryMinDelay = 10 * time.Millisecond
	teeRetryMaxDelay = time.Second
)

// LineSplitter turns a byte stream into newline-delimited lines. A line
// longer than the cap is truncated (TruncatedMarker appended) and the rest
// of it is discarded up to the next newline, so memory stays bounded by the
// cap no matter how long a line is and the splitter never stops consuming.
type LineSplitter struct {
	max       int
	emit      func(line []byte)
	buf       []byte
	truncated bool
}

// NewLineSplitter returns a splitter that calls emit once per line, without
// the trailing newline. The slice passed to emit is only valid during the
// call. max <= 0 means DefaultTeeLineCap.
func NewLineSplitter(max int, emit func(line []byte)) *LineSplitter {
	if max <= 0 {
		max = DefaultTeeLineCap
	}
	return &LineSplitter{max: max, emit: emit}
}

// Write implements io.Writer. It never returns an error.
func (s *LineSplitter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			s.add(p)
			break
		}
		s.add(p[:i])
		s.flushLine()
		p = p[i+1:]
	}
	return n, nil
}

// Flush emits a buffered partial line, if any.
func (s *LineSplitter) Flush() {
	if len(s.buf) > 0 || s.truncated {
		s.flushLine()
	}
}

func (s *LineSplitter) add(chunk []byte) {
	room := s.max - len(s.buf)
	if len(chunk) > room {
		if room > 0 {
			s.buf = append(s.buf, chunk[:room]...)
		}
		s.truncated = true
		return
	}
	s.buf = append(s.buf, chunk...)
}

func (s *LineSplitter) flushLine() {
	line := s.buf
	if s.truncated {
		line = append(line, TruncatedMarker...)
	}
	s.emit(line)
	s.buf = s.buf[:0]
	s.truncated = false
}

// Tee copies everything read from r to orig unchanged and writes one line
// at a time to sink. It drains r until EOF (or until r is closed) no matter
// what: write errors on orig or sink are ignored and over-long lines are
// truncated for sink, because a stalled reader fills the pipe and then
// blocks every write to the process's stdout/stderr. On a read error other
// than EOF it stops line capture, keeps forwarding raw bytes to orig and
// retries with a capped backoff; it never gives up on its own.
func Tee(r io.Reader, orig, sink io.Writer, lineCap int) {
	split := NewLineSplitter(lineCap, func(line []byte) { _, _ = sink.Write(line) })
	defer split.Flush()

	buf := make([]byte, teeReadSize)
	raw := false
	delay := teeRetryMinDelay
	for {
		n, err := r.Read(buf)
		if n > 0 {
			delay = teeRetryMinDelay
			_, _ = orig.Write(buf[:n])
			if !raw {
				_, _ = split.Write(buf[:n])
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, fs.ErrClosed) {
			return
		}
		if !raw {
			split.Flush()
			raw = true
		}
		time.Sleep(delay)
		delay = min(delay*2, teeRetryMaxDelay)
	}
}
