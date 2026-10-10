package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/logbuf"
)

// TestCaptureProcessOutputRestores: output written while capturing lands in
// the buffer, and the restore func puts back os.Stdout, os.Stderr and the
// log output after flushing what was written before it.
func TestCaptureProcessOutputRestores(t *testing.T) {
	origStdout, origStderr, origLog := os.Stdout, os.Stderr, log.Writer()
	buf := logbuf.New(100)

	restore := captureProcessOutput(buf)
	if os.Stderr == origStderr || os.Stdout == origStdout {
		restore()
		t.Fatal("streams not redirected")
	}
	fmt.Fprintln(os.Stdout, "capture-test stdout line")
	log.Print("capture-test fatal error")
	restore()

	if os.Stdout != origStdout || os.Stderr != origStderr || log.Writer() != origLog {
		t.Fatal("restore did not put back stdout, stderr and log output")
	}
	var got []string
	for _, e := range buf.Recent(100) {
		got = append(got, e.Message)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"capture-test stdout line", "capture-test fatal error"} {
		if !strings.Contains(joined, want) {
			t.Errorf("buffer missing %q after restore; got %q", want, got)
		}
	}
}
