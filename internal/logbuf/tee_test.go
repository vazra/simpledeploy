package logbuf

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type lineRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (l *lineRecorder) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.lines = append(l.lines, string(p))
	l.mu.Unlock()
	return len(p), nil
}

func (l *lineRecorder) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func TestLineSplitterTruncatesLongLineAndContinues(t *testing.T) {
	var got []string
	s := NewLineSplitter(1024, func(line []byte) { got = append(got, string(line)) })

	huge := strings.Repeat("x", 2<<20) // 2 MiB, no newline inside
	input := "first\n" + huge + "\nsecond\nthird"
	// Feed in odd-sized chunks so lines straddle Write calls.
	for b := []byte(input); len(b) > 0; {
		n := 7777
		if n > len(b) {
			n = len(b)
		}
		s.Write(b[:n])
		b = b[n:]
	}
	s.Flush()

	if len(got) != 4 {
		t.Fatalf("got %d lines, want 4", len(got))
	}
	if got[0] != "first" {
		t.Errorf("line 0 = %q", got[0])
	}
	want := strings.Repeat("x", 1024) + TruncatedMarker
	if got[1] != want {
		t.Errorf("line 1 len=%d, want truncated to %d + marker", len(got[1]), 1024)
	}
	if got[2] != "second" || got[3] != "third" {
		t.Errorf("lines after long line = %q, %q", got[2], got[3])
	}
}

func TestLineSplitterExactCapNotTruncated(t *testing.T) {
	var got []string
	s := NewLineSplitter(4, func(line []byte) { got = append(got, string(line)) })
	s.Write([]byte("abcd\nabcde\n"))
	if len(got) != 2 || got[0] != "abcd" || got[1] != "abcd"+TruncatedMarker {
		t.Fatalf("got %q", got)
	}
}

func TestTeeForwardsRawAndCapturesLines(t *testing.T) {
	huge := strings.Repeat("y", 1<<20+5) // > 1 MiB
	input := huge + "\nafter-1\nafter-2\n"

	var orig bytes.Buffer
	sink := &lineRecorder{}
	Tee(strings.NewReader(input), &orig, sink, DefaultTeeLineCap)

	if orig.String() != input {
		t.Fatalf("orig got %d bytes, want %d unchanged", orig.Len(), len(input))
	}
	lines := sink.get()
	if len(lines) != 3 {
		t.Fatalf("captured %d lines, want 3", len(lines))
	}
	if !strings.HasSuffix(lines[0], TruncatedMarker) || len(lines[0]) != DefaultTeeLineCap+len(TruncatedMarker) {
		t.Errorf("long line not truncated: len=%d", len(lines[0]))
	}
	if lines[1] != "after-1" || lines[2] != "after-2" {
		t.Errorf("following lines = %q", lines[1:])
	}
}

// TestTeeKeepsPipeDrained is the regression test for the old bufio.Scanner
// tee: after an over-long line it stopped reading, the pipe filled and the
// writer blocked forever.
func TestTeeKeepsPipeDrained(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var orig bytes.Buffer
	sink := &lineRecorder{}
	done := make(chan struct{})
	go func() {
		Tee(r, &orig, sink, DefaultTeeLineCap)
		close(done)
	}()

	wrote := make(chan error, 1)
	go func() {
		// Well beyond any pipe buffer: blocks forever if the tee stalls.
		if _, err := w.Write([]byte(strings.Repeat("z", 3<<20) + "\n")); err != nil {
			wrote <- err
			return
		}
		for i := 0; i < 1000; i++ {
			if _, err := w.Write([]byte("more output line\n")); err != nil {
				wrote <- err
				return
			}
		}
		wrote <- w.Close()
	}()

	select {
	case err := <-wrote:
		if err != nil {
			t.Fatalf("write: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writer blocked: tee stopped draining the pipe")
	}
	<-done
	if got := len(sink.get()); got != 1001 {
		t.Fatalf("captured %d lines, want 1001", got)
	}
}

type flakyReader struct {
	steps []func(p []byte) (int, error)
}

func (f *flakyReader) Read(p []byte) (int, error) {
	if len(f.steps) == 0 {
		return 0, io.EOF
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step(p)
}

func TestTeeKeepsForwardingAfterReadError(t *testing.T) {
	r := &flakyReader{steps: []func([]byte) (int, error){
		func(p []byte) (int, error) { return copy(p, "line-1\npart"), nil },
		func(p []byte) (int, error) { return 0, errors.New("transient") },
		func(p []byte) (int, error) { return copy(p, "ial\nraw-tail\n"), nil },
	}}
	var orig bytes.Buffer
	sink := &lineRecorder{}
	Tee(r, &orig, sink, DefaultTeeLineCap)

	if orig.String() != "line-1\npartial\nraw-tail\n" {
		t.Fatalf("orig = %q", orig.String())
	}
	lines := sink.get()
	if len(lines) < 1 || lines[0] != "line-1" {
		t.Fatalf("captured = %q", lines)
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errors.New("broken") }

func TestTeeIgnoresOrigWriteErrors(t *testing.T) {
	sink := &lineRecorder{}
	Tee(strings.NewReader("a\nb\nc\n"), failingWriter{}, sink, DefaultTeeLineCap)
	if got := sink.get(); len(got) != 3 {
		t.Fatalf("captured %q, want 3 lines", got)
	}
}

// TestTeeNeverGivesUpOnReadErrors: a reader that fails many times in a row
// (more than the old give-up limit) is still drained afterwards; nothing it
// produces later is lost.
func TestTeeNeverGivesUpOnReadErrors(t *testing.T) {
	prevMin, prevMax := teeRetryMinDelay, teeRetryMaxDelay
	teeRetryMinDelay, teeRetryMaxDelay = time.Microsecond, 4*time.Microsecond
	t.Cleanup(func() { teeRetryMinDelay, teeRetryMaxDelay = prevMin, prevMax })

	steps := []func([]byte) (int, error){
		func(p []byte) (int, error) { return copy(p, "before\n"), nil },
	}
	for i := 0; i < 500; i++ {
		steps = append(steps, func(p []byte) (int, error) { return 0, errors.New("EIO") })
	}
	steps = append(steps, func(p []byte) (int, error) { return copy(p, "after errors\n"), nil })

	var orig bytes.Buffer
	done := make(chan struct{})
	go func() {
		Tee(&flakyReader{steps: steps}, &orig, &lineRecorder{}, DefaultTeeLineCap)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Tee did not finish")
	}
	if orig.String() != "before\nafter errors\n" {
		t.Fatalf("orig = %q, want output after the read errors forwarded", orig.String())
	}
}

// TestTeeBacksOffOnRepeatedReadErrors: consecutive failures wait longer each
// time (capped) instead of spinning.
func TestTeeBacksOffOnRepeatedReadErrors(t *testing.T) {
	prevMin, prevMax := teeRetryMinDelay, teeRetryMaxDelay
	teeRetryMinDelay, teeRetryMaxDelay = time.Millisecond, 8*time.Millisecond
	t.Cleanup(func() { teeRetryMinDelay, teeRetryMaxDelay = prevMin, prevMax })

	var steps []func([]byte) (int, error)
	for i := 0; i < 6; i++ {
		steps = append(steps, func(p []byte) (int, error) { return 0, errors.New("EIO") })
	}
	start := time.Now()
	Tee(&flakyReader{steps: steps}, io.Discard, &lineRecorder{}, DefaultTeeLineCap)
	// 1+2+4+8+8+8 ms with backoff; 6 ms if the delay never grew.
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("6 failed reads took %v, want backoff (>= 25ms)", elapsed)
	}
}

// TestTeeStopsWhenReaderClosed: closing the read end (or the write end, via
// EOF) ends Tee so callers can wait for it to flush.
func TestTeeStopsWhenReaderClosed(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var orig bytes.Buffer
	done := make(chan struct{})
	go func() {
		Tee(r, &orig, &lineRecorder{}, DefaultTeeLineCap)
		close(done)
	}()
	if _, err := w.Write([]byte("last words\n")); err != nil {
		t.Fatal(err)
	}
	w.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Tee did not return after the write end closed")
	}
	r.Close()
	if orig.String() != "last words\n" {
		t.Fatalf("orig = %q", orig.String())
	}
}
