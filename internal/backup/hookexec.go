package backup

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

const (
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
	composeOneoffLabel  = "com.docker.compose.oneoff"
	composeNumberLabel  = "com.docker.compose.container-number"
)

// ContainerLister is the part of docker.Client the hook executor needs.
type ContainerLister interface {
	ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error)
}

// DockerHookExecutor runs backup hooks with the docker CLI. It finds
// containers only by their compose project and service labels and acts
// on them by ID, so a hook's service can never name an arbitrary
// container (another app's, SimpleDeploy's, or any other on the host).
type DockerHookExecutor struct {
	client ContainerLister
	docker func(ctx context.Context, args ...string) ([]byte, error)
}

// NewDockerHookExecutor returns an executor that lists containers through
// client and runs docker exec/stop/start for them.
func NewDockerHookExecutor(client ContainerLister) *DockerHookExecutor {
	return &DockerHookExecutor{client: client, docker: runDockerCLI}
}

func runDockerCLI(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

// serviceContainers returns project's containers for service, lowest
// replica number first. One-off (compose run) containers are left out.
func (e *DockerHookExecutor) serviceContainers(ctx context.Context, project, service string) ([]container.Summary, error) {
	if project == "" || service == "" {
		return nil, fmt.Errorf("hook needs an app and a service")
	}
	list, err := e.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", composeProjectLabel+"="+project),
			filters.Arg("label", composeServiceLabel+"="+service),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	// Re-check the labels rather than trusting the daemon filter alone.
	var own []container.Summary
	for _, c := range list {
		if c.Labels[composeProjectLabel] == project && c.Labels[composeServiceLabel] == service &&
			!strings.EqualFold(c.Labels[composeOneoffLabel], "true") {
			own = append(own, c)
		}
	}
	if len(own) == 0 {
		return nil, fmt.Errorf("no container found for service %q in this app; check the service name and that the app is deployed", service)
	}
	sort.SliceStable(own, func(i, j int) bool {
		ni, _ := strconv.Atoi(own[i].Labels[composeNumberLabel])
		nj, _ := strconv.Atoi(own[j].Labels[composeNumberLabel])
		if ni != nj {
			return ni < nj
		}
		return own[i].ID < own[j].ID
	})
	return own, nil
}

// ExecInService runs command with sh -c in the service's first running
// container.
func (e *DockerHookExecutor) ExecInService(ctx context.Context, project, service, command string) (string, error) {
	ctrs, err := e.serviceContainers(ctx, project, service)
	if err != nil {
		return "", err
	}
	for _, c := range ctrs {
		if c.State != container.StateRunning {
			continue
		}
		out, err := e.docker(ctx, "exec", c.ID, "sh", "-c", command)
		if err != nil {
			return string(out), fmt.Errorf("command failed: %w: %s", err, truncateOutput(out))
		}
		return string(out), nil
	}
	return "", fmt.Errorf("service %q is not running", service)
}

// StopService stops every container of the service.
func (e *DockerHookExecutor) StopService(ctx context.Context, project, service string) error {
	return e.eachContainer(ctx, project, service, "stop")
}

// StartService starts every container of the service.
func (e *DockerHookExecutor) StartService(ctx context.Context, project, service string) error {
	return e.eachContainer(ctx, project, service, "start")
}

func (e *DockerHookExecutor) eachContainer(ctx context.Context, project, service, verb string) error {
	ctrs, err := e.serviceContainers(ctx, project, service)
	if err != nil {
		return err
	}
	for _, c := range ctrs {
		if out, err := e.docker(ctx, verb, c.ID); err != nil {
			return fmt.Errorf("%s %s: %w: %s", verb, service, err, truncateOutput(out))
		}
	}
	return nil
}
