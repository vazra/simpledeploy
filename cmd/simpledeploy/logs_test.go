package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/logbuf"
)

func cliFrame(stream byte, payload string) []byte {
	hdr := make([]byte, 8)
	hdr[0] = stream
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
	return append(hdr, payload...)
}

func TestPrintContainerLogsDemux(t *testing.T) {
	var in bytes.Buffer
	in.Write(cliFrame(1, "hello\n"))
	in.Write(cliFrame(2, "oops\n"))
	var out bytes.Buffer
	if err := printContainerLogs(&out, &in, false); err != nil {
		t.Fatal(err)
	}
	if out.String() != "[stdout] hello\n[stderr] oops\n" {
		t.Fatalf("out = %q", out.String())
	}
}

func TestPrintContainerLogsRejectsOversizedFrame(t *testing.T) {
	hdr := make([]byte, 8)
	hdr[0] = 1
	binary.BigEndian.PutUint32(hdr[4:], logbuf.MaxDockerFrameSize+1)
	var out bytes.Buffer
	err := printContainerLogs(&out, bytes.NewReader(hdr), false)
	if !errors.Is(err, logbuf.ErrDockerFrameTooLarge) {
		t.Fatalf("err = %v, want ErrDockerFrameTooLarge", err)
	}
}

func TestPrintContainerLogsTTYRaw(t *testing.T) {
	raw := "\x01\x00\x00\x00 not a header, tty output\n"
	var out bytes.Buffer
	if err := printContainerLogs(&out, strings.NewReader(raw), true); err != nil {
		t.Fatal(err)
	}
	if out.String() != raw {
		t.Fatalf("tty output altered: %q", out.String())
	}
}
