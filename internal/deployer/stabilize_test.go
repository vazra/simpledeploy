package deployer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// makeRunner returns a MockRunner whose Respond hook scripts `docker ps` and
// `docker inspect` output to simulate a stabilization scenario.
//
// containers: map of containerID -> ordered list of inspect responses. Each
// response in the slice corresponds to one stabilize iteration; the last one
// repeats indefinitely. Each response carries the container's State.Status,
// .State.Health.Status, .RestartCount.
func makeRunner(t *testing.T, project string, containers []mockContainer) *MockRunner {
	t.Helper()
	var inspectCounts sync.Map // id -> *atomic.Int64

	return &MockRunner{
		Respond: func(call RunCall) (string, string, error, bool) {
			args := strings.Join(call.Args, " ")
			// docker ps --filter label=com.docker.compose.project=<p>
			if call.Name == "docker" && len(call.Args) > 0 && call.Args[0] == "ps" {
				if !strings.Contains(args, project) {
					return "", "", nil, true
				}
				var lines []string
				for _, c := range containers {
					lines = append(lines, c.id+"\t"+c.service)
				}
				return strings.Join(lines, "\n"), "", nil, true
			}
			// docker inspect --format '{{json .State}}' <id> OR
			// docker inspect --format '{{.RestartCount}}' <id>
			if call.Name == "docker" && len(call.Args) > 0 && call.Args[0] == "inspect" {
				if len(call.Args) < 4 {
					return "", "", nil, true
				}
				format := call.Args[2]
				id := call.Args[3]
				var c *mockContainer
				for i := range containers {
					if containers[i].id == id {
						c = &containers[i]
						break
					}
				}
				if c == nil {
					return "", "", fmt.Errorf("unknown id"), true
				}
				cv, _ := inspectCounts.LoadOrStore(id, new(atomic.Int64))
				counter := cv.(*atomic.Int64)
				idx := int(counter.Load())
				if idx >= len(c.states) {
					idx = len(c.states) - 1
				}
				st := c.states[idx]
				// Advance only on .State queries (RestartCount is always queried
				// after a State query, in the same loop iteration).
				if format == "{{json .State}}" {
					counter.Add(1)
				}

				if strings.Contains(format, "RestartCount") {
					return fmt.Sprintf("%d\n", st.restartCount), "", nil, true
				}
				if strings.Contains(format, ".State") {
					health := ""
					if st.health != "" {
						health = fmt.Sprintf(`,"Health":{"Status":%q}`, st.health)
					}
					body := fmt.Sprintf(`{"Status":%q,"ExitCode":%d%s}`, st.state, st.exitCode, health)
					return body, "", nil, true
				}
			}
			return "", "", nil, true
		},
	}
}

type mockContainer struct {
	id      string
	service string
	states  []mockState
}

type mockState struct {
	state        string // running, restarting, exited, ...
	health       string // healthy, unhealthy, starting, ""
	exitCode     int
	restartCount int
}

func TestStabilize_Success(t *testing.T) {
	StabilizeTimeout = 2 * time.Second
	defer func() { StabilizeTimeout = 30 * time.Second }()

	runner := makeRunner(t, "simpledeploy-myapp", []mockContainer{
		{id: "abc", service: "web", states: []mockState{
			{state: "running", health: "healthy", restartCount: 0},
		}},
	})
	d := &Deployer{runner: runner}

	status, services := d.stabilize(context.Background(), nil, "simpledeploy-myapp")
	if status != "success" {
		t.Fatalf("status: want success, got %s", status)
	}
	if len(services) != 1 || services[0].Service != "web" || services[0].State != "running" {
		t.Fatalf("services: %+v", services)
	}
}

func TestStabilize_UnstableOnRestartLoop(t *testing.T) {
	StabilizeTimeout = 1 * time.Second
	defer func() { StabilizeTimeout = 30 * time.Second }()

	// Container appears running on first poll but the restart count keeps
	// climbing - classic crash-loop signal.
	// First poll shows restarting (forces the loop to keep going). Later polls
	// show running but with a higher restart count, surfacing the bounce.
	runner := makeRunner(t, "simpledeploy-myapp", []mockContainer{
		{id: "abc", service: "web", states: []mockState{
			{state: "restarting", restartCount: 0},
			{state: "running", restartCount: 2},
			{state: "running", restartCount: 3},
		}},
	})
	d := &Deployer{runner: runner}

	status, services := d.stabilize(context.Background(), nil, "simpledeploy-myapp")
	if status != "unstable" {
		t.Fatalf("status: want unstable, got %s", status)
	}
	if len(services) != 1 || services[0].RestartCount == 0 {
		t.Fatalf("expected restart_count delta > 0, got %+v", services)
	}
}

func TestStabilize_UnstableOnUnhealthy(t *testing.T) {
	StabilizeTimeout = 500 * time.Millisecond
	defer func() { StabilizeTimeout = 30 * time.Second }()

	runner := makeRunner(t, "simpledeploy-myapp", []mockContainer{
		{id: "abc", service: "web", states: []mockState{
			{state: "running", health: "unhealthy"},
		}},
	})
	d := &Deployer{runner: runner}

	status, _ := d.stabilize(context.Background(), nil, "simpledeploy-myapp")
	if status != "unstable" {
		t.Fatalf("status: want unstable, got %s", status)
	}
}

func TestStabilize_NoContainers(t *testing.T) {
	StabilizeTimeout = 200 * time.Millisecond
	defer func() { StabilizeTimeout = 30 * time.Second }()

	runner := &MockRunner{
		Respond: func(call RunCall) (string, string, error, bool) {
			return "", "", nil, true // empty `docker ps` output
		},
	}
	d := &Deployer{runner: runner}

	status, services := d.stabilize(context.Background(), nil, "simpledeploy-myapp")
	if status != "success" {
		t.Fatalf("status: want success (no containers = nothing to wait for), got %s", status)
	}
	if len(services) != 0 {
		t.Fatalf("expected no services, got %+v", services)
	}
}

func TestStabilize_StreamsProgressLines(t *testing.T) {
	StabilizeTimeout = 500 * time.Millisecond
	defer func() { StabilizeTimeout = 30 * time.Second }()

	runner := makeRunner(t, "simpledeploy-myapp", []mockContainer{
		{id: "abc", service: "web", states: []mockState{
			{state: "running", health: "starting"},
			{state: "running", health: "healthy"},
		}},
	})
	d := &Deployer{runner: runner}
	dl := newDeployLog()

	d.stabilize(context.Background(), dl, "simpledeploy-myapp")

	// History should include at least one "Status:" line.
	dl.mu.Lock()
	defer dl.mu.Unlock()
	var found bool
	for _, line := range dl.history {
		if strings.Contains(line.Line, "Status:") || strings.Contains(line.Line, "Stabilizing") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected stabilize progress line in deploy log, got %d lines", len(dl.history))
	}
}

func TestFinishDeploy_ComposeFailure(t *testing.T) {
	d := &Deployer{runner: &MockRunner{}, Tracker: NewTracker()}
	dl, _ := d.Tracker.TrackWithLog("myapp", func() {})
	composeErr := fmt.Errorf("boom")

	status, services := d.finishDeploy(context.Background(), dl, "myapp", "simpledeploy-myapp", "deploy", composeErr)
	if status != "failed" {
		t.Fatalf("status: want failed, got %s", status)
	}
	if services != nil {
		t.Fatalf("services: want nil on compose failure, got %+v", services)
	}
}
