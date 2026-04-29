package logbuf

import (
	"io"
	"log"
	"os"
	"strings"
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

func TestSanitize_StripsANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m and \x1b[1;33mbright\x1b[0m"
	got := sanitizeLogMessage(in)
	if got != "red and bright" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitize_StripsOSC(t *testing.T) {
	in := "\x1b]52;c;ZGFuZ2Vyb3Vz\x07tail"
	got := sanitizeLogMessage(in)
	if got != "tail" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitize_TruncatesLong(t *testing.T) {
	in := strings.Repeat("a", maxLineBytes+1000)
	got := sanitizeLogMessage(in)
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Fatalf("missing truncation marker")
	}
}

func TestSanitize_DropsControlChars(t *testing.T) {
	in := "before\x00\x07after\ttab"
	got := sanitizeLogMessage(in)
	if got != "beforeafter\ttab" {
		t.Fatalf("got %q", got)
	}
}
