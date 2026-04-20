package deployer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

type ExecRunner struct{}

func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

type RunCall struct {
	Name string
	Args []string
}

type MockRunner struct {
	mu    sync.Mutex
	Calls []RunCall
	Err   error
	// Respond optionally returns a custom response per call. If nil or returns
	// ok=false, the default empty/Err response is used. Useful for tests that
	// need to script different docker subcommand outputs (e.g. stabilize).
	Respond func(call RunCall) (stdout, stderr string, err error, ok bool)
}

func (m *MockRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	call := RunCall{Name: name, Args: args}
	m.mu.Lock()
	m.Calls = append(m.Calls, call)
	respond := m.Respond
	m.mu.Unlock()
	if respond != nil {
		if stdout, stderr, err, ok := respond(call); ok {
			return stdout, stderr, err
		}
	}
	if m.Err != nil {
		return "", fmt.Sprintf("mock error: %v", m.Err), m.Err
	}
	return "", "", nil
}

func (m *MockRunner) HasCall(name string, subArgs ...string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.Calls {
		if c.Name != name {
			continue
		}
		if containsAll(c.Args, subArgs) {
			return true
		}
	}
	return false
}

func containsAll(args, sub []string) bool {
	set := make(map[string]bool, len(args))
	for _, a := range args {
		set[a] = true
	}
	for _, s := range sub {
		if !set[s] {
			return false
		}
	}
	return true
}
