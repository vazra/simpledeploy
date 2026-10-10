package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/vazra/simpledeploy/internal/logbuf"
)

// printContainerLogs copies a Docker log stream to w. TTY streams are raw
// and copied as-is. Non-TTY streams are demultiplexed with frame sizes
// capped at logbuf.MaxDockerFrameSize, and each frame is printed as
// "[stdout] ..." or "[stderr] ...". A clean end of stream returns nil.
func printContainerLogs(w io.Writer, r io.Reader, tty bool) error {
	if tty {
		if _, err := io.Copy(w, r); err != nil {
			return fmt.Errorf("read logs: %w", err)
		}
		return nil
	}
	d := logbuf.NewDockerStreamReader(r)
	for {
		stream, payload, err := d.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read logs: %w", err)
		}
		if _, err := fmt.Fprintf(w, "[%s] %s", stream, payload); err != nil {
			return err
		}
	}
}
