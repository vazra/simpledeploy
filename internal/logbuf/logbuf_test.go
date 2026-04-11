package logbuf

import (
	"io"
	"log"
	"os"
	"testing"
)

func TestBufferCapturesLogOutput(t *testing.T) {
	buf := New(100)
	log.SetOutput(io.MultiWriter(os.Stderr, buf))
	defer log.SetOutput(os.Stderr)

	log.Printf("hello from test")
	log.Printf("second message")

	entries := buf.Recent(10)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	t.Logf("entry 0: %q", entries[0].Message)
	t.Logf("entry 1: %q", entries[1].Message)
}
