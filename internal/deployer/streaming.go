package deployer

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
)

// RunStreaming runs a command, streaming output lines to the DeployLog
// while also buffering full stdout/stderr for the return value.
func RunStreaming(ctx context.Context, dl *DeployLog, name string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, name, args...)

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", err
	}

	if err := cmd.Start(); err != nil {
		return "", "", err
	}

	var outBuf, errBuf bytes.Buffer
	done := make(chan struct{})

	go func() {
		s := bufio.NewScanner(outPipe)
		for s.Scan() {
			line := s.Text()
			outBuf.WriteString(line + "\n")
			if dl != nil {
				dl.Send(OutputLine{Line: line, Stream: "stdout"})
			}
		}
		if err := s.Err(); err != nil && dl != nil {
			dl.Send(OutputLine{Line: "stdout read error: " + err.Error(), Stream: "stderr"})
		}
		done <- struct{}{}
	}()

	go func() {
		s := bufio.NewScanner(errPipe)
		for s.Scan() {
			line := s.Text()
			errBuf.WriteString(line + "\n")
			if dl != nil {
				dl.Send(OutputLine{Line: line, Stream: "stderr"})
			}
		}
		if err := s.Err(); err != nil && dl != nil {
			dl.Send(OutputLine{Line: "stderr read error: " + err.Error(), Stream: "stderr"})
		}
		done <- struct{}{}
	}()

	<-done
	<-done

	err = cmd.Wait()
	return outBuf.String(), errBuf.String(), err
}
