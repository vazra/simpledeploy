package deployer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// StabilizeTimeout is the max time we wait for containers to settle after
// `docker compose up -d` returns. Configurable for tests.
var StabilizeTimeout = 30 * time.Second

// stabilize polls the project's container states until all are running and
// healthchecks (if any) pass, or until StabilizeTimeout elapses.
//
// Status:
//
//	"success"  - all services running, no restart bouncing, healthy (or no healthcheck)
//	"unstable" - any service still restarting, unhealthy, exited, or restart-bounced
//	             during the window. Compose itself succeeded; the app is unhappy.
//
// Progress lines are streamed via dl.
func (d *Deployer) stabilize(ctx context.Context, dl *DeployLog, project string) (string, []ServiceState) {
	deadline := time.Now().Add(StabilizeTimeout)
	if dl != nil {
		dl.Send(OutputLine{Line: "Waiting for containers to stabilize...", Stream: "stdout"})
	}

	var initialRestart map[string]int
	var lastSummary string
	var services []ServiceState
	tick := 2 * time.Second

	for {
		states, err := d.queryProjectStates(ctx, project)
		if err != nil && dl != nil {
			dl.Send(OutputLine{Line: "stabilize: " + err.Error(), Stream: "stderr"})
		}
		services = states

		if initialRestart == nil {
			initialRestart = make(map[string]int, len(services))
			for _, s := range services {
				initialRestart[s.Service] = s.RestartCount
			}
		}
		// Express restart_count as a delta during the window.
		for i := range services {
			services[i].RestartCount -= initialRestart[services[i].Service]
			if services[i].RestartCount < 0 {
				services[i].RestartCount = 0
			}
		}

		anyRestarting := false
		anyExited := false
		anyUnhealthy := false
		anyStarting := false
		anyBounced := false
		for _, s := range services {
			switch s.State {
			case "restarting":
				anyRestarting = true
			case "exited", "dead":
				anyExited = true
			}
			switch s.Health {
			case "unhealthy":
				anyUnhealthy = true
			case "starting":
				anyStarting = true
			}
			if s.RestartCount > 0 {
				anyBounced = true
			}
		}

		summary := summarize(services)
		if summary != lastSummary && dl != nil {
			dl.Send(OutputLine{Line: "Status: " + summary, Stream: "stdout"})
			lastSummary = summary
		}

		if !anyRestarting && !anyExited && !anyUnhealthy && !anyStarting && !anyBounced {
			return "success", services
		}

		if time.Now().After(deadline) {
			return "unstable", services
		}

		select {
		case <-ctx.Done():
			return "unstable", services
		case <-time.After(tick):
		}
	}
}

// queryProjectStates returns the container state for every service in a
// compose project. Uses `docker ps` filtered by compose project label and
// `docker inspect` for State + RestartCount + Health.
func (d *Deployer) queryProjectStates(ctx context.Context, project string) ([]ServiceState, error) {
	stdout, _, err := d.runner.Run(ctx, "docker", "ps", "-a",
		"--filter", "label=com.docker.compose.project="+project,
		"--format", "{{.ID}}\t{{.Label \"com.docker.compose.service\"}}")
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return nil, nil
	}

	type entry struct{ id, service string }
	var entries []entry
	for _, line := range strings.Split(stdout, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 || parts[0] == "" {
			continue
		}
		entries = append(entries, entry{id: parts[0], service: parts[1]})
	}

	// Aggregate per-service: a service may have N replicas; collapse to the
	// "worst" state so a single sick replica still triggers unstable.
	merged := map[string]ServiceState{}
	for _, e := range entries {
		st, err := d.inspectState(ctx, e.id)
		if err != nil {
			continue
		}
		st.Service = e.service
		if existing, ok := merged[e.service]; ok {
			merged[e.service] = mergeWorst(existing, st)
		} else {
			merged[e.service] = st
		}
	}

	out := make([]ServiceState, 0, len(merged))
	for _, v := range merged {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out, nil
}

func (d *Deployer) inspectState(ctx context.Context, id string) (ServiceState, error) {
	stdout, _, err := d.runner.Run(ctx, "docker", "inspect", "--format", "{{json .State}}", id)
	if err != nil {
		return ServiceState{}, err
	}
	var raw struct {
		Status       string
		Restarting   bool
		Running      bool
		Dead         bool
		ExitCode     int
		RestartCount int // not in .State; falls back below
		Health       *struct {
			Status string
		}
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &raw); err != nil {
		return ServiceState{}, err
	}
	st := ServiceState{
		State:    raw.Status,
		ExitCode: raw.ExitCode,
	}
	if raw.Health != nil {
		st.Health = raw.Health.Status
	}
	// RestartCount lives at the container root, not in .State; query separately.
	if rc, _, err := d.runner.Run(ctx, "docker", "inspect", "--format", "{{.RestartCount}}", id); err == nil {
		fmt.Sscanf(strings.TrimSpace(rc), "%d", &st.RestartCount)
	}
	return st, nil
}

// mergeWorst picks the more concerning state across replicas.
func mergeWorst(a, b ServiceState) ServiceState {
	rank := func(s ServiceState) int {
		switch s.State {
		case "exited", "dead":
			return 4
		case "restarting":
			return 3
		case "created", "paused":
			return 2
		case "running":
			if s.Health == "unhealthy" {
				return 5
			}
			if s.Health == "starting" {
				return 1
			}
			return 0
		}
		return 1
	}
	if rank(b) > rank(a) {
		a.State = b.State
		a.Health = b.Health
		a.ExitCode = b.ExitCode
	}
	if b.RestartCount > a.RestartCount {
		a.RestartCount = b.RestartCount
	}
	return a
}

func summarize(services []ServiceState) string {
	if len(services) == 0 {
		return "no containers"
	}
	parts := make([]string, 0, len(services))
	for _, s := range services {
		label := s.State
		if s.Health != "" {
			label += "/" + s.Health
		}
		if s.RestartCount > 0 {
			label += fmt.Sprintf(" (restarted %dx)", s.RestartCount)
		}
		parts = append(parts, s.Service+"="+label)
	}
	return strings.Join(parts, ", ")
}
